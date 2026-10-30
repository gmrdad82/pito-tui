use crate::footer::cut;
use ratatui::{buffer::Buffer, layout::Rect, style::Style};
use unicode_segmentation::UnicodeSegmentation;
use unicode_width::UnicodeWidthStr;

pub(crate) const ELLIPSIS: &str = "…";
pub(crate) const RULE: &str = "─";
pub(crate) const INDENT: &str = "  ";

fn plain(text: &str) -> bool {
    !text.contains(char::is_control)
}

fn pieces(text: &str) -> impl Iterator<Item = &str> {
    text.split(char::is_control)
        .filter(|piece| !piece.is_empty())
}

fn cells(text: &str) -> u16 {
    u16::try_from(text.width()).unwrap_or(u16::MAX)
}

pub(crate) fn width(text: &str) -> u16 {
    if plain(text) {
        return cells(text);
    }
    pieces(text)
        .enumerate()
        .fold(0u16, |total, (index, piece)| {
            total
                .saturating_add(u16::from(index > 0))
                .saturating_add(cells(piece))
        })
}

pub(crate) fn spans_width(spans: &[(&str, Style)]) -> u16 {
    spans
        .iter()
        .fold(0u16, |total, (text, _)| total.saturating_add(width(text)))
}

pub(crate) fn rule(buf: &mut Buffer, area: Rect, style: Style) {
    if let Some(mut pen) = Pen::new(buf, area, area.x, area.y) {
        pen.fill(RULE, style);
    }
}

pub(crate) fn flow(buf: &mut Buffer, area: Rect, text: &str, style: Style) {
    let mut rest = text.trim_start();
    for y in area.top()..area.bottom() {
        if rest.is_empty() {
            return;
        }
        let Some(mut pen) = Pen::new(buf, area, area.x, y) else {
            return;
        };
        let room = pen.room();
        if y + 1 == area.bottom() || width(rest) <= room {
            pen.clip(rest, style, room);
            return;
        }
        let mut cut = 0;
        for (at, _) in rest.match_indices(|c: char| c == ' ' || c.is_control()) {
            if width(&rest[..at]) > room {
                break;
            }
            cut = at;
        }
        if cut == 0 {
            let mut used = 0u16;
            for (at, grapheme) in rest.grapheme_indices(true) {
                used = used.saturating_add(width(grapheme));
                if used > room {
                    break;
                }
                cut = at + grapheme.len();
            }
        }
        if cut == 0 {
            return;
        }
        pen.put(&rest[..cut], style);
        rest = rest[cut..].trim_start();
    }
}

pub(crate) struct Pen<'a> {
    buf: &'a mut Buffer,
    pub(crate) x: u16,
    y: u16,
    right: u16,
}

impl<'a> Pen<'a> {
    pub(crate) fn new(buf: &'a mut Buffer, area: Rect, x: u16, y: u16) -> Option<Self> {
        let area = area.intersection(buf.area);
        if area.is_empty() || y < area.top() || y >= area.bottom() {
            return None;
        }
        Some(Pen {
            buf,
            x: x.clamp(area.left(), area.right()),
            y,
            right: area.right(),
        })
    }

    pub(crate) fn room(&self) -> u16 {
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
        if plain(text) {
            return self.draw(text, style, room);
        }
        let stop = self.x + room;
        for (index, piece) in pieces(text).enumerate() {
            if index > 0 {
                self.draw(" ", style, stop - self.x);
            }
            let start = self.x;
            self.draw(piece, style, stop - self.x);
            if self.x - start < cells(piece) {
                break;
            }
        }
    }

    fn draw(&mut self, text: &str, style: Style, room: u16) {
        if room == 0 {
            return;
        }
        let (end, _) = self
            .buf
            .set_stringn(self.x, self.y, text, usize::from(room), style);
        self.x = end;
    }

    pub(crate) fn clip(&mut self, text: &str, style: Style, room: u16) {
        if width(text) <= room.min(self.room()) {
            return self.put(text, style);
        }
        self.cut(text, style, room);
    }

    pub(crate) fn rest(&mut self, text: &str, style: Style) {
        self.clip(text, style, self.room());
    }

    pub(crate) fn indented(&mut self, text: &str, style: Style) {
        self.put(INDENT, style);
        self.rest(text, style);
    }

    pub(crate) fn spans(&mut self, spans: &[(&str, Style)], room: u16) {
        let room = room.min(self.room());
        if spans_width(spans) <= room {
            for (text, style) in spans {
                self.put(text, *style);
            }
            return;
        }
        self.cut_spans(spans, room);
    }

    pub(crate) fn cut(&mut self, text: &str, style: Style, room: u16) {
        self.cut_spans(&[(text, style)], room.min(self.room()));
    }

    fn cut_spans(&mut self, spans: &[(&str, Style)], room: u16) {
        if room == 0 {
            return;
        }
        let glyphs = spans.iter().flat_map(|(text, _)| cut::graphemes(text));
        let Some(keep) = cut::keep(glyphs, usize::from(room)) else {
            for (text, style) in spans {
                self.put(text, *style);
            }
            return;
        };
        let last = cut::kept(spans, keep, |text, style| self.put(text, style));
        self.put(ELLIPSIS, last);
    }

    pub(crate) fn fill(&mut self, symbol: &str, style: Style) {
        while self.x < self.right {
            self.buf[(self.x, self.y)]
                .set_symbol(symbol)
                .set_style(style);
            self.x = self.x.saturating_add(1);
        }
    }

    pub(crate) fn skip(&mut self, cells: u16) {
        self.x = self.x.saturating_add(cells).min(self.right);
    }
}
