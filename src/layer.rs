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
}

impl Choices {
    fn new(all: Vec<Choice>, rows: u16) -> Self {
        let mut choices = Choices {
            all,
            shown: Vec::new(),
            list: List::new().keys(Keys::new()),
            rows,
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
    armed: Option<usize>,
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
        if choices.all[choice].confirm.is_some() && self.armed != Some(choice) {
            self.armed = Some(choice);
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
        if let Some(choice) = self.armed {
            let warning = self.parts.iter().find_map(|part| match part {
                Part::List(choices) => choices.all.get(choice)?.confirm.as_deref(),
                _ => None,
            });
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
                choices.rows = choices.height() - cut;
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
    use crossterm::event::KeyModifiers;

    fn press(code: KeyCode) -> KeyEvent {
        KeyEvent::new(code, KeyModifiers::NONE)
    }

    fn typed(modal: &mut Modal, text: &str) {
        for c in text.chars() {
            modal.key(press(KeyCode::Char(c)));
        }
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
}
