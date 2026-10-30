use std::{fmt::Write, iter};

use ratatui::{buffer::Buffer, layout::Rect, style::Style, widgets::Widget};

use crate::list::{
    Bar, Column, Count, List, Place, Row, Source,
    bar::{Stack, thumb},
    row::{Kind, Paint},
    text::{self, blank, put, put_runs},
};

pub const CURSOR: &str = "▌ ";
pub const MAX_COLUMNS: usize = 32;
const GAP: u16 = 2;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct Styles {
    pub(crate) selected: Style,
    pub(crate) ink: Style,
    pub(crate) faint: Style,
    pub(crate) base: Style,
    pub(crate) header: Option<Style>,
    pub(crate) header_key: Option<Style>,
    pub(crate) cursor: Option<Style>,
    pub(crate) keep: bool,
    pub(crate) track: Option<Style>,
    pub(crate) thumb: Option<Style>,
    pub(crate) count: Option<Style>,
}

impl Styles {
    pub const fn new() -> Self {
        Styles {
            selected: Style::new(),
            ink: Style::new(),
            faint: Style::new(),
            base: Style::new(),
            header: None,
            header_key: None,
            cursor: None,
            keep: false,
            track: None,
            thumb: None,
            count: None,
        }
    }

    pub const fn selected(mut self, style: Style) -> Self {
        self.selected = style;
        self
    }

    pub const fn ink(mut self, style: Style) -> Self {
        self.ink = style;
        self
    }

    pub const fn faint(mut self, style: Style) -> Self {
        self.faint = style;
        self
    }

    pub const fn base(mut self, style: Style) -> Self {
        self.base = style;
        self
    }

    pub const fn header(mut self, style: Style) -> Self {
        self.header = Some(style);
        self
    }

    pub const fn header_key(mut self, style: Style) -> Self {
        self.header_key = Some(style);
        self
    }

    pub const fn cursor(mut self, style: Style) -> Self {
        self.cursor = Some(style);
        self
    }

    pub const fn keep_colours(mut self, keep: bool) -> Self {
        self.keep = keep;
        self
    }

    pub const fn track(mut self, style: Style) -> Self {
        self.track = Some(style);
        self
    }

    pub const fn thumb(mut self, style: Style) -> Self {
        self.thumb = Some(style);
        self
    }

    pub const fn count(mut self, style: Style) -> Self {
        self.count = Some(style);
        self
    }

    fn resolve(&self, paint: Paint) -> Style {
        match paint {
            Paint::Ink => self.ink,
            Paint::Faint => self.faint,
            Paint::Own(style) => style,
        }
    }
}

impl Default for Styles {
    fn default() -> Self {
        Styles::new()
    }
}

struct Layout {
    widths: [u16; MAX_COLUMNS],
    kept: [bool; MAX_COLUMNS],
    right: [bool; MAX_COLUMNS],
    count: usize,
    flex: usize,
}

fn small(value: usize) -> u16 {
    u16::try_from(value).unwrap_or(u16::MAX)
}

impl Layout {
    fn new(columns: &[Column], width: u16, lead: u16, gap: u16) -> Self {
        let count = columns.len().min(MAX_COLUMNS);
        let mut layout = Layout {
            widths: [0; MAX_COLUMNS],
            kept: [false; MAX_COLUMNS],
            right: [false; MAX_COLUMNS],
            count,
            flex: 0,
        };
        for (right, column) in layout.right.iter_mut().zip(columns) {
            *right = column.right;
        }
        if count == 0 {
            return layout;
        }
        layout.flex = columns[..count]
            .iter()
            .position(|column| column.flex)
            .unwrap_or(count - 1);
        layout.kept[..count].fill(true);
        while layout.need(columns, lead, gap) > usize::from(width) {
            let mut pick: Option<usize> = None;
            for at in 0..count {
                if at != layout.flex && layout.kept[at] && !columns[at].pinned {
                    match pick {
                        Some(held) if columns[held].rank > columns[at].rank => {}
                        _ => pick = Some(at),
                    }
                }
            }
            match pick {
                Some(at) => layout.kept[at] = false,
                None => break,
            }
        }
        layout
    }

    fn need(&self, columns: &[Column], lead: u16, gap: u16) -> usize {
        let mut total = usize::from(lead).saturating_add(usize::from(columns[self.flex].min));
        for (at, column) in columns.iter().enumerate().take(self.count) {
            if at != self.flex && self.kept[at] {
                total = total
                    .saturating_add(usize::from(column.min))
                    .saturating_add(usize::from(gap));
            }
        }
        total
    }

    fn fits(&self, columns: &[Column], at: usize) -> bool {
        at != self.flex && self.kept[at] && columns[at].fit.is_some()
    }

    fn size(&mut self, columns: &[Column], width: u16, lead: u16, gap: u16, fitted: &[u16]) {
        if self.count == 0 {
            return;
        }
        let width = usize::from(width);
        let mut extra = width.saturating_sub(self.need(columns, lead, gap));
        let mut used = usize::from(lead);
        for (at, column) in columns.iter().enumerate().take(self.count) {
            if at == self.flex || !self.kept[at] {
                continue;
            }
            let min = usize::from(column.min);
            let preferred = match column.fit {
                Some(cap) => fitted[at].min(cap),
                None => column.preferred,
            };
            let grow = usize::from(preferred).saturating_sub(min).min(extra);
            extra -= grow;
            let size = min + grow;
            self.widths[at] = small(size);
            used = used.saturating_add(size).saturating_add(usize::from(gap));
        }
        self.widths[self.flex] = small(width.saturating_sub(used));
    }

    fn places(&self, from: u16, edge: u16, gap: u16) -> impl Iterator<Item = (usize, u16, u16)> {
        (0..self.count)
            .filter(|&at| self.kept[at])
            .scan(from, move |x, at| {
                let start = *x;
                *x = start.saturating_add(self.widths[at]).saturating_add(gap);
                Some((at, start, self.widths[at].min(edge.saturating_sub(start))))
            })
            .filter(|&(_, _, width)| width > 0)
    }
}

fn fitted<S: Source>(
    list: &List<S>,
    columns: &[Column],
    layout: &Layout,
    header: bool,
    lines: usize,
) -> [u16; MAX_COLUMNS] {
    let mut widths = [0; MAX_COLUMNS];
    let count = layout.count;
    if !(0..count).any(|at| layout.fits(columns, at)) {
        return widths;
    }
    if header {
        for (at, width) in widths.iter_mut().enumerate().take(count) {
            if layout.fits(columns, at) {
                *width = text::cells(columns[at].title);
            }
        }
    }
    let from = list.top();
    let to = from.saturating_add(lines).min(list.len());
    for index in from..to {
        let row = list.row(index);
        if row.kind != Kind::Item {
            continue;
        }
        for (at, cell) in row.cells.iter().enumerate().take(count) {
            if !layout.kept[at] {
                continue;
            }
            if cell.rest {
                break;
            }
            if layout.fits(columns, at) {
                let cells = cell.pieces().fold(0usize, |total, (text, _)| {
                    total.saturating_add(text::width(text))
                });
                widths[at] = widths[at].max(small(cells));
            }
        }
    }
    widths
}

fn columns_line<'r, I>(
    buf: &mut Buffer,
    clip: Rect,
    y: u16,
    from: u16,
    layout: &Layout,
    gap: u16,
    mut cell: impl FnMut(usize) -> Option<(I, bool)>,
) -> u16
where
    I: Iterator<Item = (&'r str, Style)> + Clone,
{
    let mut x = from;
    let mut ink = 0;
    for at in 0..layout.count {
        if !layout.kept[at] {
            continue;
        }
        let size = layout.widths[at];
        if let Some((runs, rest)) = cell(at) {
            if rest {
                let room = clip.right().saturating_sub(x);
                return ink.max(put_runs(buf, clip, x, y, room, runs, layout.right[at]));
            }
            ink = ink.max(put_runs(buf, clip, x, y, size, runs, layout.right[at]));
        }
        x = x.saturating_add(size).saturating_add(gap);
    }
    ink
}

#[derive(Debug)]
pub struct ListView<'a, S = Vec<Row>> {
    list: &'a mut List<S>,
    columns: &'a [Column<'a>],
    styles: Styles,
    cursor: &'a str,
    gap: u16,
    header: bool,
    key: Option<usize>,
    end: Option<&'a str>,
    empty: Option<&'a str>,
    bar: Option<Bar<'a>>,
    count: Option<Count<'a>>,
}

impl<'a, S: Source> ListView<'a, S> {
    pub fn new(list: &'a mut List<S>, columns: &'a [Column<'a>]) -> Self {
        ListView {
            list,
            columns,
            styles: Styles::new(),
            cursor: CURSOR,
            gap: GAP,
            header: false,
            key: None,
            end: None,
            empty: None,
            bar: None,
            count: None,
        }
    }

    pub fn scrollbar(mut self, bar: Option<Bar<'a>>) -> Self {
        self.bar = bar;
        self
    }

    pub fn count(mut self, count: Option<Count<'a>>) -> Self {
        self.count = count;
        self
    }

    pub fn styles(mut self, styles: Styles) -> Self {
        self.styles = styles;
        self
    }

    pub fn cursor(mut self, cursor: &'a str) -> Self {
        self.cursor = cursor;
        self
    }

    pub fn gap(mut self, gap: u16) -> Self {
        self.gap = gap;
        self
    }

    pub fn header(mut self, header: bool) -> Self {
        self.header = header;
        self
    }

    pub fn key_column(mut self, column: Option<usize>) -> Self {
        self.key = column;
        self
    }

    pub fn end(mut self, end: Option<&'a str>) -> Self {
        self.end = end;
        self
    }

    pub fn empty(mut self, empty: Option<&'a str>) -> Self {
        self.empty = empty;
        self
    }
}

struct Line<'a> {
    styles: Styles,
    chosen: Style,
    layout: Layout,
    gap: u16,
    indent: u16,
    lead: u16,
    cursor: &'a str,
    mark_width: u16,
    area: Rect,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Lit {
    No,
    Range,
    Cursor,
}

impl Line<'_> {
    fn paint(&self, paint: Paint, chosen: bool) -> Style {
        match (chosen, self.styles.keep) {
            (false, _) => self.styles.resolve(paint),
            (true, false) => self.styles.selected,
            (true, true) => self.styles.resolve(paint).patch(self.styles.selected),
        }
    }

    fn row(&self, buf: &mut Buffer, y: u16, row: &Row, lit: Lit) -> u16 {
        match row.kind {
            Kind::Item => self.item(buf, y, row, lit),
            Kind::Heading => self.heading(buf, y, row),
            Kind::Blank => 0,
        }
    }

    fn item(&self, buf: &mut Buffer, y: u16, row: &Row, lit: Lit) -> u16 {
        let area = self.area;
        let mut x = area.x;
        let room = area.width;
        let chosen = lit != Lit::No;
        let mut ink = 0;
        if chosen {
            blank(buf, area, x, y, room, self.chosen);
        }
        if lit == Lit::Cursor {
            let style = self.styles.cursor.unwrap_or(self.styles.selected);
            ink = put(buf, area, x, y, self.indent, self.cursor, style);
        }
        x = x.saturating_add(self.indent);
        if let Some(mark) = &row.mark {
            ink = ink.max(put(
                buf,
                area,
                x,
                y,
                self.mark_width,
                &mark.text,
                self.paint(mark.paint, chosen),
            ));
        }
        x = x.saturating_add(self.lead - self.indent);
        let cells = columns_line(buf, area, y, x, &self.layout, self.gap, |at| {
            let cell = row.cells.get(at)?;
            let runs = cell
                .pieces()
                .map(move |(text, paint)| (text, self.paint(paint, chosen)));
            Some((runs, cell.rest))
        });
        ink.max(cells)
    }

    fn heading(&self, buf: &mut Buffer, y: u16, row: &Row) -> u16 {
        let Some(cell) = row.cells.first() else {
            return 0;
        };
        let x = self.area.x.saturating_add(self.indent);
        let room = self.area.width.saturating_sub(self.indent);
        let runs = cell
            .pieces()
            .map(|(text, paint)| (text, self.styles.resolve(paint)));
        put_runs(buf, self.area, x, y, room, runs, false)
    }

    fn header(&self, buf: &mut Buffer, wide: Rect, columns: &[Column], key: Option<usize>) {
        let styles = self.styles;
        let y = wide.y;
        if let Some(style) = styles.header {
            blank(buf, wide, wide.x, y, wide.width, styles.base.patch(style));
        }
        let title = styles.header.unwrap_or(styles.faint);
        let x = self.area.x.saturating_add(self.lead);
        columns_line(buf, self.area, y, x, &self.layout, self.gap, |at| {
            let style = match styles.header_key {
                Some(style) if key == Some(at) => style,
                _ => title,
            };
            Some((iter::once((columns[at].title, style)), false))
        });
    }

    fn note(&self, buf: &mut Buffer, y: u16, text: &str) -> u16 {
        let x = self.area.x.saturating_add(self.indent);
        let room = self.area.width.saturating_sub(self.indent);
        put(buf, self.area, x, y, room, text, self.styles.faint)
    }

    fn bar(&self, buf: &mut Buffer, bar: &Bar, x: u16, y: u16, top: usize, lines: usize) {
        let height = usize::from(self.area.bottom().saturating_sub(y));
        let unit = if bar.halves.is_some() { 2 } else { 1 };
        let (start, size) = thumb(top, lines, height, unit);
        let end = start.saturating_add(size);
        let track = self.styles.track.unwrap_or(self.styles.faint);
        let lit = self
            .styles
            .thumb
            .or(self.styles.cursor)
            .unwrap_or(self.styles.selected);
        let clip = Rect {
            x,
            width: 1,
            ..self.area
        };
        for offset in 0..height {
            let from = offset * unit;
            let upper = (start..end).contains(&from);
            let lower = (start..end).contains(&(from + unit - 1));
            let at = y.saturating_add(small(offset));
            match bar.glyph(upper, lower) {
                Some(glyph) => put(buf, clip, x, at, 1, glyph, lit),
                None => put(buf, clip, x, at, 1, bar.track, track),
            };
        }
    }
}

fn label(
    buf: &mut Buffer,
    count: &Count,
    shown: (usize, usize, usize),
    style: Style,
    line: Rect,
    free: Option<u16>,
) {
    let mut text = Stack::new();
    let (first, last, total) = shown;
    if write!(text, "{}", count.label(first, last, total)).is_err() {
        return;
    }
    let text = text.text();
    if let Some(free) = free
        && line.right().saturating_sub(text::cells(text)) < free
    {
        return;
    }
    let runs = iter::once((text, style));
    put_runs(buf, line, line.x, line.y, line.width, runs, true);
}

impl<S: Source> Widget for ListView<'_, S> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        let area = area.intersection(buf.area);
        if area.is_empty() {
            self.list.place(iter::empty());
            self.list.drawn(None, false, false);
            return;
        }
        let ListView {
            list,
            columns,
            styles,
            cursor,
            gap,
            header,
            key,
            end,
            empty,
            bar,
            count,
        } = self;
        for y in area.top()..area.bottom() {
            blank(buf, area, area.x, y, area.width, styles.base);
        }
        let indent = text::cells(cursor);
        let mark_width = list.mark_width();
        let lead = indent
            .saturating_add(mark_width)
            .saturating_add(u16::from(mark_width > 0));
        let y = area.y.saturating_add(u16::from(header));
        let body = area.bottom().saturating_sub(y);
        let len = list.len();
        let lines = if len == 0 {
            0
        } else {
            len.saturating_add(usize::from(end.is_some()))
        };
        let over = body > 0 && lines > usize::from(body);
        let count = count.filter(|_| over);
        let cut = count.is_some_and(|count| count.place == Place::Line && body > 1);
        let bar = bar.filter(|_| over && area.width > 1);
        let rows = Rect {
            width: area.width - u16::from(bar.is_some()),
            height: area.height - u16::from(cut),
            ..area
        };
        let height = usize::from(rows.bottom().saturating_sub(y));
        let selected = list.follow(height, end.is_some());
        let mut layout = Layout::new(columns, rows.width, lead, gap);
        let fitted = fitted(list, columns, &layout, header, height);
        layout.size(columns, rows.width, lead, gap, &fitted);
        list.place(layout.places(rows.x.saturating_add(lead), rows.right(), gap));
        let line = Line {
            styles,
            chosen: styles.base.patch(styles.selected),
            layout,
            gap,
            indent,
            lead,
            cursor,
            mark_width,
            area: rows,
        };
        if header {
            line.header(buf, Rect { height: 1, ..area }, columns, key);
        }
        if list.is_empty() {
            list.drawn(None, false, false);
            if let Some(text) = empty
                && height > 0
            {
                line.note(buf, y, text);
            }
            return;
        }
        let top = list.top();
        let mut ink = 0;
        for offset in 0..height {
            let index = top.saturating_add(offset);
            let at = y.saturating_add(small(offset));
            if index < len {
                let lit = if selected == Some(index) {
                    Lit::Cursor
                } else if list.in_range(index) {
                    Lit::Range
                } else {
                    Lit::No
                };
                ink = line.row(buf, at, &list.row(index), lit);
                continue;
            }
            ink = 0;
            if let Some(text) = end
                && index == len
            {
                ink = line.note(buf, at, text);
            }
            break;
        }
        let on = len.saturating_sub(top).min(height);
        let shown = (on > 0).then(|| (top + 1, top + on, len));
        list.drawn(shown, cut, bar.is_some());
        if let Some(bar) = bar {
            line.bar(buf, &bar, area.right() - 1, y, top, lines);
        }
        let (Some(count), Some(shown)) = (count, shown) else {
            return;
        };
        let style = styles.count.unwrap_or(styles.faint);
        let (at, free) = match count.place {
            Place::Line if cut => (
                Rect {
                    y: rows.bottom(),
                    height: 1,
                    ..area
                },
                None,
            ),
            _ => {
                let last = y.saturating_add(small(height)).saturating_sub(1);
                let free = if ink == 0 {
                    rows.x
                } else {
                    ink.saturating_add(1)
                };
                (
                    Rect {
                        y: last,
                        height: 1,
                        ..rows
                    },
                    Some(free),
                )
            }
        };
        label(buf, &count, shown, style, at, free);
    }
}
