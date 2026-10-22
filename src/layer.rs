use std::borrow::Cow;
use std::time::Duration;

use crossterm::event::{KeyCode, KeyEvent};
use pito_footer::{Edit, Hint, Input, InputBar, Key};
use pito_list::{Cell, Column, Keys, List, Part as Piece, Row, Step};
use ratatui::{
    Frame,
    layout::Rect,
    style::Style,
    text::Line,
    widgets::{Block, BorderType, Clear, Paragraph},
};

use crate::filter::matches;
use crate::label::Span;
use crate::layout::Spot;
use crate::palette::{Palette, Tone};
use crate::text::cells;

const CANCEL: &[Key] = &[Key::Esc];
const DISMISS: &[Key] = &[Key::Enter, Key::Char('q')];
const ROWS: u16 = 10;
const NARROWEST: u16 = 24;
const FIELD: u16 = 32;
const LASTING: Duration = Duration::from_secs(3);

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Event {
    Chosen { part: usize, choice: usize },
    Moved { part: usize, choice: usize },
    Edited { part: usize },
    Submitted { part: usize },
    Closed,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Choice {
    spans: Vec<(String, Style)>,
    skip: bool,
    confirm: Option<Cow<'static, str>>,
}

impl Choice {
    pub fn new(text: impl Into<String>) -> Self {
        Choice::spans([(text.into(), Style::new())])
    }

    pub fn spans(spans: impl IntoIterator<Item = (String, Style)>) -> Self {
        Choice {
            spans: spans.into_iter().collect(),
            skip: false,
            confirm: None,
        }
    }

    pub fn skip(mut self) -> Self {
        self.skip = true;
        self
    }

    pub fn confirm(mut self, warning: impl Into<Cow<'static, str>>) -> Self {
        self.confirm = Some(warning.into());
        self
    }

    pub fn text(&self) -> String {
        self.spans.iter().map(|(text, _)| text.as_str()).collect()
    }

    fn width(&self) -> u16 {
        u16::try_from(cells(&self.text())).unwrap_or(u16::MAX)
    }

    fn row(&self) -> Row {
        let parts = self
            .spans
            .iter()
            .map(|(text, style)| Piece::new(text.clone()).style(*style));
        if self.skip {
            Row::heading(parts)
        } else {
            Row::new([Cell::parts(parts)])
        }
    }
}

#[derive(Debug, Clone)]
pub struct Field {
    label: Cow<'static, str>,
    placeholder: Cow<'static, str>,
    input: Input,
}

impl Field {
    pub fn new(label: impl Into<Cow<'static, str>>) -> Self {
        Field {
            label: label.into(),
            placeholder: Cow::Borrowed(""),
            input: Input::new(),
        }
    }

    pub fn placeholder(mut self, text: impl Into<Cow<'static, str>>) -> Self {
        self.placeholder = text.into();
        self
    }

    pub fn input(mut self, input: Input) -> Self {
        self.input = input;
        self
    }

    fn width(&self) -> u16 {
        let label = u16::try_from(cells(&self.label)).unwrap_or(u16::MAX);
        label.saturating_add(FIELD)
    }
}

#[derive(Debug, Clone)]
struct Choices {
    all: Vec<Choice>,
    shown: Vec<usize>,
    list: List,
    rows: u16,
    cut: u16,
}

impl Choices {
    fn new(all: Vec<Choice>, rows: u16) -> Self {
        let mut choices = Choices {
            all,
            shown: Vec::new(),
            list: List::new().keys(Keys::new()),
            rows,
            cut: 0,
        };
        choices.sift("");
        choices
    }

    fn sift(&mut self, query: &str) {
        let kept = self.current();
        self.shown = (0..self.all.len())
            .filter(|index| query.trim().is_empty() || matches(&self.all[*index].text(), query))
            .collect();
        let rows: Vec<Row> = self
            .shown
            .iter()
            .map(|index| self.all[*index].row())
            .collect();
        self.list.set_rows(rows);
        let at = kept
            .and_then(|kept| self.shown.iter().position(|index| *index == kept))
            .or_else(|| self.shown.iter().position(|index| !self.all[*index].skip));
        if let Some(at) = at {
            self.list.select(at);
        }
    }

    fn current(&self) -> Option<usize> {
        self.list
            .selected()
            .and_then(|at| self.shown.get(at).copied())
    }

    fn width(&self) -> u16 {
        self.all.iter().map(Choice::width).max().unwrap_or(0)
    }

    fn height(&self) -> u16 {
        u16::try_from(self.shown.len())
            .unwrap_or(u16::MAX)
            .clamp(1, self.rows.max(1))
            .saturating_sub(self.cut)
            .max(1)
    }
}

#[derive(Debug, Clone)]
enum Part {
    Text(Vec<Line<'static>>),
    List(Box<Choices>),
    Field(Field),
}

impl Part {
    fn focusable(&self) -> bool {
        !matches!(self, Part::Text(_))
    }

    fn height(&self) -> u16 {
        match self {
            Part::Text(lines) => u16::try_from(lines.len()).unwrap_or(u16::MAX),
            Part::List(choices) => choices.height(),
            Part::Field(_) => 1,
        }
    }

    fn width(&self) -> u16 {
        match self {
            Part::Text(lines) => lines
                .iter()
                .map(|line| u16::try_from(line.width()).unwrap_or(u16::MAX))
                .max()
                .unwrap_or(0),
            Part::List(choices) => choices.width().saturating_add(3),
            Part::Field(field) => field.width(),
        }
    }
}

#[derive(Debug, Clone)]
pub struct Modal {
    pub(crate) id: u64,
    title: Cow<'static, str>,
    parts: Vec<Part>,
    focus: usize,
    status: Option<(Cow<'static, str>, Tone)>,
    width: u16,
    pub(crate) cancel: &'static [Key],
    pub(crate) close: &'static [Key],
    pub(crate) hints: Vec<Hint<'static>>,
    filter: Option<(usize, usize)>,
    armed: Option<(usize, usize)>,
}

pub(crate) enum Outcome {
    Stay,
    Heard(Event),
    Close,
}

impl Modal {
    pub fn new(id: u64, title: impl Into<Cow<'static, str>>) -> Self {
        Modal {
            id,
            title: title.into(),
            parts: Vec::new(),
            focus: 0,
            status: None,
            width: 0,
            cancel: CANCEL,
            close: &[],
            hints: Vec::new(),
            filter: None,
            armed: None,
        }
    }

    pub fn alert(
        id: u64,
        title: impl Into<Cow<'static, str>>,
        lines: impl IntoIterator<Item = Line<'static>>,
    ) -> Self {
        Modal::new(id, title).text(lines).close_keys(DISMISS)
    }

    pub fn text(mut self, lines: impl IntoIterator<Item = Line<'static>>) -> Self {
        self.parts.push(Part::Text(lines.into_iter().collect()));
        self.refocus();
        self
    }

    pub fn list(self, choices: impl IntoIterator<Item = Choice>) -> Self {
        self.list_rows(choices, ROWS)
    }

    pub fn list_rows(mut self, choices: impl IntoIterator<Item = Choice>, rows: u16) -> Self {
        let choices = Choices::new(choices.into_iter().collect(), rows);
        self.parts.push(Part::List(Box::new(choices)));
        self.refocus();
        self
    }

    pub fn field(mut self, field: Field) -> Self {
        self.parts.push(Part::Field(field));
        self.refocus();
        self
    }

    pub fn filter(mut self, field: usize, list: usize) -> Self {
        let linked = matches!(self.parts.get(field), Some(Part::Field(_)))
            && matches!(self.parts.get(list), Some(Part::List(_)));
        self.filter = linked.then_some((field, list));
        self
    }

    pub fn status(mut self, text: impl Into<Cow<'static, str>>, tone: Tone) -> Self {
        self.status = Some((text.into(), tone));
        self
    }

    pub fn width(mut self, cells: u16) -> Self {
        self.width = cells;
        self
    }

    pub fn hints(mut self, hints: impl IntoIterator<Item = Hint<'static>>) -> Self {
        self.hints = hints.into_iter().collect();
        self
    }

    pub fn cancel_keys(mut self, keys: &'static [Key]) -> Self {
        self.cancel = keys;
        self
    }

    pub fn close_keys(mut self, keys: &'static [Key]) -> Self {
        self.close = keys;
        self
    }

    pub fn focus(mut self, part: usize) -> Self {
        if self.parts.get(part).is_some_and(Part::focusable) {
            self.focus = part;
        }
        self
    }

    pub fn id(&self) -> u64 {
        self.id
    }

    pub fn focused(&self) -> usize {
        self.focus
    }

    pub fn set_status(&mut self, status: Option<(Cow<'static, str>, Tone)>) {
        self.status = status;
    }

    pub fn set_text(
        &mut self,
        part: usize,
        lines: impl IntoIterator<Item = Line<'static>>,
    ) -> bool {
        let Some(Part::Text(text)) = self.parts.get_mut(part) else {
            return false;
        };
        *text = lines.into_iter().collect();
        true
    }

    pub fn set_choices(&mut self, part: usize, choices: impl IntoIterator<Item = Choice>) -> bool {
        let query = self.query(part).unwrap_or_default();
        let Some(Part::List(list)) = self.parts.get_mut(part) else {
            return false;
        };
        list.all = choices.into_iter().collect();
        list.sift(&query);
        self.armed = None;
        true
    }

    pub fn choice(&self, part: usize) -> Option<usize> {
        match self.parts.get(part)? {
            Part::List(choices) => choices.current(),
            _ => None,
        }
    }

    pub fn value(&self, part: usize) -> Option<&str> {
        match self.parts.get(part)? {
            Part::Field(field) => Some(field.input.value()),
            _ => None,
        }
    }

    pub fn input_mut(&mut self, part: usize) -> Option<&mut Input> {
        match self.parts.get_mut(part)? {
            Part::Field(field) => Some(&mut field.input),
            _ => None,
        }
    }

    fn query(&self, list: usize) -> Option<String> {
        let (field, linked) = self.filter?;
        (linked == list).then(|| self.value(field).unwrap_or_default().to_string())
    }

    fn refocus(&mut self) {
        if !self.parts.get(self.focus).is_some_and(Part::focusable)
            && let Some(at) = self.parts.iter().position(Part::focusable)
        {
            self.focus = at;
        }
    }

    fn cycle(&mut self, by: isize) {
        self.armed = None;
        let count = self.parts.len();
        if count == 0 {
            return;
        }
        let mut at = self.focus;
        for _ in 0..count {
            at = (at as isize + by).rem_euclid(count as isize) as usize;
            if self.parts[at].focusable() {
                self.focus = at;
                return;
            }
        }
    }

    fn steered(&self) -> Option<usize> {
        match self.parts.get(self.focus)? {
            Part::List(_) => Some(self.focus),
            Part::Field(_) => match self.filter {
                Some((field, list)) if field == self.focus => Some(list),
                _ => self
                    .parts
                    .iter()
                    .position(|part| matches!(part, Part::List(_))),
            },
            Part::Text(_) => None,
        }
    }

    fn choose(&mut self, part: usize) -> Outcome {
        let Some(Part::List(choices)) = self.parts.get(part) else {
            return Outcome::Stay;
        };
        let Some(choice) = choices.current() else {
            return Outcome::Stay;
        };
        if choices.all[choice].confirm.is_some() && self.armed != Some((part, choice)) {
            self.armed = Some((part, choice));
            return Outcome::Stay;
        }
        self.armed = None;
        Outcome::Heard(Event::Chosen { part, choice })
    }

    pub(crate) fn key(&mut self, event: KeyEvent) -> Outcome {
        let key: Key = event.into();
        if self.cancel.contains(&key) || self.close.contains(&key) {
            return Outcome::Close;
        }
        match key {
            Key::Tab => {
                self.cycle(1);
                return Outcome::Stay;
            }
            Key::BackTab => {
                self.cycle(-1);
                return Outcome::Stay;
            }
            Key::Enter => {
                return match (self.parts.get(self.focus), self.filter) {
                    (Some(Part::List(_)), _) => self.choose(self.focus),
                    (Some(Part::Field(_)), Some((field, list))) if field == self.focus => {
                        self.choose(list)
                    }
                    (Some(Part::Field(_)), _) => {
                        Outcome::Heard(Event::Submitted { part: self.focus })
                    }
                    _ => Outcome::Stay,
                };
            }
            _ => {}
        }
        let on_field = matches!(self.parts.get(self.focus), Some(Part::Field(_)));
        let listing = !on_field
            || matches!(
                event.code,
                KeyCode::Up | KeyCode::Down | KeyCode::PageUp | KeyCode::PageDown
            );
        if listing
            && let Some(part) = self.steered()
            && let Some(Part::List(choices)) = self.parts.get_mut(part)
        {
            match choices.list.key(event.into()) {
                Step::Moved => {
                    self.armed = None;
                    let choice = choices.current().unwrap_or(0);
                    return Outcome::Heard(Event::Moved { part, choice });
                }
                Step::Held => return Outcome::Stay,
                _ => {}
            }
        }
        self.type_in(|input| input.key(key))
    }

    pub(crate) fn paste(&mut self, text: &str) -> Outcome {
        self.type_in(|input| input.paste(text))
    }

    fn type_in(&mut self, edit: impl FnOnce(&mut Input) -> Edit) -> Outcome {
        let part = self.focus;
        let Some(Part::Field(field)) = self.parts.get_mut(part) else {
            return Outcome::Stay;
        };
        if edit(&mut field.input) != Edit::Changed {
            return Outcome::Stay;
        }
        self.armed = None;
        if let Some((linked, list)) = self.filter
            && linked == part
        {
            let query = self.value(part).unwrap_or_default().to_string();
            if let Some(Part::List(choices)) = self.parts.get_mut(list) {
                choices.sift(&query);
            }
        }
        Outcome::Heard(Event::Edited { part })
    }

    fn line(&self) -> Option<(&str, Tone)> {
        if let Some((part, choice)) = self.armed {
            let warning = match self.parts.get(part) {
                Some(Part::List(choices)) => {
                    choices.all.get(choice).and_then(|it| it.confirm.as_deref())
                }
                _ => None,
            };
            if let Some(warning) = warning {
                return Some((warning, Tone::Warn));
            }
        }
        self.status
            .as_ref()
            .map(|(text, tone)| (text.as_ref(), *tone))
            .filter(|(text, _)| !text.is_empty())
    }

    pub(crate) fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        for part in &mut self.parts {
            if let Part::List(choices) = part {
                choices.cut = 0;
            }
        }
        let title = u16::try_from(cells(&self.title)).unwrap_or(u16::MAX);
        let line = self.line().map_or(0, |(text, _)| {
            u16::try_from(cells(text)).unwrap_or(u16::MAX)
        });
        let natural = self
            .parts
            .iter()
            .map(Part::width)
            .chain([title.saturating_add(2), line])
            .max()
            .unwrap_or(0)
            .saturating_add(4)
            .max(NARROWEST);
        let width = if self.width > 0 { self.width } else { natural }.min(area.width);
        let status = u16::from(self.line().is_some()) * 2;
        let gaps = u16::try_from(self.parts.len().saturating_sub(1)).unwrap_or(0);
        let body: u16 = self.parts.iter().map(Part::height).sum();
        let wanted = body
            .saturating_add(gaps)
            .saturating_add(status)
            .saturating_add(2);
        let height = wanted.min(area.height);
        let mut spare = wanted - height;
        for part in self.parts.iter_mut().rev() {
            if let Part::List(choices) = part {
                let cut = spare.min(choices.height().saturating_sub(1));
                choices.cut = cut;
                spare -= cut;
            }
        }
        let place = Spot::THIRD.place(area, width, height);
        frame.render_widget(Clear, place);
        let block = Block::bordered()
            .border_type(BorderType::Rounded)
            .border_style(palette.rule)
            .style(palette.base)
            .title(Line::styled(format!(" {} ", self.title), palette.selected));
        let inner = block.inner(place);
        frame.render_widget(block, place);
        let inner = Rect {
            x: inner.x.saturating_add(1),
            width: inner.width.saturating_sub(2),
            ..inner
        };
        let mut y = inner.y;
        let columns = [Column::new("", 4, 0).flex()];
        let focus = self.focus;
        for (index, part) in self.parts.iter_mut().enumerate() {
            if y >= inner.bottom() {
                break;
            }
            if index > 0 {
                y += 1;
            }
            let rows = part.height().min(inner.bottom().saturating_sub(y));
            let at = Rect::new(inner.x, y, inner.width, rows);
            match part {
                Part::Text(lines) => {
                    let lines: Vec<Line> = lines
                        .iter()
                        .map(|line| line.clone().patch_style(palette.ink))
                        .collect();
                    frame.render_widget(Paragraph::new(lines), at);
                }
                Part::List(choices) => {
                    let view = palette.view(&mut choices.list, &columns).header(false);
                    frame.render_widget(view, at);
                }
                Part::Field(field) => {
                    let bar = InputBar::new(&field.label, &field.input)
                        .placeholder(&field.placeholder)
                        .rule(false)
                        .caret(index == focus)
                        .styles(palette.footer());
                    frame.render_widget(bar, at);
                }
            }
            y = y.saturating_add(rows);
        }
        if let Some((text, tone)) = self.line() {
            let at = Rect::new(inner.x, inner.bottom().saturating_sub(1), inner.width, 1);
            frame.render_widget(
                Paragraph::new(Line::styled(text.to_string(), palette.tone(tone))),
                at,
            );
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Takeover {
    pub(crate) id: u64,
    pub(crate) title: Vec<Span>,
    pub(crate) label: Cow<'static, str>,
    pub(crate) cancel: &'static [Key],
}

impl Takeover {
    pub fn new(id: u64, title: impl Into<Cow<'static, str>>) -> Self {
        Takeover {
            id,
            title: vec![(title.into(), Style::new())],
            label: Cow::Borrowed(""),
            cancel: &[],
        }
    }

    pub fn title_spans<T: Into<Cow<'static, str>>>(
        mut self,
        spans: impl IntoIterator<Item = (T, Style)>,
    ) -> Self {
        self.title = spans
            .into_iter()
            .map(|(text, style)| (text.into(), style))
            .collect();
        self
    }

    pub fn label(mut self, text: impl Into<Cow<'static, str>>) -> Self {
        self.label = text.into();
        self
    }

    pub fn cancel(mut self, keys: &'static [Key]) -> Self {
        self.cancel = keys;
        self
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Toast {
    pub(crate) text: Cow<'static, str>,
    pub(crate) tone: Tone,
    pub(crate) spot: Spot,
    pub(crate) lasting: Duration,
}

impl Toast {
    pub fn new(text: impl Into<Cow<'static, str>>) -> Self {
        Toast {
            text: text.into(),
            tone: Tone::Ink,
            spot: Spot::TOP_RIGHT,
            lasting: LASTING,
        }
    }

    pub fn tone(mut self, tone: Tone) -> Self {
        self.tone = tone;
        self
    }

    pub fn at(mut self, spot: Spot) -> Self {
        self.spot = spot;
        self
    }

    pub fn lasting(mut self, lasting: Duration) -> Self {
        self.lasting = lasting;
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::dump;
    use crossterm::event::KeyModifiers;
    use ratatui::{Terminal, backend::TestBackend, buffer::Buffer, style::Color};

    const TONES: [Tone; 7] = [
        Tone::Ink,
        Tone::Strong,
        Tone::Muted,
        Tone::Accent,
        Tone::Good,
        Tone::Warn,
        Tone::Bad,
    ];

    fn press(code: KeyCode) -> KeyEvent {
        KeyEvent::new(code, KeyModifiers::NONE)
    }

    fn typed(modal: &mut Modal, text: &str) {
        for c in text.chars() {
            modal.key(press(KeyCode::Char(c)));
        }
    }

    fn palette() -> Palette {
        Palette::new(Color::Blue)
    }

    fn shot_in(modal: &mut Modal, width: u16, height: u16, area: Rect) -> Buffer {
        let Ok(mut terminal) = Terminal::new(TestBackend::new(width, height));
        let Ok(_) = terminal.draw(|frame| modal.draw(frame, area, &palette()));
        terminal.backend().buffer().clone()
    }

    fn shot(modal: &mut Modal, width: u16, height: u16) -> Buffer {
        shot_in(modal, width, height, Rect::new(0, 0, width, height))
    }

    fn drawn(modal: &mut Modal, width: u16, height: u16) -> String {
        dump::text(&shot(modal, width, height))
    }

    fn boxed(buffer: &Buffer) -> Option<Rect> {
        let area = buffer.area;
        let mut corner = None;
        let mut end = None;
        for y in area.top()..area.bottom() {
            for x in area.left()..area.right() {
                match buffer[(x, y)].symbol() {
                    "╭" if corner.is_none() => corner = Some((x, y)),
                    "╯" => end = Some((x, y)),
                    _ => {}
                }
            }
        }
        let ((left, top), (right, bottom)) = (corner?, end?);
        Some(Rect::new(left, top, right - left + 1, bottom - top + 1))
    }

    fn found(buffer: &Buffer, needle: &str) -> Option<(u16, u16)> {
        let first = needle.chars().next()?.to_string();
        let area = buffer.area;
        (area.top()..area.bottom()).find_map(|y| {
            (area.left()..area.right()).find_map(|x| {
                let rest: String = (x..area.right().min(x + 64))
                    .map(|at| buffer[(at, y)].symbol().to_string())
                    .collect();
                (buffer[(x, y)].symbol() == first && rest.starts_with(needle)).then_some((x, y))
            })
        })
    }

    fn wears(buffer: &Buffer, at: (u16, u16), style: Style) -> bool {
        let cell = &buffer[at];
        style.fg.is_none_or(|fg| cell.fg == fg) && cell.modifier.contains(style.add_modifier)
    }

    fn names(count: usize) -> Vec<Choice> {
        (0..count)
            .map(|n| Choice::new(format!("item {n:02}")))
            .collect()
    }

    #[test]
    fn a_filtered_list_skips_headings_and_a_risky_row_needs_a_second_enter() {
        let mut modal = Modal::new(7, "Move")
            .field(Field::new("Find"))
            .list([
                Choice::new("Recent").skip(),
                Choice::new("archive"),
                Choice::new("delete forever").confirm("enter again deletes it"),
                Choice::new("draft"),
            ])
            .filter(0, 1);
        assert_eq!(
            modal.choice(1),
            Some(1),
            "the cursor starts past the heading"
        );
        typed(&mut modal, "de");
        assert_eq!(modal.choice(1), Some(2), "only delete forever is left");
        assert!(matches!(modal.key(press(KeyCode::Enter)), Outcome::Stay));
        assert_eq!(modal.line(), Some(("enter again deletes it", Tone::Warn)));
        let second = modal.key(press(KeyCode::Enter));
        assert!(matches!(
            second,
            Outcome::Heard(Event::Chosen { part: 1, choice: 2 })
        ));
        assert!(matches!(modal.key(press(KeyCode::Esc)), Outcome::Close));
    }

    #[test]
    fn every_builder_lands_in_the_modal_and_every_accessor_reads_it_back() {
        let mut modal = Modal::new(9, "Settings")
            .text([Line::from("pick one")])
            .field(
                Field::new("Name")
                    .placeholder("who")
                    .input(Input::new().with("ann")),
            )
            .list([Choice::new("red"), Choice::new("blue")])
            .status("saved", Tone::Good)
            .width(50)
            .hints([Hint::new("enter", "choose")])
            .cancel_keys(&[Key::Char('x')])
            .close_keys(&[Key::Ctrl('d')])
            .focus(2);
        assert_eq!(modal.id(), 9);
        assert_eq!(modal.focused(), 2);
        assert_eq!(modal.hints.len(), 1);
        assert_eq!(modal.cancel, &[Key::Char('x')]);
        assert_eq!(modal.close, &[Key::Ctrl('d')]);
        assert_eq!(modal.value(1), Some("ann"));
        assert_eq!(modal.value(0), None, "a text part has no value");
        assert_eq!(modal.value(2), None, "a list has no value");
        assert_eq!(modal.value(99), None);
        assert_eq!(modal.choice(2), Some(0));
        assert_eq!(modal.choice(1), None, "a field has no choice");
        assert_eq!(modal.choice(99), None);
        assert!(modal.input_mut(0).is_none());
        assert!(modal.input_mut(99).is_none());
        if let Some(input) = modal.input_mut(1) {
            input.set("bob");
        }
        assert_eq!(modal.value(1), Some("bob"));
        let buffer = shot(&mut modal, 120, 40);
        let shown = dump::text(&buffer);
        for word in [
            "Settings", "pick one", "Name", "bob", "red", "blue", "saved",
        ] {
            assert!(shown.contains(word), "{word} is drawn:\n{shown}");
        }
        assert_eq!(boxed(&buffer).map(|place| place.width), Some(50));
        let at = found(&buffer, "saved");
        assert!(at.is_some_and(|at| wears(&buffer, at, palette().good)));
    }

    #[test]
    fn set_text_and_set_choices_only_touch_the_part_kind_they_name() {
        let mut modal = Modal::new(1, "Edit")
            .text([Line::from("before")])
            .field(Field::new("Find"))
            .list([Choice::new("one"), Choice::new("two")]);
        assert!(modal.set_text(0, [Line::from("after")]));
        assert!(!modal.set_text(1, [Line::from("lost")]));
        assert!(!modal.set_text(7, [Line::from("lost")]));
        assert!(!modal.set_choices(1, [Choice::new("lost")]));
        assert!(!modal.set_choices(0, [Choice::new("lost")]));
        assert!(!modal.set_choices(42, [Choice::new("lost")]));
        assert!(modal.set_choices(2, [Choice::new("three"), Choice::new("four")]));
        let shown = drawn(&mut modal, 80, 24);
        assert!(shown.contains("after") && !shown.contains("before"));
        assert!(shown.contains("three") && shown.contains("four"));
        assert!(!shown.contains("lost") && !shown.contains("one"));
        modal.set_status(Some(("busy".into(), Tone::Warn)));
        assert_eq!(modal.line(), Some(("busy", Tone::Warn)));
        modal.set_status(None);
        assert_eq!(modal.line(), None);
        assert!(!drawn(&mut modal, 80, 24).contains("busy"));
    }

    #[test]
    fn set_choices_keeps_the_filter_query_and_drops_an_armed_confirm() {
        let mut modal = Modal::new(1, "Go")
            .field(Field::new("Find"))
            .list([Choice::new("wipe").confirm("again wipes")])
            .filter(0, 1);
        modal.key(press(KeyCode::Enter));
        assert_eq!(modal.line(), Some(("again wipes", Tone::Warn)));
        typed(&mut modal, "b");
        assert_eq!(modal.line(), None, "typing disarms");
        modal.set_choices(1, [Choice::new("alpha"), Choice::new("beta")]);
        assert_eq!(modal.choice(1), Some(1), "the query b still sifts");
        let shown = drawn(&mut modal, 80, 24);
        assert!(shown.contains("beta") && !shown.contains("alpha"));
    }

    #[test]
    fn focus_lands_only_on_parts_that_take_it() {
        let modal = Modal::new(1, "F")
            .text([Line::from("hi")])
            .field(Field::new("A"));
        assert_eq!(modal.focused(), 1, "the first focusable part takes focus");
        assert_eq!(modal.focus(0).focused(), 1, "a text part refuses focus");
        let modal = Modal::new(1, "F").field(Field::new("A")).focus(9);
        assert_eq!(modal.focused(), 0, "an index past the parts is ignored");
        let modal = Modal::new(1, "F").focus(1).field(Field::new("A"));
        assert_eq!(
            modal.focused(),
            0,
            "focus before the part exists is ignored"
        );
        let modal = Modal::new(1, "F")
            .field(Field::new("A"))
            .field(Field::new("B"))
            .focus(1)
            .text([Line::from("late")]);
        assert_eq!(modal.focused(), 1, "a later part keeps the chosen focus");
    }

    #[test]
    fn tab_and_backtab_cycle_over_focusable_parts_only() {
        let mut none = Modal::alert(1, "A", [Line::from("x")]);
        assert!(matches!(none.key(press(KeyCode::Tab)), Outcome::Stay));
        assert!(matches!(none.key(press(KeyCode::BackTab)), Outcome::Stay));
        assert_eq!(none.focused(), 0);
        let mut empty = Modal::new(1, "E");
        empty.key(press(KeyCode::Tab));
        assert_eq!(empty.focused(), 0);
        let mut one = Modal::new(1, "O")
            .text([Line::from("x")])
            .field(Field::new("A"));
        one.key(press(KeyCode::Tab));
        assert_eq!(one.focused(), 1);
        one.key(press(KeyCode::BackTab));
        assert_eq!(one.focused(), 1);
        let mut several = Modal::new(1, "S")
            .text([Line::from("x")])
            .field(Field::new("A"))
            .text([Line::from("y")])
            .list([Choice::new("c")])
            .field(Field::new("B"));
        let mut seen = Vec::new();
        for _ in 0..4 {
            several.key(press(KeyCode::Tab));
            seen.push(several.focused());
        }
        assert_eq!(seen, [3, 4, 1, 3]);
        let mut back = Vec::new();
        for _ in 0..3 {
            several.key(KeyEvent::new(KeyCode::BackTab, KeyModifiers::SHIFT));
            back.push(several.focused());
        }
        assert_eq!(back, [1, 4, 3]);
    }

    #[test]
    fn a_field_takes_typing_paste_and_editing_keys() {
        let mut modal = Modal::new(1, "Type")
            .field(Field::new("Name"))
            .list([Choice::new("one")]);
        assert!(matches!(
            modal.key(press(KeyCode::Char('a'))),
            Outcome::Heard(Event::Edited { part: 0 })
        ));
        typed(&mut modal, "bc");
        modal.key(press(KeyCode::Left));
        modal.key(press(KeyCode::Backspace));
        assert_eq!(modal.value(0), Some("ac"));
        modal.key(press(KeyCode::Home));
        assert!(matches!(modal.key(press(KeyCode::Left)), Outcome::Stay));
        modal.key(press(KeyCode::Delete));
        assert_eq!(modal.value(0), Some("c"));
        modal.key(press(KeyCode::End));
        assert!(matches!(
            modal.paste("xy"),
            Outcome::Heard(Event::Edited { part: 0 })
        ));
        assert_eq!(modal.value(0), Some("cxy"));
        modal.key(KeyEvent::new(KeyCode::Char('u'), KeyModifiers::CONTROL));
        assert_eq!(modal.value(0), Some(""));
        modal.key(press(KeyCode::Tab));
        assert!(
            matches!(modal.paste("z"), Outcome::Stay),
            "a list takes no paste"
        );
        assert!(matches!(
            modal.key(press(KeyCode::Char('q'))),
            Outcome::Stay
        ));
        assert_eq!(modal.value(0), Some(""));
    }

    #[test]
    fn a_filter_field_sifts_its_list_by_case_unicode_and_words() {
        let mut modal = Modal::new(1, "Find")
            .field(Field::new("Find"))
            .list([
                Choice::new("Alpha"),
                Choice::new("beta"),
                Choice::new("Ștefan"),
                Choice::new("gamma ray"),
            ])
            .filter(0, 1);
        let shown = drawn(&mut modal, 80, 24);
        for name in ["Alpha", "beta", "Ștefan", "gamma ray"] {
            assert!(shown.contains(name));
        }
        typed(&mut modal, "ALP");
        assert_eq!(modal.choice(1), Some(0));
        let shown = drawn(&mut modal, 80, 24);
        assert!(shown.contains("Alpha") && !shown.contains("beta"));
        modal.key(KeyEvent::new(KeyCode::Char('u'), KeyModifiers::CONTROL));
        typed(&mut modal, "ȘTE");
        assert_eq!(modal.choice(1), Some(2));
        modal.key(KeyEvent::new(KeyCode::Char('u'), KeyModifiers::CONTROL));
        typed(&mut modal, "ray gam");
        assert_eq!(
            modal.choice(1),
            Some(3),
            "every word must match, in any order"
        );
        typed(&mut modal, "zz");
        assert_eq!(modal.choice(1), None);
        assert!(matches!(modal.key(press(KeyCode::Enter)), Outcome::Stay));
        assert!(matches!(modal.key(press(KeyCode::Down)), Outcome::Stay));
        let shown = drawn(&mut modal, 80, 24);
        assert!(!shown.contains("gamma") && !shown.contains("Alpha"));
        modal.key(KeyEvent::new(KeyCode::Char('u'), KeyModifiers::CONTROL));
        typed(&mut modal, "   ");
        assert_eq!(modal.choice(1), Some(0), "a blank query shows everything");
    }

    #[test]
    fn the_current_choice_follows_the_filter_while_it_still_matches() {
        let mut modal = Modal::new(1, "Find")
            .field(Field::new("Find"))
            .list([
                Choice::new("Alpha"),
                Choice::new("beta"),
                Choice::new("delta"),
            ])
            .filter(0, 1);
        assert!(matches!(
            modal.key(press(KeyCode::Down)),
            Outcome::Heard(Event::Moved { part: 1, choice: 1 })
        ));
        assert_eq!(modal.focused(), 0, "arrows steer the list from the field");
        typed(&mut modal, "ta");
        assert_eq!(modal.choice(1), Some(1), "beta still matches and stays");
        modal.key(press(KeyCode::Down));
        assert_eq!(modal.choice(1), Some(2));
        modal.key(press(KeyCode::Backspace));
        modal.key(press(KeyCode::Backspace));
        typed(&mut modal, "alp");
        assert_eq!(
            modal.choice(1),
            Some(0),
            "a lost choice falls to the first left"
        );
        assert!(matches!(
            modal.key(press(KeyCode::Enter)),
            Outcome::Heard(Event::Chosen { part: 1, choice: 0 })
        ));
    }

    #[test]
    fn a_filter_with_wrong_parts_links_nothing() {
        let mut modal = Modal::new(1, "Find")
            .list([Choice::new("one"), Choice::new("two")])
            .field(Field::new("Find"))
            .filter(0, 1)
            .focus(1);
        typed(&mut modal, "two");
        assert_eq!(modal.choice(0), Some(0), "nothing was sifted");
        assert!(matches!(
            modal.key(press(KeyCode::Enter)),
            Outcome::Heard(Event::Submitted { part: 1 })
        ));
        let mut modal = Modal::new(1, "Find")
            .field(Field::new("A"))
            .field(Field::new("B"))
            .list([Choice::new("one")])
            .filter(0, 2)
            .focus(1);
        assert!(matches!(
            modal.key(press(KeyCode::Enter)),
            Outcome::Heard(Event::Submitted { part: 1 })
        ));
        assert!(matches!(modal.key(press(KeyCode::Down)), Outcome::Stay));
    }

    #[test]
    fn enter_on_a_list_chooses_and_a_confirm_arms_first() {
        let mut modal = Modal::new(1, "Pick").list([
            Choice::new("keep"),
            Choice::new("wipe").confirm("again wipes"),
        ]);
        assert!(matches!(
            modal.key(press(KeyCode::Enter)),
            Outcome::Heard(Event::Chosen { part: 0, choice: 0 })
        ));
        modal.key(press(KeyCode::Down));
        assert!(matches!(modal.key(press(KeyCode::Enter)), Outcome::Stay));
        let buffer = shot(&mut modal, 80, 24);
        let at = found(&buffer, "again wipes");
        assert!(at.is_some_and(|at| wears(&buffer, at, palette().warn)));
        modal.key(press(KeyCode::Up));
        modal.key(press(KeyCode::Down));
        assert_eq!(modal.line(), None, "moving disarms");
        assert!(matches!(modal.key(press(KeyCode::Enter)), Outcome::Stay));
        assert!(matches!(
            modal.key(press(KeyCode::Enter)),
            Outcome::Heard(Event::Chosen { part: 0, choice: 1 })
        ));
        assert_eq!(modal.line(), None, "choosing disarms");
        let mut quiet = Modal::new(1, "T").text([Line::from("x")]);
        assert!(matches!(quiet.key(press(KeyCode::Enter)), Outcome::Stay));
    }

    #[test]
    fn an_armed_confirm_does_not_carry_to_another_list() {
        let mut modal = Modal::new(1, "Two")
            .list([
                Choice::new("keep"),
                Choice::new("wipe").confirm("again wipes"),
            ])
            .list([
                Choice::new("Danger").skip(),
                Choice::new("burn").confirm("again burns"),
            ]);
        modal.key(press(KeyCode::Down));
        assert!(matches!(modal.key(press(KeyCode::Enter)), Outcome::Stay));
        modal.key(press(KeyCode::Tab));
        assert_eq!(modal.focused(), 1);
        assert_eq!(modal.line(), None, "a focus change disarms");
        assert!(
            matches!(modal.key(press(KeyCode::Enter)), Outcome::Stay),
            "burn still asks first"
        );
        assert!(matches!(
            modal.key(press(KeyCode::Enter)),
            Outcome::Heard(Event::Chosen { part: 1, choice: 1 })
        ));
    }

    #[test]
    fn the_warning_shown_is_the_armed_lists_own() {
        let mut modal = Modal::new(1, "Two")
            .list([
                Choice::new("keep"),
                Choice::new("wipe").confirm("again wipes"),
            ])
            .list([
                Choice::new("Danger").skip(),
                Choice::new("burn").confirm("again burns"),
            ])
            .focus(1);
        modal.key(press(KeyCode::Enter));
        assert_eq!(modal.line(), Some(("again burns", Tone::Warn)));
    }

    #[test]
    fn cancel_and_close_keys_close_and_replace_the_defaults() {
        let mut modal = Modal::new(1, "C").list([Choice::new("a")]);
        assert!(matches!(modal.key(press(KeyCode::Esc)), Outcome::Close));
        let mut modal = Modal::new(1, "C")
            .list([Choice::new("a")])
            .cancel_keys(&[Key::Char('n')])
            .close_keys(&[Key::Char('y')]);
        assert!(matches!(modal.key(press(KeyCode::Esc)), Outcome::Stay));
        assert!(matches!(
            modal.key(press(KeyCode::Char('n'))),
            Outcome::Close
        ));
        assert!(matches!(
            modal.key(press(KeyCode::Char('y'))),
            Outcome::Close
        ));
        let mut typing = Modal::new(1, "C")
            .field(Field::new("A"))
            .close_keys(&[Key::Char('y')]);
        assert!(
            matches!(typing.key(press(KeyCode::Char('y'))), Outcome::Close),
            "a close key wins over typing"
        );
    }

    #[test]
    fn an_alert_shows_its_lines_and_closes_on_enter_q_and_esc() {
        for code in [KeyCode::Enter, KeyCode::Char('q'), KeyCode::Esc] {
            let mut alert = Modal::alert(4, "Heads up", [Line::from("disk full")]);
            assert_eq!(alert.id(), 4);
            let shown = drawn(&mut alert, 80, 24);
            assert!(shown.contains("Heads up") && shown.contains("disk full"));
            assert!(matches!(alert.key(press(code)), Outcome::Close));
        }
        let mut alert = Modal::alert(4, "Heads up", [Line::from("disk full")]);
        assert!(matches!(
            alert.key(press(KeyCode::Char('x'))),
            Outcome::Stay
        ));
    }

    #[test]
    fn a_list_scrolls_past_its_rows() {
        let mut modal = Modal::new(1, "Long").list_rows(names(30), 5);
        let buffer = shot(&mut modal, 80, 24);
        assert_eq!(boxed(&buffer).map(|place| place.height), Some(7));
        for _ in 0..10 {
            modal.key(press(KeyCode::Down));
        }
        assert_eq!(modal.choice(0), Some(10));
        let shown = drawn(&mut modal, 80, 24);
        assert!(shown.contains("item 10") && !shown.contains("item 00"));
        modal.key(press(KeyCode::End));
        assert_eq!(modal.choice(0), Some(29));
        assert!(matches!(modal.key(press(KeyCode::Down)), Outcome::Stay));
        assert!(drawn(&mut modal, 80, 24).contains("item 29"));
        modal.key(press(KeyCode::Home));
        modal.key(press(KeyCode::PageDown));
        assert!(modal.choice(0).is_some_and(|at| at > 0));
    }

    #[test]
    fn the_box_is_as_tall_as_its_parts_and_as_wide_as_its_widest() {
        let mut modal = Modal::new(1, "Pick").list_rows(names(10), 4);
        assert_eq!(boxed(&shot(&mut modal, 60, 30)).map(|b| b.height), Some(6));
        let mut modal = modal.status("ready", Tone::Muted);
        assert_eq!(boxed(&shot(&mut modal, 60, 30)).map(|b| b.height), Some(8));
        let mut modal = Modal::new(1, "Pick").list([Choice::new("a".repeat(40))]);
        assert_eq!(boxed(&shot(&mut modal, 120, 30)).map(|b| b.width), Some(47));
        let mut modal = Modal::new(1, "P").text([Line::from("x")]);
        assert_eq!(
            boxed(&shot(&mut modal, 120, 30)).map(|b| b.width),
            Some(NARROWEST)
        );
        let mut modal = Modal::new(1, "P").status("s".repeat(60), Tone::Ink);
        assert_eq!(boxed(&shot(&mut modal, 120, 30)).map(|b| b.width), Some(64));
        let mut modal = Modal::new(1, "P").field(Field::new("Label"));
        assert_eq!(
            boxed(&shot(&mut modal, 120, 30)).map(|b| b.width),
            Some(5 + FIELD + 4)
        );
    }

    fn crowded() -> Modal {
        Modal::new(1, "A rather long title for a crowded modal")
            .text((0..5).map(|n| Line::from(format!("line {n}"))))
            .field(Field::new("Find").placeholder("type"))
            .list_rows(names(40), 30)
            .filter(1, 2)
            .status("all good", Tone::Good)
    }

    #[test]
    fn a_crowded_modal_fits_every_terminal_and_never_draws_outside_its_area() {
        for (width, height) in [(120, 40), (200, 60), (20, 6), (10, 3), (4, 2), (1, 1)] {
            let mut modal = crowded();
            let outer = Rect::new(0, 0, width + 6, height + 4);
            let area = Rect::new(3, 2, width, height);
            let buffer = shot_in(&mut modal, outer.width, outer.height, area);
            for y in 0..outer.height {
                for x in 0..outer.width {
                    if !area.contains((x, y).into()) {
                        assert_eq!(buffer[(x, y)].symbol(), " ", "{width}x{height} at {x},{y}");
                    }
                }
            }
            let found = boxed(&buffer);
            if width >= 20 {
                assert!(found.is_some(), "{width}x{height} draws its box");
            }
            if let Some(place) = found {
                assert!(area.contains(place.as_position()));
                assert!(place.right() <= area.right() && place.bottom() <= area.bottom());
            }
        }
        let shown = drawn(&mut crowded(), 120, 40);
        assert!(shown.contains("line 4") && shown.contains("all good"));
    }

    #[test]
    fn a_short_terminal_cuts_the_list_and_keeps_the_status() {
        let mut modal = crowded();
        let buffer = shot(&mut modal, 80, 16);
        let shown = dump::text(&buffer);
        assert!(shown.contains("all good"), "{shown}");
        assert!(shown.contains("Find"));
        assert_eq!(boxed(&buffer).map(|b| b.height), Some(16));
    }

    #[test]
    fn a_list_cut_by_a_short_terminal_grows_back_when_it_is_tall_again() {
        let mut modal = Modal::new(1, "Long").list_rows(names(12), 10);
        shot(&mut modal, 80, 6);
        let buffer = shot(&mut modal, 80, 30);
        assert_eq!(boxed(&buffer).map(|b| b.height), Some(12));
        assert!(dump::text(&buffer).contains("item 09"));
    }

    #[test]
    fn a_list_narrowed_by_its_filter_grows_back_when_the_query_clears() {
        let mut modal = Modal::new(1, "Go")
            .field(Field::new("Find"))
            .list(names(6))
            .filter(0, 1);
        typed(&mut modal, "03");
        assert!(!drawn(&mut modal, 80, 24).contains("item 04"));
        modal.key(press(KeyCode::Backspace));
        modal.key(press(KeyCode::Backspace));
        let shown = drawn(&mut modal, 80, 24);
        for n in 0..6 {
            assert!(shown.contains(&format!("item {n:02}")), "{shown}");
        }
    }

    #[test]
    fn the_status_line_wears_each_tone_and_an_empty_one_takes_no_room() {
        for tone in TONES {
            let mut modal = Modal::new(1, "S")
                .text([Line::from("x")])
                .status("toned", tone);
            let buffer = shot(&mut modal, 60, 20);
            let at = found(&buffer, "toned");
            assert!(
                at.is_some_and(|at| wears(&buffer, at, palette().tone(tone))),
                "{tone:?}"
            );
        }
        let mut modal = Modal::new(1, "S")
            .text([Line::from("x")])
            .status("", Tone::Bad);
        assert_eq!(modal.line(), None);
        assert_eq!(boxed(&shot(&mut modal, 60, 20)).map(|b| b.height), Some(3));
    }

    #[test]
    fn wide_titles_and_texts_are_clipped_by_cells_inside_the_box() {
        let title = "日本語のとても長いタイトルです";
        let text = "幅の広い文字がたくさん並んでいる一行のテキストです";
        for width in [24, 25, 31] {
            let mut modal = Modal::new(1, title)
                .text([Line::from(text)])
                .status("状態の行もまた長くて幅が広い文字ばかりです", Tone::Warn)
                .width(width);
            let outer = Rect::new(0, 0, 60, 12);
            let buffer = shot_in(&mut modal, 60, 12, outer);
            let place = boxed(&buffer);
            assert_eq!(place.map(|b| b.width), Some(width));
            let Some(place) = place else { continue };
            for y in 0..outer.height {
                for x in place.right()..outer.width {
                    assert_eq!(buffer[(x, y)].symbol(), " ", "{width} at {x},{y}");
                }
                for x in 0..place.left() {
                    assert_eq!(buffer[(x, y)].symbol(), " ", "{width} at {x},{y}");
                }
            }
            for y in place.top()..place.bottom() {
                let edge = buffer[(place.right() - 1, y)].symbol();
                assert!(
                    ["│", "╮", "╯"].contains(&edge),
                    "{width} edge {edge:?} at {y}"
                );
            }
        }
    }

    #[test]
    fn a_modal_wider_and_taller_than_its_area_fills_it_and_stops() {
        let mut modal = Modal::new(1, "Huge")
            .text((0..50).map(|n| Line::from(format!("row {n}"))))
            .width(500);
        let buffer = shot(&mut modal, 40, 10);
        assert_eq!(boxed(&buffer), Some(Rect::new(0, 0, 40, 10)));
        let shown = dump::text(&buffer);
        assert!(shown.contains("row 0") && !shown.contains("row 9"));
    }

    #[test]
    fn the_box_sits_a_third_down_and_centred() {
        let mut modal = Modal::new(1, "Pick").list_rows(names(3), 3).width(40);
        let place = boxed(&shot(&mut modal, 120, 40));
        assert_eq!(place, Some(Rect::new(40, 11, 40, 5)));
    }
}
