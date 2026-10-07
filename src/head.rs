use ratatui::{buffer::Buffer, layout::Rect, style::Style};
use unicode_segmentation::UnicodeSegmentation;

use crate::label::{Label, Span};
use crate::text::cells;

const RULE: &str = "─";
const ELLIPSIS: &str = "…";
const SIDE_MIN: u16 = 6;
const GROUP_GAP: u16 = 2;
const SECTION_GAP: u16 = 0;
const TAIL_GAP: u16 = 2;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Row {
    Title,
    Groups,
    Caption,
    Sections,
    Facts,
    Crumbs,
    Blank,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
#[non_exhaustive]
pub enum Numbers {
    #[default]
    Across,
    InGroup,
    Off,
}

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub struct Head {
    pub rows: Vec<Row>,
    pub numbers: Numbers,
    pub lone: bool,
}

impl Head {
    pub fn new(rows: impl IntoIterator<Item = Row>) -> Self {
        Head {
            rows: rows.into_iter().collect(),
            numbers: Numbers::Across,
            lone: false,
        }
    }

    pub fn numbers(mut self, numbers: Numbers) -> Self {
        self.numbers = numbers;
        self
    }

    pub fn lone(mut self, lone: bool) -> Self {
        self.lone = lone;
        self
    }
}

impl Default for Head {
    fn default() -> Self {
        Head::new([
            Row::Title,
            Row::Groups,
            Row::Sections,
            Row::Facts,
            Row::Crumbs,
        ])
    }
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
#[non_exhaustive]
pub enum Focus {
    #[default]
    Content,
    Groups,
    Sections,
    Slot,
}

fn width(text: &str) -> u16 {
    u16::try_from(cells(text)).unwrap_or(u16::MAX)
}

fn spans_width<T: AsRef<str>>(spans: &[(T, Style)]) -> u16 {
    spans.iter().fold(0u16, |total, (text, _)| {
        total.saturating_add(width(text.as_ref()))
    })
}

fn digits(number: usize) -> String {
    let shown = if number == 10 { 0 } else { number };
    shown.to_string()
}

pub(crate) struct Pen<'a> {
    buf: &'a mut Buffer,
    pub(crate) x: u16,
    y: u16,
    right: u16,
}

impl<'a> Pen<'a> {
    pub(crate) fn new(buf: &'a mut Buffer, line: Rect, x: u16) -> Option<Self> {
        let area = line.intersection(buf.area);
        if area.is_empty() {
            return None;
        }
        Some(Pen {
            buf,
            x: x.clamp(area.left(), area.right()),
            y: area.y,
            right: area.right(),
        })
    }

    fn room(&self) -> u16 {
        self.right.saturating_sub(self.x)
    }

    pub(crate) fn put(&mut self, text: &str, style: Style) {
        self.putn(text, style, self.room());
    }

    fn putn(&mut self, text: &str, style: Style, room: u16) {
        let room = room.min(self.room());
        if room == 0 || text.is_empty() {
            return;
        }
        let (end, _) = self
            .buf
            .set_stringn(self.x, self.y, text, usize::from(room), style);
        self.x = end;
    }

    pub(crate) fn fill_to(&mut self, to: u16, symbol: &str, style: Style) {
        let to = to.min(self.right);
        while self.x < to {
            self.buf[(self.x, self.y)]
                .set_symbol(symbol)
                .set_style(style);
            self.x += 1;
        }
    }

    pub(crate) fn fill(&mut self, symbol: &str, style: Style) {
        self.fill_to(self.right, symbol, style);
    }

    fn spans<T: AsRef<str>>(&mut self, spans: &[(T, Style)], base: Style, room: u16) {
        let room = room.min(self.room());
        if spans_width(spans) <= room {
            for (text, style) in spans {
                self.put(text.as_ref(), base.patch(*style));
            }
            return;
        }
        if room == 0 {
            return;
        }
        let stop = self.x + room - 1;
        let mut last = base;
        for (text, style) in spans {
            let style = base.patch(*style);
            if self.x >= stop {
                break;
            }
            last = style;
            self.putn(text.as_ref(), style, stop - self.x);
        }
        self.x = self.x.min(stop);
        self.put(ELLIPSIS, last);
    }
}

fn label<T: AsRef<str>>(pen: &mut Pen, spans: &[(T, Style)], base: Style, room: u16) {
    if spans.is_empty() {
        return;
    }
    let first = base.patch(spans[0].1);
    let last = base.patch(spans[spans.len() - 1].1);
    pen.put(" ", first);
    pen.spans(spans, base, room);
    pen.put(" ", last);
}

pub(crate) struct Rule<'a> {
    pub(crate) rule: Style,
    pub(crate) base: Style,
    pub(crate) middle: &'a [Span],
    pub(crate) left: &'a [(String, Style)],
    pub(crate) right: &'a [(String, Style)],
}

impl Rule<'_> {
    pub(crate) fn draw(&self, buf: &mut Buffer, line: Rect) {
        let Some(mut pen) = Pen::new(buf, line, line.x) else {
            return;
        };
        let wide = spans_width(self.middle).saturating_add(2).min(line.width);
        let start = line.x.saturating_add((line.width - wide) / 2);
        let side = start.saturating_sub(line.x).saturating_sub(2);
        let sides = side >= SIDE_MIN;
        let present = |parts: &[(String, Style)]| spans_width(parts) > 0;
        if sides && present(self.left) {
            pen.fill_to(line.x.saturating_add(1), RULE, self.rule);
            label(&mut pen, self.left, Style::new(), side - 2);
        }
        pen.fill_to(start, RULE, self.rule);
        if spans_width(self.middle) > 0 {
            label(&mut pen, self.middle, self.base, wide.saturating_sub(2));
        }
        if sides && present(self.right) {
            let wide = spans_width(self.right).min(side - 2).saturating_add(2);
            pen.fill_to(line.right().saturating_sub(1 + wide), RULE, self.rule);
            label(&mut pen, self.right, Style::new(), wide - 2);
        }
        pen.fill(RULE, self.rule);
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Kind {
    Groups,
    Sections,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Form {
    Full,
    Short,
    Lone,
    Bare,
}

const FORMS: [Form; 4] = [Form::Full, Form::Short, Form::Lone, Form::Bare];

#[derive(Debug, Clone, Copy)]
pub(crate) struct Item<'a> {
    pub(crate) label: &'a Label,
    pub(crate) number: Option<usize>,
    pub(crate) on: bool,
}

fn first(spans: &[Span]) -> Option<(&str, Style)> {
    let (text, style) = spans.iter().find(|(text, _)| !text.is_empty())?;
    Some((text.graphemes(true).next()?, *style))
}

enum Body<'a> {
    Spans(&'a [Span]),
    Glyph(Option<(&'a str, Style)>),
}

impl Body<'_> {
    fn width(&self) -> u16 {
        match self {
            Body::Spans(spans) => spans_width(spans),
            Body::Glyph(glyph) => glyph.map_or(0, |(text, _)| width(text)),
        }
    }
}

impl<'a> Item<'a> {
    fn body(&self, kind: Kind, form: Form) -> Body<'a> {
        let short = self.label.short_spans_or_full();
        let initial = || Body::Glyph(first(short));
        let numbered = self.number.is_some() && kind == Kind::Sections;
        match form {
            Form::Full => Body::Spans(self.label.full()),
            Form::Short => Body::Spans(short),
            Form::Lone if self.on => Body::Spans(short),
            Form::Lone | Form::Bare if numbered => Body::Glyph(None),
            Form::Lone | Form::Bare => initial(),
        }
    }

    fn width(&self, kind: Kind, form: Form) -> u16 {
        let body = self.body(kind, form).width();
        let number = self.number.map_or(0, |number| {
            width(&digits(number)).saturating_add(u16::from(body > 0))
        });
        number.saturating_add(2).saturating_add(body)
    }

    fn draw(&self, pen: &mut Pen, kind: Kind, form: Form, style: Style) {
        let body = self.body(kind, form);
        pen.put(" ", style);
        if let Some(number) = self.number {
            pen.put(&digits(number), style);
            if body.width() > 0 {
                pen.put(" ", style);
            }
        }
        match body {
            Body::Spans(spans) => {
                for (text, span) in spans {
                    pen.put(text, style.patch(*span));
                }
            }
            Body::Glyph(Some((text, span))) => pen.put(text, style.patch(span)),
            Body::Glyph(None) => {}
        }
        pen.put(" ", style);
    }
}

pub(crate) struct Labels<'a> {
    pub(crate) items: Vec<Item<'a>>,
    pub(crate) kind: Kind,
    pub(crate) on: Style,
    pub(crate) off: Style,
    pub(crate) tail: &'a [(String, Style)],
    pub(crate) tail_style: Style,
}

impl Labels<'_> {
    fn gap(&self) -> u16 {
        match self.kind {
            Kind::Groups => GROUP_GAP,
            Kind::Sections => SECTION_GAP,
        }
    }

    fn total(&self, form: Form, tail: bool) -> u16 {
        let gap = self.gap();
        let labels = self
            .items
            .iter()
            .enumerate()
            .fold(0u16, |total, (index, item)| {
                let gap = if index > 0 { gap } else { 0 };
                total
                    .saturating_add(gap)
                    .saturating_add(item.width(self.kind, form))
            });
        let tail = if tail { spans_width(self.tail) } else { 0 };
        if tail == 0 {
            return labels;
        }
        let gap = if labels > 0 { TAIL_GAP } else { 0 };
        labels.saturating_add(gap).saturating_add(tail)
    }

    fn fit(&self, line: Rect) -> (Form, u16) {
        for form in FORMS {
            let total = self.total(form, true);
            if total <= line.width {
                return (form, line.x + (line.width - total) / 2);
            }
        }
        (Form::Bare, line.x)
    }

    pub(crate) fn draw(&self, buf: &mut Buffer, line: Rect) {
        let (form, start) = self.fit(line);
        let Some(mut pen) = Pen::new(buf, line, start) else {
            return;
        };
        let gap = self.gap();
        for (index, item) in self.items.iter().enumerate() {
            if index > 0 {
                pen.x = pen.x.saturating_add(gap).min(pen.right);
            }
            let style = if item.on { self.on } else { self.off };
            item.draw(&mut pen, self.kind, form, style);
        }
        if spans_width(self.tail) == 0 {
            return;
        }
        if !self.items.is_empty() {
            pen.x = pen.x.saturating_add(TAIL_GAP).min(pen.right);
        }
        let room = pen.room();
        pen.spans(self.tail, self.tail_style, room);
    }

    pub(crate) fn hit(&self, line: Rect, column: u16) -> Option<usize> {
        if column < line.x || column >= line.right() {
            return None;
        }
        let (form, start) = self.fit(line);
        let mut x = start;
        for (index, item) in self.items.iter().enumerate() {
            if index > 0 {
                x = x.saturating_add(self.gap());
            }
            let end = x.saturating_add(item.width(self.kind, form));
            if (x..end).contains(&column) {
                return Some(index);
            }
            x = end;
        }
        None
    }
}

pub(crate) fn blank(buf: &mut Buffer, line: Rect, style: Style) {
    if let Some(mut pen) = Pen::new(buf, line, line.x) {
        pen.fill(" ", style);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ratatui::style::Modifier;

    fn row(buf: &Buffer, y: u16) -> String {
        (0..buf.area.width)
            .map(|x| buf[(x, y)].symbol().to_string())
            .collect()
    }

    #[test]
    fn numbers_restart_in_a_group_and_a_lone_tab_shows_none_with_its_slot_after() {
        let home = Label::new("Home");
        let next = Label::new("Next");
        let accel = Label::spans([
            ("M", Style::new().add_modifier(Modifier::UNDERLINED)),
            ("ail", Style::new()),
        ]);
        let labels = Labels {
            items: vec![
                Item {
                    label: &home,
                    number: Some(1),
                    on: true,
                },
                Item {
                    label: &next,
                    number: Some(2),
                    on: false,
                },
            ],
            kind: Kind::Sections,
            on: Style::new().add_modifier(Modifier::BOLD),
            off: Style::new(),
            tail: &[("‹ all ›".to_string(), Style::new())],
            tail_style: Style::new(),
        };
        let mut buf = Buffer::empty(Rect::new(0, 0, 30, 1));
        labels.draw(&mut buf, Rect::new(0, 0, 30, 1));
        assert_eq!(row(&buf, 0), "   1 Home  2 Next   ‹ all ›   ");
        assert_eq!(labels.hit(Rect::new(0, 0, 30, 1), 10), Some(1));
        let lone = Labels {
            items: vec![Item {
                label: &accel,
                number: None,
                on: true,
            }],
            kind: Kind::Sections,
            on: Style::new(),
            off: Style::new(),
            tail: &[],
            tail_style: Style::new(),
        };
        let mut buf = Buffer::empty(Rect::new(0, 0, 10, 1));
        lone.draw(&mut buf, Rect::new(0, 0, 10, 1));
        assert_eq!(row(&buf, 0), "   Mail   ");
        assert!(buf[(3, 0)].modifier.contains(Modifier::UNDERLINED));
        assert!(!buf[(4, 0)].modifier.contains(Modifier::UNDERLINED));
    }
}
