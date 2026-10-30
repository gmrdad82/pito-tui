use std::fmt::{self, Write};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct Bar<'a> {
    pub(crate) track: &'a str,
    pub(crate) thumb: &'a str,
    pub(crate) halves: Option<(&'a str, &'a str)>,
}

impl<'a> Bar<'a> {
    pub const LINE: Bar<'static> = Bar::new("│", "┃").halves("╽", "╿");

    pub const fn new(track: &'a str, thumb: &'a str) -> Self {
        Bar {
            track,
            thumb,
            halves: None,
        }
    }

    pub const fn halves(mut self, top: &'a str, bottom: &'a str) -> Self {
        self.halves = Some((top, bottom));
        self
    }

    pub(crate) fn glyph(&self, upper: bool, lower: bool) -> Option<&'a str> {
        match (upper, lower, self.halves) {
            (true, true, _) => Some(self.thumb),
            (false, false, _) => None,
            (false, true, Some((top, _))) => Some(top),
            (true, false, Some((_, bottom))) => Some(bottom),
            (_, _, None) => Some(self.thumb),
        }
    }
}

impl Default for Bar<'_> {
    fn default() -> Self {
        Bar::LINE
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum Place {
    #[default]
    Line,
    Foot,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct Count<'a> {
    pub(crate) to: &'a str,
    pub(crate) of: &'a str,
    pub(crate) group: &'a str,
    pub(crate) place: Place,
}

impl<'a> Count<'a> {
    pub const fn new(to: &'a str, of: &'a str) -> Self {
        Count {
            to,
            of,
            group: "",
            place: Place::Line,
        }
    }

    pub const fn group(mut self, separator: &'a str) -> Self {
        self.group = separator;
        self
    }

    pub const fn place(mut self, place: Place) -> Self {
        self.place = place;
        self
    }

    pub fn label(&self, first: usize, last: usize, total: usize) -> impl fmt::Display + use<'a> {
        Label {
            count: *self,
            first,
            last,
            total,
        }
    }
}

struct Label<'a> {
    count: Count<'a>,
    first: usize,
    last: usize,
    total: usize,
}

impl Label<'_> {
    fn number(&self, f: &mut fmt::Formatter<'_>, value: usize) -> fmt::Result {
        let mut digits = [0u8; 20];
        let mut at = digits.len();
        let mut rest = value;
        loop {
            at -= 1;
            digits[at] = b'0' + (rest % 10) as u8;
            rest /= 10;
            if rest == 0 {
                break;
            }
        }
        let digits = &digits[at..];
        let lead = match digits.len() % 3 {
            0 => 3,
            lead => lead,
        };
        for (n, &digit) in digits.iter().enumerate() {
            if n >= lead && (n - lead).is_multiple_of(3) {
                f.write_str(self.count.group)?;
            }
            f.write_char(char::from(digit))?;
        }
        Ok(())
    }
}

impl fmt::Display for Label<'_> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.number(f, self.first)?;
        if self.last != self.first {
            f.write_str(self.count.to)?;
            self.number(f, self.last)?;
        }
        f.write_str(self.count.of)?;
        self.number(f, self.total)
    }
}

pub(crate) struct Stack {
    bytes: [u8; 128],
    len: usize,
}

impl Stack {
    pub(crate) const fn new() -> Self {
        Stack {
            bytes: [0; 128],
            len: 0,
        }
    }

    pub(crate) fn text(&self) -> &str {
        std::str::from_utf8(&self.bytes[..self.len]).unwrap_or_default()
    }
}

impl Write for Stack {
    fn write_str(&mut self, text: &str) -> fmt::Result {
        for c in text.chars() {
            let end = self.len + c.len_utf8();
            if end > self.bytes.len() {
                return Err(fmt::Error);
            }
            c.encode_utf8(&mut self.bytes[self.len..end]);
            self.len = end;
        }
        Ok(())
    }
}

fn share(value: usize, times: usize, over: usize) -> usize {
    if over == 0 {
        return 0;
    }
    let exact = (value as u128 * times as u128 * 2 + over as u128) / (over as u128 * 2);
    usize::try_from(exact).unwrap_or(usize::MAX)
}

pub(crate) fn thumb(top: usize, lines: usize, height: usize, unit: usize) -> (usize, usize) {
    let track = height.saturating_mul(unit);
    if lines <= height || track == 0 {
        return (0, track);
    }
    let most = track.saturating_sub(1).max(unit).min(track);
    let size = share(track, height, lines).clamp(unit.min(track), most);
    let room = track - size;
    let end = lines - height;
    let top = top.min(end);
    let mut start = share(top, room, end);
    if room >= 2 && top > 0 && top < end {
        start = start.clamp(1, room - 1);
    }
    (start, size)
}
