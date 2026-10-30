use std::fmt;

use ratatui::{
    buffer::Buffer,
    layout::{Position, Rect},
    style::{Modifier, Style},
    widgets::Widget,
};
use unicode_segmentation::UnicodeSegmentation;

use crate::footer::key::Key;
use crate::footer::styles::{Styles, Tone};
use crate::footer::text::{self, ELLIPSIS, Pen};

const MASK: &str = "•";
const SHARE: usize = 4;
const GAP: u16 = 1;
const CARET: &str = " ";

#[derive(Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Edit {
    Pass,
    Held,
    Changed,
    Submit(String),
    Cancel,
}

impl fmt::Debug for Edit {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Edit::Pass => f.write_str("Pass"),
            Edit::Held => f.write_str("Held"),
            Edit::Changed => f.write_str("Changed"),
            Edit::Submit(value) => write!(f, "Submit(<{} bytes>)", value.len()),
            Edit::Cancel => f.write_str("Cancel"),
        }
    }
}

#[derive(Clone, PartialEq, Eq)]
pub struct Input {
    value: String,
    cursor: usize,
    masked: bool,
    edited: bool,
    limit: usize,
}

impl Default for Input {
    fn default() -> Self {
        Input {
            value: String::new(),
            cursor: 0,
            masked: false,
            edited: false,
            limit: usize::MAX,
        }
    }
}

impl fmt::Debug for Input {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let mut out = f.debug_struct("Input");
        if self.masked {
            out.field("value", &format_args!("<masked>"));
        } else {
            out.field("value", &self.value);
        }
        out.field("cursor", &self.cursor())
            .field("masked", &self.masked)
            .finish()
    }
}

impl Input {
    pub fn new() -> Self {
        Input::default()
    }

    pub fn masked(mut self, masked: bool) -> Self {
        self.masked = masked;
        self
    }

    pub fn limit(mut self, chars: usize) -> Self {
        self.limit = chars;
        let kept = self.value.char_indices().nth(chars).map(|(at, _)| at);
        if let Some(at) = kept {
            self.value.truncate(at);
            self.cursor = self.cursor.min(at);
            self.snap();
        }
        self
    }

    pub fn with(mut self, value: &str) -> Self {
        self.set(value);
        self
    }

    pub fn edited(&self) -> bool {
        self.edited
    }

    pub fn value(&self) -> &str {
        &self.value
    }

    pub fn is_masked(&self) -> bool {
        self.masked
    }

    pub fn is_empty(&self) -> bool {
        self.value.is_empty()
    }

    pub fn cursor(&self) -> usize {
        self.value
            .grapheme_indices(true)
            .take_while(|(at, _)| *at < self.cursor)
            .count()
    }

    pub fn set(&mut self, value: &str) {
        self.value = clean(value);
        if let Some((at, _)) = self.value.char_indices().nth(self.limit) {
            self.value.truncate(at);
        }
        self.cursor = self.value.len();
        self.edited = false;
    }

    pub fn clear(&mut self) {
        self.value.clear();
        self.cursor = 0;
        self.edited = false;
    }

    pub fn key(&mut self, key: Key) -> Edit {
        match key {
            Key::Enter => Edit::Submit(self.value.clone()),
            Key::Esc => Edit::Cancel,
            Key::Left => self.go(self.prev()),
            Key::Right => self.go(self.next()),
            Key::Home | Key::Ctrl('a') => self.go(0),
            Key::End | Key::Ctrl('e') => self.go(self.value.len()),
            Key::Backspace => self.cut(self.prev(), self.cursor),
            Key::Delete => self.cut(self.cursor, self.next()),
            Key::Ctrl('u') => self.cut(0, self.cursor),
            Key::Ctrl('w') => self.cut(self.word(), self.cursor),
            Key::Char(c) if !c.is_control() => self.insert(c.encode_utf8(&mut [0; 4])),
            _ => Edit::Pass,
        }
    }

    pub fn paste(&mut self, text: &str) -> Edit {
        self.insert(&clean(text))
    }

    fn go(&mut self, at: usize) -> Edit {
        self.cursor = at;
        Edit::Held
    }

    fn cut(&mut self, from: usize, to: usize) -> Edit {
        if from >= to {
            return Edit::Held;
        }
        self.value.replace_range(from..to, "");
        self.cursor = from;
        self.snap();
        self.edited = true;
        Edit::Changed
    }

    fn insert(&mut self, text: &str) -> Edit {
        let room = self.limit.saturating_sub(self.value.chars().count());
        let text = match text.char_indices().nth(room) {
            Some((at, _)) => &text[..at],
            None => text,
        };
        if text.is_empty() {
            return Edit::Held;
        }
        self.value.insert_str(self.cursor, text);
        self.cursor += text.len();
        self.snap();
        self.edited = true;
        Edit::Changed
    }

    fn snap(&mut self) {
        self.cursor = self
            .value
            .grapheme_indices(true)
            .map(|(at, _)| at)
            .chain([self.value.len()])
            .find(|at| *at >= self.cursor)
            .unwrap_or(self.value.len());
    }

    fn prev(&self) -> usize {
        self.value
            .grapheme_indices(true)
            .rev()
            .find(|(at, _)| *at < self.cursor)
            .map_or(0, |(at, _)| at)
    }

    fn next(&self) -> usize {
        self.value
            .grapheme_indices(true)
            .map(|(at, grapheme)| at + grapheme.len())
            .find(|end| *end > self.cursor)
            .unwrap_or(self.value.len())
    }

    fn word(&self) -> usize {
        let mut at = self.cursor;
        let mut word = false;
        for (start, grapheme) in self
            .value
            .grapheme_indices(true)
            .rev()
            .skip_while(|(start, _)| *start >= self.cursor)
        {
            let space = grapheme.chars().all(char::is_whitespace);
            if space && word {
                break;
            }
            word |= !space;
            at = start;
        }
        at
    }

    fn units<'a>(
        &'a self,
        mask: &'a str,
        shape: Shape,
    ) -> impl Iterator<Item = (&'a str, bool)> + Clone {
        self.value
            .graphemes(true)
            .enumerate()
            .map(move |(index, grapheme)| shape.unit(index, grapheme, mask))
    }

    fn units_back<'a>(
        &'a self,
        mask: &'a str,
        shape: Shape,
    ) -> impl Iterator<Item = (usize, &'a str)> {
        self.value
            .graphemes(true)
            .rev()
            .scan(shape.count, move |index, grapheme| {
                *index -= 1;
                Some((*index, shape.unit(*index, grapheme, mask).0))
            })
    }

    fn shape(&self, reveal: usize) -> Shape {
        let count = self.value.graphemes(true).count();
        let open =
            self.masked && !self.edited && reveal > 0 && count >= reveal.saturating_mul(SHARE);
        Shape {
            count,
            masked: self.masked,
            reveal: if open { reveal } else { 0 },
        }
    }
}

#[derive(Debug, Clone, Copy)]
struct Shape {
    count: usize,
    masked: bool,
    reveal: usize,
}

impl Shape {
    fn unit<'a>(&self, index: usize, grapheme: &'a str, mask: &'a str) -> (&'a str, bool) {
        let shown = !self.masked
            || (self.reveal > 0 && (index < self.reveal || index >= self.count - self.reveal));
        if shown {
            (grapheme, false)
        } else {
            (mask, true)
        }
    }
}

fn clean(text: &str) -> String {
    let mut out = String::with_capacity(text.len());
    for line in text.split(['\n', '\r']).filter(|line| !line.is_empty()) {
        if !out.is_empty() {
            out.push(' ');
        }
        for c in line.chars() {
            if c == '\t' {
                out.push(' ');
            } else if !c.is_control() {
                out.push(c);
            }
        }
    }
    out
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct InputBar<'a> {
    label: &'a str,
    input: &'a Input,
    placeholder: &'a str,
    hint: Option<&'a str>,
    mask: &'a str,
    rule: bool,
    caret: bool,
    reveal: usize,
    tone: Tone,
    styles: Option<Styles>,
}

struct Field {
    line: Rect,
    x: u16,
    start: usize,
    stop: usize,
    count: usize,
    cursor: usize,
    shape: Shape,
}

impl<'a> InputBar<'a> {
    pub fn new(label: &'a str, input: &'a Input) -> Self {
        InputBar {
            label,
            input,
            placeholder: "",
            hint: None,
            mask: MASK,
            rule: true,
            caret: true,
            reveal: 0,
            tone: Tone::Ink,
            styles: None,
        }
    }

    pub fn placeholder(mut self, placeholder: &'a str) -> Self {
        self.placeholder = placeholder;
        self
    }

    pub fn hint(mut self, hint: &'a str) -> Self {
        self.hint = Some(hint);
        self
    }

    pub fn mask(mut self, mask: &'a str) -> Self {
        self.mask = mask;
        self
    }

    pub fn rule(mut self, rule: bool) -> Self {
        self.rule = rule;
        self
    }

    pub fn caret(mut self, caret: bool) -> Self {
        self.caret = caret;
        self
    }

    pub fn reveal_ends(mut self, chars: usize) -> Self {
        self.reveal = chars;
        self
    }

    pub fn tone(mut self, tone: Tone) -> Self {
        self.tone = tone;
        self
    }

    pub fn styles(mut self, styles: Styles) -> Self {
        self.styles = Some(styles);
        self
    }

    pub(crate) fn styled(mut self, fallback: Styles) -> Self {
        self.styles = Some(self.styles.unwrap_or(fallback));
        self
    }

    fn look(&self) -> Styles {
        self.styles.unwrap_or_default()
    }

    pub fn height(&self) -> u16 {
        1 + u16::from(self.rule) + u16::from(self.hint.is_some())
    }

    pub fn cursor(&self, area: Rect) -> Option<Position> {
        let field = self.field(area)?;
        let before = self.span(&field, field.start, field.cursor.max(field.start));
        let x = field
            .x
            .saturating_add(u16::from(field.start > 0))
            .saturating_add(before);
        (x < field.line.right()).then_some(Position::new(x, field.line.y))
    }

    fn label_width(&self, width: u16) -> u16 {
        text::width(self.label).min(width.saturating_sub(GAP) / 2)
    }

    fn span(&self, field: &Field, from: usize, to: usize) -> u16 {
        self.input
            .units(self.mask, field.shape)
            .skip(from)
            .take(to.saturating_sub(from))
            .fold(0u16, |total, (unit, _)| {
                total.saturating_add(text::width(unit))
            })
    }

    fn field(&self, area: Rect) -> Option<Field> {
        let y = area.y.checked_add(u16::from(self.rule))?;
        if area.is_empty() || y >= area.bottom() {
            return None;
        }
        let line = Rect::new(area.x, y, area.width, 1);
        let label = self.label_width(line.width);
        let x = if label > 0 {
            line.x.saturating_add(label).saturating_add(GAP)
        } else {
            line.x
        };
        let room = line.right().saturating_sub(x);
        let shape = self.input.shape(self.reveal);
        let count = shape.count;
        let cursor = self.input.cursor();
        let mut field = Field {
            line,
            x,
            start: 0,
            stop: count,
            count,
            cursor,
            shape,
        };
        let mut total = 0u16;
        let mut caret = 1;
        for (index, (unit, _)) in self.input.units(self.mask, shape).enumerate() {
            let wide = text::width(unit);
            total = total.saturating_add(wide);
            if index == cursor {
                caret = wide;
            }
        }
        if total.saturating_add(u16::from(cursor == count)) <= room {
            return Some(field);
        }
        let tail = u16::from(cursor + 1 < count);
        let need = caret.saturating_add(tail);
        let mut behind = 0u16;
        let mut start = cursor;
        for (index, unit) in self
            .input
            .units_back(self.mask, shape)
            .skip(count - cursor.min(count))
        {
            behind = behind.saturating_add(text::width(unit));
            if behind.saturating_add(need) > room {
                break;
            }
            let lead = u16::from(index > 0);
            if behind.saturating_add(need).saturating_add(lead) <= room {
                start = index;
            }
        }
        let lead = u16::from(start > 0);
        let mut used = self.span(&field, start, (cursor + 1).min(count));
        let mut stop = (cursor + 1).min(count);
        for (unit, _) in self.input.units(self.mask, shape).skip(stop) {
            let next = used.saturating_add(text::width(unit));
            let more = u16::from(stop + 1 < count);
            if lead.saturating_add(next).saturating_add(more) > room {
                break;
            }
            used = next;
            stop += 1;
        }
        field.start = start;
        field.stop = stop;
        Some(field)
    }

    fn draw_field(&self, buf: &mut Buffer, field: &Field) -> Option<Position> {
        let mut pen = Pen::new(buf, field.line, field.x, field.line.y)?;
        let styles = self.look();
        let ink = styles.ink;
        let muted = styles.muted;
        let caret = |style: Style| {
            if self.caret {
                style.add_modifier(Modifier::REVERSED)
            } else {
                style
            }
        };
        if field.count == 0 {
            let mut graphemes = self.placeholder.graphemes(true);
            match graphemes.next() {
                Some(first) => {
                    pen.put(first, caret(muted));
                    pen.rest(graphemes.as_str(), muted);
                }
                None => pen.put(CARET, caret(ink)),
            }
            return Some(Position::new(pen.x, field.line.y));
        }
        if field.start > 0 {
            pen.put(ELLIPSIS, muted);
        }
        for (index, (unit, hidden)) in self
            .input
            .units(self.mask, field.shape)
            .enumerate()
            .skip(field.start)
            .take(field.stop - field.start)
        {
            let style = if hidden { muted } else { ink };
            if index == field.cursor {
                pen.put(unit, caret(style));
            } else {
                pen.put(unit, style);
            }
        }
        if field.stop < field.count {
            pen.put(ELLIPSIS, muted);
        } else if field.cursor == field.count {
            pen.put(CARET, caret(ink));
        }
        Some(Position::new(pen.x, field.line.y))
    }
}

impl Widget for InputBar<'_> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        (&self).render(area, buf);
    }
}

impl Widget for &InputBar<'_> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        self.paint(area, buf);
    }
}

impl InputBar<'_> {
    pub(crate) fn paint(&self, area: Rect, buf: &mut Buffer) -> Option<Position> {
        if area.is_empty() {
            return None;
        }
        let styles = self.look();
        if self.rule {
            text::rule(buf, area, styles.rule);
        }
        let field = self.field(area)?;
        if let Some(mut pen) = Pen::new(buf, field.line, field.line.x, field.line.y) {
            let style = self.tone.style(&styles).add_modifier(Modifier::BOLD);
            pen.clip(self.label, style, self.label_width(field.line.width));
        }
        let end = self.draw_field(buf, &field);
        let y = field.line.y.saturating_add(1);
        if let Some(hint) = self.hint
            && let Some(mut pen) = Pen::new(buf, area, area.x, y)
        {
            pen.indented(hint, styles.muted);
            return Some(Position::new(pen.x, y));
        }
        end
    }
}
