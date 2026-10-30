#![doc = include_str!("README.md")]

use std::time::Duration;

use ratatui::{
    buffer::{Buffer, CellWidth},
    layout::Rect,
    style::{Modifier, Style},
    widgets::Widget,
};
use unicode_segmentation::UnicodeSegmentation;

pub const WIDTH: u16 = 11;
pub const GLASS_ROWS: u16 = 6;
const LABEL: &str = "Loading...";
const DRAIN_FRAMES: u128 = 7;
const FLIP_FRAMES: u128 = 6;
const DRAIN_MS: u128 = 3000;
const FLIP_MS: u128 = 1200;
const SWEEP_MS: u128 = 1200;
const SHIMMER_MS: u128 = 1500;
pub const CYCLE: Duration = Duration::from_millis((DRAIN_MS + FLIP_MS) as u64);

pub const FRAMES: [[&str; 6]; 13] = [
    [
        "   ╭───╮   ",
        "   │⣿⣿⣿│   ",
        "   ╰╮⣿╭╯   ",
        "   ╭╯ ╰╮   ",
        "   │   │   ",
        "   ╰───╯   ",
    ],
    [
        "   ╭───╮   ",
        "   │⣿⣿⣿│   ",
        "   ╰╮ ╭╯   ",
        "   ╭╯⡇╰╮   ",
        "   │ ⣀ │   ",
        "   ╰───╯   ",
    ],
    [
        "   ╭───╮   ",
        "   │⣿ ⣿│   ",
        "   ╰╮ ╭╯   ",
        "   ╭╯⡇╰╮   ",
        "   │⣀⣤⣀│   ",
        "   ╰───╯   ",
    ],
    [
        "   ╭───╮   ",
        "   │⣀ ⣀│   ",
        "   ╰╮ ╭╯   ",
        "   ╭╯⡇╰╮   ",
        "   │⣤⣤⣤│   ",
        "   ╰───╯   ",
    ],
    [
        "   ╭───╮   ",
        "   │   │   ",
        "   ╰╮ ╭╯   ",
        "   ╭╯⡇╰╮   ",
        "   │⣿⣤⣿│   ",
        "   ╰───╯   ",
    ],
    [
        "   ╭───╮   ",
        "   │   │   ",
        "   ╰╮ ╭╯   ",
        "   ╭╯⡇╰╮   ",
        "   │⣿⣿⣿│   ",
        "   ╰───╯   ",
    ],
    [
        "   ╭───╮   ",
        "   │   │   ",
        "   ╰╮ ╭╯   ",
        "   ╭╯ ╰╮   ",
        "   │⣿⣿⣿│   ",
        "   ╰───╯   ",
    ],
    [
        "╭───╮      ",
        " │   │     ",
        "  ╰╮ ╭╯    ",
        "    ╭╯ ╰╮  ",
        "     │⣿⣿⣿│ ",
        "      ╰───╯",
    ],
    [
        "╭─         ",
        "│ ──╮      ",
        "╰─   ⣠╭──  ",
        "  ──╯ ⣾⣿⣿─╮",
        "      ╰──⣿│",
        "         ─╯",
    ],
    [
        "           ",
        "╭───╮ ╭───╮",
        "│    ⣠⣾⣿⣿⣿│",
        "╰───╯ ╰───╯",
        "           ",
        "           ",
    ],
    [
        "         ─╮",
        "      ╭──⣿│",
        "  ──╮ ⣾⣿⣿─╯",
        "╭─    ╰──  ",
        "│ ──╯      ",
        "╰─         ",
    ],
    [
        "      ╭───╮",
        "     │⣿⣿⣿│ ",
        "    ╰╮ ╭╯  ",
        "  ╭╯ ╰╮    ",
        " │   │     ",
        "╰───╯      ",
    ],
    [
        "   ╭───╮   ",
        "   │⣿⣿⣿│   ",
        "   ╰╮⣿╭╯   ",
        "   ╭╯ ╰╮   ",
        "   │   │   ",
        "   ╰───╯   ",
    ],
];

pub fn frame_at(elapsed: Duration) -> usize {
    let e = elapsed.as_millis() % CYCLE.as_millis();
    let frame = if e < DRAIN_MS {
        e * DRAIN_FRAMES / DRAIN_MS
    } else {
        DRAIN_FRAMES + (e - DRAIN_MS) * FLIP_FRAMES / FLIP_MS
    };
    frame as usize
}

fn band_centre(elapsed: Duration, n: usize) -> Option<i128> {
    let t = elapsed.as_millis() % SHIMMER_MS;
    if t >= SWEEP_MS {
        return None;
    }
    let reach = n as u128 + 4;
    Some((t * reach / SWEEP_MS) as i128 - 2)
}

pub fn band_at(elapsed: Duration, n: usize) -> Option<usize> {
    let centre = usize::try_from(band_centre(elapsed, n)?).ok()?;
    (centre < n).then_some(centre)
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Hourglass<'a> {
    elapsed: Duration,
    label: &'a str,
    hint: Option<&'a str>,
    glass: Style,
    accent: Style,
    muted: Style,
}

impl<'a> Hourglass<'a> {
    pub fn new(elapsed: Duration) -> Self {
        Hourglass {
            elapsed,
            label: LABEL,
            hint: None,
            glass: Style::new(),
            accent: Style::new(),
            muted: Style::new(),
        }
    }

    pub fn label(mut self, label: &'a str) -> Self {
        self.label = label;
        self
    }

    pub fn hint(mut self, hint: Option<&'a str>) -> Self {
        self.hint = hint;
        self
    }

    pub fn glass(mut self, style: Style) -> Self {
        self.glass = style;
        self
    }

    pub fn accent(mut self, style: Style) -> Self {
        self.accent = style;
        self
    }

    pub fn muted(mut self, style: Style) -> Self {
        self.muted = style;
        self
    }

    fn shade(&self, index: usize, centre: Option<i128>) -> Style {
        match centre.map(|centre| (index as i128 - centre).abs()) {
            Some(0) => self.accent.add_modifier(Modifier::BOLD),
            Some(1) => self.accent,
            _ => self.muted,
        }
    }

    fn shimmer(&self, buf: &mut Buffer, area: Rect, y: u16) {
        centred(buf, area, y, self.label, |index, n| {
            self.shade(index, band_centre(self.elapsed, n))
        });
    }
}

fn fitted(text: &str, room: u16) -> impl Iterator<Item = (&str, u16)> + Clone {
    text.graphemes(true)
        .filter(|glyph| !glyph.contains(char::is_control))
        .map(|glyph| (glyph, glyph.cell_width()))
        .filter(|(_, width)| *width > 0)
        .scan(0u16, move |used, (glyph, width)| {
            *used = used.checked_add(width).filter(|next| *next <= room)?;
            Some((glyph, width))
        })
}

fn centred(
    buf: &mut Buffer,
    area: Rect,
    y: u16,
    text: &str,
    shade: impl Fn(usize, usize) -> Style,
) {
    let glyphs = fitted(text, area.width);
    let n = glyphs.clone().count();
    let used = glyphs
        .clone()
        .fold(0u16, |used, (_, width)| used.saturating_add(width));
    let mut x = area.x.saturating_add(area.width.saturating_sub(used) / 2);
    for (index, (glyph, width)) in glyphs.enumerate() {
        buf.set_stringn(x, y, glyph, usize::from(width), shade(index, n));
        x = x.saturating_add(width);
    }
}

impl Widget for &Hourglass<'_> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        let area = area.intersection(buf.area);
        if area.is_empty() {
            return;
        }
        let hint = self.hint.filter(|hint| !hint.is_empty());
        let block = GLASS_ROWS + 1 + u16::from(hint.is_some());
        if area.width >= WIDTH && area.height >= block {
            let top = area.y + (area.height - block) / 2;
            let left = area.x + (area.width - WIDTH) / 2;
            for (y, line) in (top..).zip(FRAMES[frame_at(self.elapsed)]) {
                buf.set_stringn(left, y, line, usize::from(WIDTH), self.glass);
            }
            self.shimmer(buf, area, top + GLASS_ROWS);
            if let Some(hint) = hint {
                centred(buf, area, top + GLASS_ROWS + 1, hint, |_, _| self.muted);
            }
            return;
        }
        let rows = if hint.is_some() && area.height >= 2 {
            2
        } else {
            1
        };
        let top = area.y + (area.height - rows) / 2;
        self.shimmer(buf, area, top);
        if let Some(hint) = hint.filter(|_| rows == 2) {
            centred(buf, area, top + 1, hint, |_, _| self.muted);
        }
    }
}

impl Widget for Hourglass<'_> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        (&self).render(area, buf);
    }
}
