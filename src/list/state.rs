use std::{borrow::Cow, sync::Arc};

use ratatui::layout::Rect;

use crate::list::{Key, MAX_COLUMNS, Row, row::Kind};

const PAGE: usize = 10;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct Keys {
    pub up: &'static [Key],
    pub down: &'static [Key],
    pub page_up: &'static [Key],
    pub page_down: &'static [Key],
    pub half_up: &'static [Key],
    pub half_down: &'static [Key],
    pub first: &'static [Key],
    pub last: &'static [Key],
    pub open: &'static [Key],
}

impl Keys {
    pub const fn new() -> Self {
        Keys {
            up: &[Key::Up],
            down: &[Key::Down],
            page_up: &[Key::PageUp],
            page_down: &[Key::PageDown],
            half_up: &[Key::Ctrl('u')],
            half_down: &[Key::Ctrl('d')],
            first: &[Key::Home],
            last: &[Key::End],
            open: &[Key::Enter],
        }
    }

    pub const VIM: Keys = Keys {
        up: &[Key::Up, Key::Char('k')],
        down: &[Key::Down, Key::Char('j')],
        page_up: &[Key::PageUp],
        page_down: &[Key::PageDown],
        half_up: &[Key::Ctrl('u')],
        half_down: &[Key::Ctrl('d')],
        first: &[Key::Home, Key::Char('g')],
        last: &[Key::End, Key::Char('G')],
        open: &[Key::Enter],
    };

    pub const fn up(mut self, keys: &'static [Key]) -> Self {
        self.up = keys;
        self
    }

    pub const fn down(mut self, keys: &'static [Key]) -> Self {
        self.down = keys;
        self
    }

    pub const fn page_up(mut self, keys: &'static [Key]) -> Self {
        self.page_up = keys;
        self
    }

    pub const fn page_down(mut self, keys: &'static [Key]) -> Self {
        self.page_down = keys;
        self
    }

    pub const fn half_up(mut self, keys: &'static [Key]) -> Self {
        self.half_up = keys;
        self
    }

    pub const fn half_down(mut self, keys: &'static [Key]) -> Self {
        self.half_down = keys;
        self
    }

    pub const fn first(mut self, keys: &'static [Key]) -> Self {
        self.first = keys;
        self
    }

    pub const fn last(mut self, keys: &'static [Key]) -> Self {
        self.last = keys;
        self
    }

    pub const fn open(mut self, keys: &'static [Key]) -> Self {
        self.open = keys;
        self
    }
}

impl Default for Keys {
    fn default() -> Self {
        Keys::new()
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Step {
    Pass,
    Held,
    Moved,
    Open(usize),
}

pub trait Source {
    fn len(&self) -> usize;

    fn row(&self, index: usize) -> Cow<'_, Row>;

    fn is_empty(&self) -> bool {
        self.len() == 0
    }

    fn selectable(&self, index: usize) -> bool {
        self.row(index).selectable()
    }

    fn mark_width(&self) -> u16 {
        0
    }
}

macro_rules! rows {
    ($($source:ty),*) => {$(
        impl Source for $source {
            fn len(&self) -> usize {
                <[Row]>::len(self)
            }

            fn row(&self, index: usize) -> Cow<'_, Row> {
                Cow::Borrowed(&self[index])
            }

            fn selectable(&self, index: usize) -> bool {
                self[index].selectable()
            }

            fn mark_width(&self) -> u16 {
                self.iter().map(Row::mark_width).max().unwrap_or(0)
            }
        }
    )*};
}

rows!(Vec<Row>, Arc<Vec<Row>>, Arc<[Row]>);

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum Paging {
    #[default]
    Cursor,
    View,
}

#[derive(Debug, Clone)]
pub struct List<S = Vec<Row>> {
    source: S,
    selected: usize,
    top: usize,
    page: usize,
    mark_width: u16,
    touched: Option<(usize, u16)>,
    rescan: bool,
    range: Option<(usize, usize)>,
    keys: Keys,
    paging: Paging,
    placed: [(usize, u16, u16); MAX_COLUMNS],
    placed_len: usize,
    shown: Option<(usize, usize, usize)>,
    cut: bool,
    bar: bool,
}

impl Default for List {
    fn default() -> Self {
        List::new()
    }
}

impl List {
    pub fn new() -> Self {
        List::from_source(Vec::new())
    }

    pub fn with_rows(mut self, rows: impl IntoIterator<Item = Row>) -> Self {
        self.set_rows(rows);
        self
    }

    pub fn set_rows(&mut self, rows: impl IntoIterator<Item = Row>) {
        self.source.clear();
        self.mark_width = 0;
        self.touched = None;
        self.rescan = false;
        for row in rows {
            self.push(row);
        }
        self.settle();
    }

    pub fn push(&mut self, row: Row) {
        self.mark_width = self.mark_width.max(row.mark_width());
        self.source.push(row);
    }

    pub fn clear(&mut self) {
        self.source.clear();
        self.mark_width = 0;
        self.touched = None;
        self.rescan = false;
        self.selected = 0;
        self.top = 0;
    }

    pub fn rows(&self) -> &[Row] {
        &self.source
    }

    pub fn selected_row(&self) -> Option<&Row> {
        self.source.get(self.selected()?)
    }

    pub fn row_mut(&mut self, index: usize) -> Option<&mut Row> {
        self.settle_marks();
        let before = self.source.get(index)?.mark_width();
        self.touched = Some((index, before));
        self.source.get_mut(index)
    }
}

impl<S: Source> List<S> {
    pub fn from_source(source: S) -> Self {
        let mark_width = source.mark_width();
        List {
            source,
            selected: 0,
            top: 0,
            page: PAGE,
            mark_width,
            touched: None,
            rescan: false,
            range: None,
            keys: Keys::new(),
            paging: Paging::Cursor,
            placed: [(0, 0, 0); MAX_COLUMNS],
            placed_len: 0,
            shown: None,
            cut: false,
            bar: false,
        }
    }

    pub fn keys(mut self, keys: Keys) -> Self {
        self.keys = keys;
        self
    }

    pub fn paging(mut self, paging: Paging) -> Self {
        self.paging = paging;
        self
    }

    pub fn source(&self) -> &S {
        &self.source
    }

    pub fn set_source(&mut self, source: S) {
        self.source = source;
        self.mark_width = self.source.mark_width();
        self.touched = None;
        self.rescan = false;
        self.settle();
    }

    pub fn update(&mut self, change: impl FnOnce(&mut S)) {
        change(&mut self.source);
        self.mark_width = self.source.mark_width();
        self.touched = None;
        self.rescan = false;
        self.settle();
    }

    fn settle(&mut self) {
        let last = self.source.len().saturating_sub(1);
        self.selected = self.selected.min(last);
        self.top = self.top.min(last);
    }

    pub fn len(&self) -> usize {
        self.source.len()
    }

    pub fn is_empty(&self) -> bool {
        self.source.is_empty()
    }

    pub fn selected(&self) -> Option<usize> {
        let last = self.source.len().checked_sub(1)?;
        let at = self.selected.min(last);
        self.next(at).or_else(|| self.prev(at))
    }

    pub fn select(&mut self, index: usize) {
        let at = index.min(self.source.len().saturating_sub(1));
        self.selected = self.next(at).or_else(|| self.prev(at)).unwrap_or(at);
    }

    pub fn select_range(&mut self, range: Option<(usize, usize)>) {
        self.range = range;
    }

    pub fn range(&self) -> Option<(usize, usize)> {
        self.range
    }

    pub(crate) fn in_range(&self, index: usize) -> bool {
        self.range
            .is_some_and(|(from, to)| (from.min(to)..=from.max(to)).contains(&index))
    }

    pub fn top(&self) -> usize {
        self.top
    }

    pub fn scroll_to(&mut self, top: usize) {
        self.top = top.min(self.source.len().saturating_sub(1));
    }

    pub fn page(&self) -> usize {
        self.page
    }

    pub fn columns(&self) -> &[(usize, u16, u16)] {
        &self.placed[..self.placed_len]
    }

    pub fn shown(&self) -> Option<(usize, usize, usize)> {
        self.shown
    }

    pub fn position(&self) -> Option<(usize, usize)> {
        Some((self.selected()?.saturating_add(1), self.source.len()))
    }

    pub(crate) fn drawn(&mut self, shown: Option<(usize, usize, usize)>, cut: bool, bar: bool) {
        self.shown = shown;
        self.cut = cut;
        self.bar = bar;
    }

    pub(crate) fn place(&mut self, placed: impl IntoIterator<Item = (usize, u16, u16)>) {
        self.placed_len = 0;
        for (slot, column) in self.placed.iter_mut().zip(placed) {
            *slot = column;
            self.placed_len += 1;
        }
    }

    pub(crate) fn mark_width(&mut self) -> u16 {
        self.settle_marks();
        if self.rescan {
            self.mark_width = self.source.mark_width();
            self.rescan = false;
        }
        self.mark_width
    }

    fn settle_marks(&mut self) {
        let Some((index, before)) = self.touched.take() else {
            return;
        };
        let after = if index < self.source.len() {
            self.source.row(index).mark_width()
        } else {
            0
        };
        if after >= self.mark_width {
            self.mark_width = after;
            self.rescan = false;
        } else if before >= self.mark_width {
            self.rescan = true;
        }
    }

    pub(crate) fn row(&self, index: usize) -> Cow<'_, Row> {
        self.source.row(index)
    }

    fn next(&self, from: usize) -> Option<usize> {
        (from..self.source.len()).find(|&at| self.source.selectable(at))
    }

    fn prev(&self, from: usize) -> Option<usize> {
        let end = from.saturating_add(1).min(self.source.len());
        (0..end).rev().find(|&at| self.source.selectable(at))
    }

    fn go(&mut self, to: impl FnOnce(&Self, usize) -> Option<usize>) -> Step {
        let Some(from) = self.selected() else {
            return Step::Held;
        };
        self.selected = from;
        match to(self, from) {
            Some(to) if to != from => {
                self.selected = to;
                Step::Moved
            }
            _ => Step::Held,
        }
    }

    pub fn up(&mut self) -> Step {
        self.go(|list, from| list.prev(from.checked_sub(1)?))
    }

    pub fn down(&mut self) -> Step {
        self.go(|list, from| list.next(from.saturating_add(1)))
    }

    pub fn page_up(&mut self) -> Step {
        self.hop_up(self.page)
    }

    pub fn page_down(&mut self) -> Step {
        self.hop_down(self.page)
    }

    pub fn half_up(&mut self) -> Step {
        self.hop_up(self.page.div_ceil(2))
    }

    pub fn half_down(&mut self) -> Step {
        self.hop_down(self.page.div_ceil(2))
    }

    fn hop_up(&mut self, rows: usize) -> Step {
        self.flip(|list, from| {
            let to = from.saturating_sub(rows);
            list.prev(to).or_else(|| list.next(to))
        })
    }

    fn hop_down(&mut self, rows: usize) -> Step {
        self.flip(|list, from| {
            let to = from
                .saturating_add(rows)
                .min(list.source.len().saturating_sub(1));
            list.next(to).or_else(|| list.prev(to))
        })
    }

    fn flip(&mut self, to: impl FnOnce(&Self, usize) -> Option<usize>) -> Step {
        let from = self.selected();
        let step = self.go(to);
        if let (Step::Moved, Paging::View, Some(from)) = (step, self.paging, from) {
            let to = self.selected;
            self.top = if to > from {
                self.top.saturating_add(to - from)
            } else {
                self.top.saturating_sub(from - to)
            };
        }
        step
    }

    pub fn first(&mut self) -> Step {
        self.go(|list, _| list.next(0))
    }

    pub fn last(&mut self) -> Step {
        self.go(|list, _| list.prev(list.source.len().saturating_sub(1)))
    }

    pub fn key(&mut self, key: Key) -> Step {
        let keys = self.keys;
        if keys.up.contains(&key) {
            self.up()
        } else if keys.down.contains(&key) {
            self.down()
        } else if keys.page_up.contains(&key) {
            self.page_up()
        } else if keys.page_down.contains(&key) {
            self.page_down()
        } else if keys.half_up.contains(&key) {
            self.half_up()
        } else if keys.half_down.contains(&key) {
            self.half_down()
        } else if keys.first.contains(&key) {
            self.first()
        } else if keys.last.contains(&key) {
            self.last()
        } else if keys.open.contains(&key) {
            match self.selected() {
                Some(index) => Step::Open(index),
                None => Step::Held,
            }
        } else {
            Step::Pass
        }
    }

    pub fn hit(&self, area: Rect, header: bool, column: u16, row: u16) -> Option<usize> {
        let first = area.y.saturating_add(u16::from(header));
        let right = area.right().saturating_sub(u16::from(self.bar));
        let bottom = area.bottom().saturating_sub(u16::from(self.cut));
        if column < area.x || column >= right || row < first || row >= bottom {
            return None;
        }
        let index = self.top.saturating_add(usize::from(row - first));
        (index < self.source.len() && self.source.selectable(index)).then_some(index)
    }

    fn heading_above(&self, at: usize, height: usize) -> usize {
        let reach = at.saturating_sub(height.saturating_sub(1));
        let mut head = at;
        for above in (reach..at).rev() {
            let row = self.source.row(above);
            match row.kind {
                Kind::Item => break,
                Kind::Heading => head = above,
                Kind::Blank => {}
            }
        }
        head
    }

    pub(crate) fn follow(&mut self, height: usize, tail: bool) -> Option<usize> {
        if height > 0 {
            self.page = height.saturating_sub(1).max(1);
        }
        let len = self.source.len();
        if height == 0 || len == 0 {
            self.top = 0;
            return None;
        }
        let lines = len + usize::from(tail);
        let Some(selected) = self.selected() else {
            self.top = self.top.min(lines.saturating_sub(height));
            return None;
        };
        self.selected = selected;
        if selected < self.top {
            self.top = selected;
        }
        let last = selected + 1 == len;
        let bottom = selected + 1 + usize::from(last && tail);
        if bottom > self.top.saturating_add(height) {
            self.top = bottom - height;
        }
        self.top = self.top.min(self.heading_above(selected, height));
        self.top = self.top.min(lines.saturating_sub(height));
        Some(selected)
    }
}
