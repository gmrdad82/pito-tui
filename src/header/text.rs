use ratatui::{buffer::Buffer, layout::Rect, style::Style};
use unicode_segmentation::UnicodeSegmentation;
use unicode_width::UnicodeWidthStr;

pub(crate) const ELLIPSIS: &str = "…";

fn plain(text: &str) -> bool {
    !text.contains(char::is_control)
}

fn pieces(text: &str) -> impl Iterator<Item = &str> {
    text.split(char::is_control)
        .filter(|piece| !piece.is_empty())
}

fn cells(text: &str) -> u16 {
    if text.is_ascii() {
        return u16::try_from(text.len()).unwrap_or(u16::MAX);
    }
    let wide = text
        .graphemes(true)
        .map(|grapheme| {
            grapheme.width()
                + grapheme
                    .chars()
                    .filter(|c| matches!(c, '\u{FF9E}' | '\u{FF9F}'))
                    .count()
        })
        .fold(0usize, usize::saturating_add);
    u16::try_from(wide).unwrap_or(u16::MAX)
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

pub(crate) fn first(text: &str) -> &str {
    text.graphemes(true).next().unwrap_or("")
}

pub(crate) fn digits(number: usize, out: &mut [u8; 20]) -> &str {
    let mut n = number;
    let mut at = out.len();
    loop {
        at -= 1;
        out[at] = b'0' + (n % 10) as u8;
        n /= 10;
        if n == 0 {
            break;
        }
    }
    std::str::from_utf8(&out[at..]).unwrap_or("")
}

pub(crate) fn number_width(number: usize) -> u16 {
    let mut n = number / 10;
    let mut count = 1;
    while n > 0 {
        n /= 10;
        count += 1;
    }
    count
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

    pub(crate) fn putn(&mut self, text: &str, style: Style, room: u16) {
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
        let room = room.min(self.room());
        if width(text) <= room {
            return self.put(text, style);
        }
        if room == 0 {
            return;
        }
        self.putn(text, style, room - 1);
        self.put(ELLIPSIS, style);
    }

    pub(crate) fn fill(&mut self, symbol: &str, style: Style) {
        self.fill_to(self.right, symbol, style);
    }

    pub(crate) fn fill_to(&mut self, to: u16, symbol: &str, style: Style) {
        let to = to.min(self.right);
        while self.x < to {
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
