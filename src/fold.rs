use std::borrow::Cow;
use std::time::{Duration, Instant};

use crossterm::event::KeyEvent;
use pito_footer::Key;
use ratatui::{Frame, buffer::Buffer, layout::Rect, style::Style};

use crate::head::RULE;
use crate::label::Span;
use crate::palette::Palette;
use crate::text::cells;

const LASTING: Duration = Duration::from_millis(180);
const OPEN: &str = "▾";
const SHUT: &str = "▸";

#[derive(Debug, Clone, Copy)]
pub struct Fold<'a> {
    pub title: &'a [Span],
    pub summary: &'a [Span],
    pub rows: u16,
}

#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct FoldState {
    pub sections: Vec<(String, bool)>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct Placed {
    pub title: Rect,
    pub body: Rect,
    pub rows: u16,
    pub folding: bool,
}

#[derive(Debug, Clone)]
struct Entry {
    id: Cow<'static, str>,
    shut: bool,
    from: f32,
    start: Option<Instant>,
}

impl Entry {
    fn to(&self) -> f32 {
        if self.shut { 0.0 } else { 1.0 }
    }
}

#[derive(Debug, Clone)]
pub struct Folds {
    entries: Vec<Entry>,
    one: &'static [Key],
    all: &'static [Key],
    still: bool,
    lasting: Duration,
    gap: u16,
    open: &'static str,
    shut: &'static str,
    lit: bool,
    cursor: usize,
    now: Option<Instant>,
    top: u16,
    total: u16,
    area: Rect,
    tops: Vec<u16>,
    placed: Vec<Placed>,
    scratch: Buffer,
}

fn ease(t: f32) -> f32 {
    if t < 0.5 {
        4.0 * t * t * t
    } else {
        let back = -2.0 * t + 2.0;
        1.0 - back * back * back / 2.0
    }
}

fn width(text: &str) -> u16 {
    u16::try_from(cells(text)).unwrap_or(u16::MAX)
}

fn spans_width(spans: &[Span]) -> u16 {
    spans
        .iter()
        .fold(0u16, |total, (text, _)| total.saturating_add(width(text)))
}

fn hidden(area: Rect) -> Rect {
    Rect::new(area.x, area.bottom(), 0, 0)
}

struct Pen<'a> {
    buf: &'a mut Buffer,
    x: u16,
    y: u16,
    right: u16,
    full: bool,
}

impl Pen<'_> {
    fn put(&mut self, text: &str, style: Style) {
        if self.full || text.is_empty() {
            return;
        }
        let room = self.right.saturating_sub(self.x);
        if room == 0 {
            self.full = true;
            return;
        }
        let (end, _) = self
            .buf
            .set_stringn(self.x, self.y, text, usize::from(room), style);
        let end = end.min(self.right);
        if end - self.x < width(text) {
            self.full = true;
        }
        self.x = end;
    }

    fn spans(&mut self, spans: &[Span], base: Style) {
        for (text, style) in spans {
            self.put(text, base.patch(*style));
        }
    }

    fn rule_to(&mut self, to: u16, style: Style) {
        let to = to.min(self.right);
        while self.x < to {
            self.buf[(self.x, self.y)].set_symbol(RULE).set_style(style);
            self.x += 1;
        }
    }
}

impl Folds {
    pub fn new<I: Into<Cow<'static, str>>>(ids: impl IntoIterator<Item = I>) -> Self {
        Folds {
            entries: ids
                .into_iter()
                .map(|id| Entry {
                    id: id.into(),
                    shut: false,
                    from: 1.0,
                    start: None,
                })
                .collect(),
            one: &[],
            all: &[],
            still: false,
            lasting: LASTING,
            gap: 0,
            open: OPEN,
            shut: SHUT,
            lit: false,
            cursor: 0,
            now: None,
            top: 0,
            total: 0,
            area: Rect::ZERO,
            tops: Vec::new(),
            placed: Vec::new(),
            scratch: Buffer::empty(Rect::ZERO),
        }
    }

    pub fn keys(mut self, one: &'static [Key], all: &'static [Key]) -> Self {
        self.one = one;
        self.all = all;
        self
    }

    pub fn still(mut self, still: bool) -> Self {
        self.set_still(still);
        self
    }

    pub fn set_still(&mut self, still: bool) {
        self.still = still;
        if still {
            self.settle();
        }
    }

    pub fn lasting(mut self, fold: Duration) -> Self {
        self.lasting = fold;
        if fold.is_zero() {
            self.settle();
        }
        self
    }

    pub fn gap(mut self, rows: u16) -> Self {
        self.gap = rows;
        self
    }

    pub fn marks(mut self, open: &'static str, shut: &'static str) -> Self {
        self.open = open;
        self.shut = shut;
        self
    }

    pub fn lit(mut self, lit: bool) -> Self {
        self.lit = lit;
        self
    }

    pub fn len(&self) -> usize {
        self.entries.len()
    }

    pub fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }

    pub fn id(&self, index: usize) -> Option<&str> {
        self.entries.get(index).map(|entry| entry.id.as_ref())
    }

    pub fn index(&self, id: &str) -> Option<usize> {
        self.entries.iter().position(|entry| entry.id == id)
    }

    pub fn is_shut(&self, index: usize) -> bool {
        self.entries.get(index).is_some_and(|entry| entry.shut)
    }

    pub fn cursor(&self) -> usize {
        self.cursor
    }

    pub fn point(&mut self, index: usize) {
        self.cursor = index.min(self.entries.len().saturating_sub(1));
    }

    pub fn toggle(&mut self, index: usize, now: Instant) {
        if let Some(shut) = self.entries.get(index).map(|entry| entry.shut) {
            self.set(index, !shut, now);
        }
    }

    pub fn toggle_all(&mut self, now: Instant) {
        let shut = self.entries.iter().any(|entry| !entry.shut);
        self.set_all(shut, now);
    }

    pub fn set(&mut self, index: usize, shut: bool, now: Instant) {
        let instant = self.still || self.lasting.is_zero();
        let Some(entry) = self.entries.get(index) else {
            return;
        };
        if entry.shut == shut {
            return;
        }
        let from = self.fraction(entry, Some(now));
        let entry = &mut self.entries[index];
        entry.shut = shut;
        entry.from = from;
        entry.start = if instant || (entry.to() - from).abs() <= f32::EPSILON {
            None
        } else {
            Some(now)
        };
    }

    pub fn set_all(&mut self, shut: bool, now: Instant) {
        for index in 0..self.entries.len() {
            self.set(index, shut, now);
        }
    }

    pub fn key(&mut self, key: KeyEvent, now: Instant) -> bool {
        if self.entries.is_empty() {
            return false;
        }
        let key: Key = key.into();
        if self.one.contains(&key) {
            self.toggle(self.cursor, now);
            return true;
        }
        if self.all.contains(&key) {
            self.toggle_all(now);
            return true;
        }
        false
    }

    pub fn state(&self) -> FoldState {
        FoldState {
            sections: self
                .entries
                .iter()
                .map(|entry| (entry.id.to_string(), entry.shut))
                .collect(),
        }
    }

    pub fn restore(&mut self, state: &FoldState) {
        for (id, shut) in &state.sections {
            if let Some(index) = self.index(id) {
                let entry = &mut self.entries[index];
                entry.shut = *shut;
                entry.from = entry.to();
                entry.start = None;
            }
        }
    }

    pub fn moment(&mut self, now: Instant) {
        self.now = Some(now);
        let lasting = self.lasting;
        for entry in &mut self.entries {
            if progress(entry, lasting, Some(now)).is_some_and(|t| t >= 1.0) {
                entry.from = entry.to();
                entry.start = None;
            }
        }
    }

    pub fn animating(&self) -> bool {
        self.entries
            .iter()
            .any(|entry| progress(entry, self.lasting, self.now).is_some_and(|t| t < 1.0))
    }

    pub fn scroll(&mut self, top: u16) {
        self.top = top;
    }

    pub fn total(&self) -> u16 {
        self.total
    }

    pub fn top_of(&self, index: usize) -> Option<u16> {
        self.tops.get(index).copied()
    }

    pub fn title_at(&self, column: u16, row: u16) -> Option<usize> {
        self.placed.iter().position(|placed| {
            let title = placed.title;
            !title.is_empty() && title.y == row && column >= title.x && column < title.right()
        })
    }

    pub fn placed(&self) -> &[Placed] {
        &self.placed
    }

    fn settle(&mut self) {
        for entry in &mut self.entries {
            entry.from = entry.to();
            entry.start = None;
        }
    }

    fn fraction(&self, entry: &Entry, now: Option<Instant>) -> f32 {
        match progress(entry, self.lasting, now) {
            Some(t) => entry.from + (entry.to() - entry.from) * ease(t),
            None => entry.to(),
        }
    }

    pub fn draw(
        &mut self,
        frame: &mut Frame,
        area: Rect,
        palette: &Palette,
        sections: &[Fold<'_>],
        mut body: impl FnMut(&mut Buffer, usize, Rect),
    ) {
        let buf = frame.buffer_mut();
        let area = area.intersection(buf.area);
        let count = self.entries.len().min(sections.len());
        self.placed.clear();
        self.tops.clear();
        self.area = area;
        let mut total = 0u16;
        for (index, section) in sections.iter().enumerate().take(count) {
            if index > 0 {
                total = total.saturating_add(self.gap);
            }
            self.tops.push(total);
            let open = self.fraction(&self.entries[index], self.now);
            let shown = (f32::from(section.rows) * open).round() as u16;
            total = total
                .saturating_add(1)
                .saturating_add(shown.min(section.rows));
        }
        self.total = total;
        self.top = self.top.min(total.saturating_sub(area.height));
        if area.is_empty() {
            return;
        }
        let view = self.top;
        let end = view.saturating_add(area.height);
        for (index, section) in sections.iter().enumerate().take(count) {
            let entry = &self.entries[index];
            let folding = progress(entry, self.lasting, self.now).is_some_and(|t| t < 1.0);
            let open = self.fraction(entry, self.now);
            let shown = ((f32::from(section.rows) * open).round() as u16).min(section.rows);
            let top = self.tops[index];
            let title = if top >= view && top < end {
                let line = Rect::new(area.x, area.y + (top - view), area.width, 1);
                self.title(buf, line, palette, index, section);
                line
            } else {
                hidden(area)
            };
            let first = top.saturating_add(1);
            let last = first.saturating_add(shown);
            let from = first.max(view);
            let to = last.min(end);
            let mut placed = Placed {
                title,
                body: hidden(area),
                rows: section.rows,
                folding,
            };
            if from < to {
                let y = area.y + (from - view);
                let visible = Rect::new(area.x, y, area.width, to - from);
                let top_on = first >= view && first < end;
                if !folding && top_on {
                    let room = area.bottom() - y;
                    let rect = Rect::new(area.x, y, area.width, section.rows.min(room));
                    body(buf, index, rect);
                    placed.body = rect;
                } else {
                    let height = if top_on {
                        section.rows.min(area.bottom() - y)
                    } else {
                        section.rows
                    };
                    let rect = Rect::new(area.x, 0, area.width, height);
                    self.curtain(buf, &mut body, index, rect, from - first, visible);
                    placed.body = visible;
                }
            }
            self.placed.push(placed);
        }
    }

    fn curtain(
        &mut self,
        buf: &mut Buffer,
        body: &mut impl FnMut(&mut Buffer, usize, Rect),
        index: usize,
        rect: Rect,
        skip: u16,
        visible: Rect,
    ) {
        let held = self.scratch.area;
        if rect.right() > held.width || rect.height > held.height {
            self.scratch.resize(Rect::new(
                0,
                0,
                held.width.max(rect.right()),
                held.height.max(rect.height),
            ));
        }
        let rows = visible.height.min(rect.height.saturating_sub(skip));
        for row in 0..rows {
            for x in visible.left()..visible.right() {
                self.scratch[(x, skip + row)] = buf[(x, visible.y + row)].clone();
            }
        }
        body(&mut self.scratch, index, rect);
        for row in 0..rows {
            for x in visible.left()..visible.right() {
                buf[(x, visible.y + row)] = self.scratch[(x, skip + row)].clone();
            }
        }
    }

    fn title(&self, buf: &mut Buffer, line: Rect, palette: &Palette, index: usize, fold: &Fold) {
        let entry = &self.entries[index];
        let base = if self.lit && index == self.cursor {
            palette.selected
        } else {
            palette.strong
        };
        let mark = if entry.shut { self.shut } else { self.open };
        let mut pen = Pen {
            buf,
            x: line.x,
            y: line.y,
            right: line.right(),
            full: false,
        };
        let summary = spans_width(fold.summary);
        let need = width(mark)
            .saturating_add(1)
            .saturating_add(spans_width(fold.title))
            .saturating_add(3)
            .saturating_add(summary)
            .saturating_add(2);
        let tail = entry.shut && summary > 0 && need <= line.width;
        pen.put(mark, base);
        pen.put(" ", base);
        pen.spans(fold.title, base);
        pen.put(" ", base);
        if tail {
            pen.rule_to(line.right() - summary - 3, palette.rule);
            pen.put(" ", palette.muted);
            pen.spans(fold.summary, palette.muted);
            pen.put(" ", palette.muted);
        }
        pen.rule_to(line.right(), palette.rule);
    }
}

fn progress(entry: &Entry, lasting: Duration, now: Option<Instant>) -> Option<f32> {
    let start = entry.start?;
    let span = lasting.as_secs_f32() * (entry.to() - entry.from).abs();
    if span <= 0.0 {
        return Some(1.0);
    }
    let elapsed = now.map_or(0.0, |now| {
        now.saturating_duration_since(start).as_secs_f32()
    });
    Some((elapsed / span).clamp(0.0, 1.0))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn ease_is_cubic_in_out_and_pinned_at_both_ends() {
        assert_eq!(ease(0.0), 0.0);
        assert_eq!(ease(1.0), 1.0);
        assert!((ease(0.5) - 0.5).abs() < 1e-6);
        assert!(ease(0.25) < 0.25);
        assert!(ease(0.75) > 0.75);
        let mut last = 0.0;
        for step in 0..=100 {
            let value = ease(step as f32 / 100.0);
            assert!(value >= last);
            last = value;
        }
    }

    #[test]
    fn a_reversal_mid_fold_starts_from_the_current_fraction() {
        let start = Instant::now();
        let mut folds = Folds::new(["a"]);
        folds.toggle(0, start);
        let mid = start + Duration::from_millis(60);
        let before = folds.fraction(&folds.entries[0], Some(mid));
        folds.toggle(0, mid);
        let after = folds.fraction(&folds.entries[0], Some(mid));
        assert!((before - after).abs() < 1e-6);
        assert!(!folds.is_shut(0));
    }
}
