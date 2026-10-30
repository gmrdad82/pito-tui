use std::iter;

use ratatui::{buffer::Buffer, layout::Rect, style::Style};
use unicode_segmentation::UnicodeSegmentation;
use unicode_width::UnicodeWidthStr;

pub(crate) const ELLIPSIS: &str = "…";

fn glyph(grapheme: &str) -> Option<(&str, usize)> {
    if grapheme.contains(char::is_control) {
        return grapheme.contains(['\t', '\n', '\r']).then_some((" ", 1));
    }
    Some((grapheme, grapheme.width()))
}

pub(crate) fn width(text: &str) -> usize {
    text.graphemes(true)
        .filter_map(glyph)
        .fold(0usize, |total, (_, cells)| total.saturating_add(cells))
}

#[derive(Clone, Copy, PartialEq, Eq)]
enum Kind {
    Space,
    Joint,
    Wide,
    Word,
}

fn kind(grapheme: &str, cells: usize) -> Kind {
    if grapheme.chars().all(char::is_whitespace) {
        Kind::Space
    } else if matches!(grapheme, "/" | "-" | "_") {
        Kind::Joint
    } else if cells > 1 {
        Kind::Wide
    } else {
        Kind::Word
    }
}

#[derive(Clone, Copy, Default)]
struct Piece {
    seen: bool,
    letters: bool,
    digits: bool,
    other: bool,
    wide: bool,
}

impl Piece {
    fn add(&mut self, grapheme: &str, cells: usize) {
        self.seen = true;
        self.wide |= cells > 1;
        for c in grapheme.chars() {
            self.letters |= c.is_alphanumeric();
            self.digits |= c.is_numeric();
            self.other |= !(c.is_numeric() || matches!(c, '.' | ',' | '+' | '-' | '_'));
        }
    }

    fn number(self) -> bool {
        self.digits && !self.other
    }

    fn word(self) -> bool {
        (self.letters || self.wide) && !self.number()
    }

    fn separator(self) -> bool {
        self.seen && !self.letters && !self.wide
    }
}

#[derive(Clone, Copy, Default)]
struct Spot {
    count: usize,
    cells: usize,
}

struct Scan {
    limit: usize,
    count: usize,
    used: usize,
    prev: Option<Kind>,
    token: Piece,
    piece: Piece,
    joint: bool,
    last: Option<Spot>,
    after: bool,
    ended: bool,
    parted: bool,
    word: Option<Spot>,
    part: Option<Spot>,
}

impl Scan {
    fn new(limit: usize) -> Self {
        Scan {
            limit,
            count: 0,
            used: 0,
            prev: None,
            token: Piece::default(),
            piece: Piece::default(),
            joint: false,
            last: None,
            after: false,
            ended: false,
            parted: false,
            word: None,
            part: None,
        }
    }

    fn mark(&mut self, spot: Spot) {
        if !self.parted && spot.cells <= self.limit {
            self.word = Some(spot);
        }
    }

    fn end(&mut self, spot: Spot) {
        if self.token.separator() {
            if let Some(last) = self
                .last
                .filter(|last| self.after && last.cells <= self.limit)
            {
                self.part = Some(last);
            }
            self.parted |= self.ended;
            self.after = false;
        } else if self.token.word() {
            self.mark(spot);
            self.last = Some(spot);
            self.after = true;
        } else {
            self.last = None;
            self.after = false;
        }
        self.ended = true;
    }

    fn open(&self) -> bool {
        self.after
            && !self.token.letters
            && !self.token.wide
            && self.last.is_some_and(|last| last.cells <= self.limit)
    }

    fn feed(&mut self, grapheme: &str, cells: usize) {
        let kind = kind(grapheme, cells);
        let here = Spot {
            count: self.count,
            cells: self.used,
        };
        match (self.prev, kind) {
            (Some(prev), Kind::Space) if prev != Kind::Space => self.end(here),
            (Some(Kind::Joint), _) if self.joint => self.mark(here),
            (Some(Kind::Wide), next) if next != Kind::Space => self.mark(here),
            (Some(Kind::Word), Kind::Wide) if self.piece.word() => self.mark(here),
            _ => {}
        }
        match kind {
            Kind::Space => {
                self.token = Piece::default();
                self.piece = Piece::default();
                self.joint = false;
            }
            Kind::Joint => {
                self.joint = self.prev.is_some_and(|prev| prev != Kind::Space) && self.piece.word();
                self.token.add(grapheme, cells);
                self.piece = Piece::default();
            }
            Kind::Wide | Kind::Word => {
                self.joint = false;
                self.token.add(grapheme, cells);
                self.piece.add(grapheme, cells);
            }
        }
        self.prev = Some(kind);
        self.count += 1;
        self.used = self.used.saturating_add(cells);
    }

    fn done(&self, room: usize) -> bool {
        self.used > room && !self.open()
    }

    fn fit(&self, room: usize) -> Fit {
        if self.used <= room {
            return Fit {
                keep: None,
                cells: self.used,
            };
        }
        let spot = self.part.or(self.word).unwrap_or_default();
        Fit {
            keep: Some(spot.count),
            cells: spot.cells.saturating_add(1).min(room),
        }
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) struct Fit {
    pub(crate) keep: Option<usize>,
    pub(crate) cells: usize,
}

pub(crate) fn fit<'t>(runs: impl Iterator<Item = &'t str>, room: usize) -> Fit {
    let mut scan = Scan::new(room.saturating_sub(1));
    'runs: for text in runs {
        for (grapheme, cells) in text.graphemes(true).filter_map(glyph) {
            scan.feed(grapheme, cells);
            if scan.done(room) {
                break 'runs;
            }
        }
    }
    scan.fit(room)
}

/// Returns `None` when `text` fits in `room` cells; otherwise `Some(kept)`, the longest whole-word (or name-part) prefix that leaves a cell for the "…" the caller draws after it. `kept` may be empty.
pub fn cut(text: &str, room: usize) -> Option<&str> {
    let keep = fit(iter::once(text), room).keep?;
    let end = text
        .grapheme_indices(true)
        .filter(|(_, grapheme)| glyph(grapheme).is_some())
        .nth(keep)
        .map_or(text.len(), |(at, _)| at);
    Some(&text[..end])
}

pub(crate) fn cells(text: &str) -> u16 {
    u16::try_from(width(text)).unwrap_or(u16::MAX)
}

fn small(value: usize) -> u16 {
    u16::try_from(value).unwrap_or(u16::MAX)
}

pub(crate) fn blank(buf: &mut Buffer, clip: Rect, x: u16, y: u16, room: u16, style: Style) {
    let area = clip.intersection(buf.area);
    if y < area.top() || y >= area.bottom() {
        return;
    }
    let from = x.max(area.left());
    let to = x.saturating_add(room).min(area.right());
    for column in from..to {
        let cell = &mut buf[(column, y)];
        cell.reset();
        cell.set_style(style);
    }
}

pub(crate) fn put(
    buf: &mut Buffer,
    clip: Rect,
    x: u16,
    y: u16,
    room: u16,
    text: &str,
    style: Style,
) -> u16 {
    put_runs(buf, clip, x, y, room, iter::once((text, style)), false)
}

pub(crate) fn put_runs<'t, I>(
    buf: &mut Buffer,
    clip: Rect,
    x: u16,
    y: u16,
    room: u16,
    runs: I,
    right: bool,
) -> u16
where
    I: Iterator<Item = (&'t str, Style)> + Clone,
{
    let area = clip.intersection(buf.area);
    if y < area.top() || y >= area.bottom() {
        return 0;
    }
    let visible = room.min(area.right().saturating_sub(x));
    if visible == 0 {
        return 0;
    }
    let fit = fit(runs.clone().map(|(text, _)| text), usize::from(visible));
    let x = match fit.keep {
        None if right => x.saturating_add(visible - small(fit.cells)),
        _ => x,
    };
    if x < area.left() || x >= area.right() {
        return 0;
    }
    let mut used = 0usize;
    let mut index = 0usize;
    let mut cut = None;
    'runs: for (text, style) in runs {
        for (grapheme, cells) in text.graphemes(true).filter_map(glyph) {
            if fit.keep.is_some_and(|keep| index >= keep) {
                cut = Some(style);
                break 'runs;
            }
            index += 1;
            if cells == 0 {
                continue;
            }
            buf.set_stringn(x.saturating_add(small(used)), y, grapheme, cells, style);
            used += cells;
        }
    }
    if let Some(style) = cut {
        buf.set_stringn(x.saturating_add(small(used)), y, ELLIPSIS, 1, style);
        used += 1;
    }
    if used == 0 {
        0
    } else {
        x.saturating_add(small(used))
    }
}
