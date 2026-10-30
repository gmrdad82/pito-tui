use std::iter;
use std::ptr;
use std::time::{Duration, Instant};

use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::Style,
    widgets::{Block, BorderType, Clear, Widget},
};

use crate::fold::ease;
use crate::footer::Hint as Shown;
use crate::keys::chord::Chord;
use crate::keys::keymap::{Keymap, Mode, Now, Row, When};
use crate::keys::sheet::{Outcome, Sheet};
use crate::layout::Spot;
use crate::palette::Palette;
use crate::text::cells;

const LASTING: Duration = Duration::from_millis(180);
const KEYS: u16 = 24;
const NARROWEST: u16 = 24;
const SLASH: &str = "/ ";
const CARET: &str = "▏";
const UP: &str = "↑";
const DOWN: &str = "↓";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Entry<'k> {
    Heading(&'k str),
    Group(&'k str),
    Row(Row<'k>, bool),
    Gap,
    Nothing(&'k str),
}

#[derive(Debug, Clone)]
pub struct Panel<'k> {
    sheet: Sheet,
    entries: Vec<Entry<'k>>,
    map: usize,
    head: u16,
    title: &'k str,
    mode: Option<Mode>,
    when: Vec<When>,
    query: String,
    typing: bool,
    stale: bool,
    keys: u16,
    natural: u16,
    top: u16,
    from: f32,
    since: Option<Instant>,
    still: bool,
}

impl Default for Panel<'_> {
    fn default() -> Self {
        Panel::new()
    }
}

fn width(text: &str) -> u16 {
    u16::try_from(cells(text)).unwrap_or(u16::MAX)
}

fn put(buf: &mut Buffer, x: u16, y: u16, right: u16, text: &str, style: Style) -> u16 {
    if x >= right || text.is_empty() {
        return x;
    }
    let (end, _) = buf.set_stringn(x, y, text, usize::from(right - x), style);
    end.min(right)
}

fn measure(row: &Row, keys: u16) -> u16 {
    let mut total = 3u16
        .saturating_add(keys)
        .saturating_add(2)
        .saturating_add(width(row.does));
    if let Some(note) = row.note {
        total = total.saturating_add(2).saturating_add(width(note));
    }
    if let Some(when) = row.when {
        total = total.saturating_add(3).saturating_add(width(when));
    }
    if !row.modes.is_empty() {
        total = total.saturating_add(3).saturating_add(width(row.modes));
    }
    total
}

impl<'k> Panel<'k> {
    pub fn new() -> Self {
        Panel {
            sheet: Sheet::new(),
            entries: Vec::new(),
            map: 0,
            head: 0,
            title: "",
            mode: None,
            when: Vec::new(),
            query: String::new(),
            typing: false,
            stale: true,
            keys: 0,
            natural: 0,
            top: 0,
            from: 0.0,
            since: None,
            still: false,
        }
    }

    pub fn still(mut self, still: bool) -> Self {
        self.set_still(still);
        self
    }

    pub fn set_still(&mut self, still: bool) {
        self.still = still;
        if still {
            self.since = None;
        }
    }

    pub fn sheet(&self) -> &Sheet {
        &self.sheet
    }

    pub fn is_open(&self) -> bool {
        self.sheet.is_open()
    }

    pub fn toggle(&mut self, moment: Instant) {
        let shown = self.shown(moment);
        self.sheet.toggle();
        self.turned(shown, moment);
    }

    pub fn close(&mut self, moment: Instant) {
        if self.sheet.is_open() {
            self.toggle(moment);
        }
    }

    pub fn key(&mut self, chord: Chord, moment: Instant) -> Outcome {
        let open = self.sheet.is_open();
        let shown = self.shown(moment);
        let outcome = self.sheet.key(chord);
        if outcome == Outcome::Took {
            if open != self.sheet.is_open() {
                self.turned(shown, moment);
            } else if self.sheet.typing() != self.typing || self.sheet.query() != self.query {
                self.stale = true;
            }
        }
        outcome
    }

    pub fn paste(&mut self, text: &str) -> bool {
        if !self.sheet.is_open() {
            return false;
        }
        if self.sheet.typing() {
            let mut query = String::from(self.sheet.query());
            query.extend(text.chars().map(|c| if c.is_control() { ' ' } else { c }));
            self.sheet.search(query);
            self.stale = true;
        }
        true
    }

    fn turned(&mut self, shown: f32, moment: Instant) {
        self.stale = true;
        self.from = shown;
        self.since = (!self.still).then_some(moment);
    }

    pub fn shown(&self, moment: Instant) -> f32 {
        let to = if self.sheet.is_open() { 1.0 } else { 0.0 };
        let Some(since) = self.since else {
            return to;
        };
        let t = moment.saturating_duration_since(since).as_secs_f32() / LASTING.as_secs_f32();
        if t >= 1.0 {
            return to;
        }
        self.from + (to - self.from) * ease(t)
    }

    pub fn animating(&self, moment: Instant) -> bool {
        self.since
            .is_some_and(|since| moment.saturating_duration_since(since) < LASTING)
    }

    fn build<A>(&mut self, keymap: &'k Keymap<A>, now: Now<'_>) {
        self.stale = false;
        self.map = ptr::from_ref(keymap).addr();
        self.mode = Some(now.mode);
        self.when.clear();
        self.when.extend_from_slice(now.when);
        self.query.clear();
        self.query.push_str(self.sheet.query());
        self.typing = self.sheet.typing();
        let words = keymap.sheet_words();
        self.title = &words.title;
        let groups = || keymap.groups.iter().chain(iter::once(&keymap.help));
        let rows = || {
            groups().flat_map(|group| {
                group
                    .binds()
                    .iter()
                    .map(move |bind| bind.row(group.name(), true))
            })
        };
        self.keys = rows()
            .map(|row| width(row.keys))
            .max()
            .unwrap_or(0)
            .min(KEYS);
        let headings = keymap
            .order()
            .iter()
            .map(|mode| width(mode.0))
            .chain([
                width(&words.elsewhere),
                width(&words.nothing).saturating_add(1),
            ])
            .chain(groups().map(|group| width(group.name()).saturating_add(1)));
        let keys = self.keys;
        self.natural = rows()
            .map(|row| measure(&row, keys))
            .chain(headings)
            .chain([width(self.title).saturating_add(4)])
            .max()
            .unwrap_or(0);
        let view = self.sheet.view(keymap, now);
        self.entries.clear();
        self.head = u16::from(self.typing || !self.query.is_empty());
        if !view.here.is_empty() && !now.mode.0.is_empty() {
            self.entries.push(Entry::Heading(now.mode.0));
        }
        for section in &view.here {
            self.entries.push(Entry::Group(section.group));
            self.entries
                .extend(section.rows.iter().map(|row| Entry::Row(*row, false)));
        }
        if !view.elsewhere.is_empty() {
            if !view.here.is_empty() {
                self.entries.push(Entry::Gap);
            }
            self.entries.push(Entry::Heading(&words.elsewhere));
        }
        for section in &view.elsewhere {
            self.entries.push(Entry::Group(section.group));
            self.entries
                .extend(section.rows.iter().map(|row| Entry::Row(*row, true)));
        }
        if view.is_empty() {
            self.entries.push(Entry::Nothing(&words.nothing));
        }
    }

    pub fn draw<A>(
        &mut self,
        buf: &mut Buffer,
        area: Rect,
        palette: &Palette,
        keymap: &'k Keymap<A>,
        now: Now<'_>,
        moment: Instant,
    ) {
        let moved = self.mode != Some(now.mode)
            || self.when != now.when
            || self.map != ptr::from_ref(keymap).addr();
        if self.sheet.is_open() && (self.stale || moved) {
            self.build(keymap, now);
        }
        let shown = self.shown(moment);
        if shown <= 0.0 || self.mode.is_none() || area.is_empty() {
            return;
        }
        let rows = u16::try_from(self.entries.len()).unwrap_or(u16::MAX);
        let wide = self
            .natural
            .saturating_add(4)
            .max(NARROWEST)
            .min(area.width);
        let full = rows
            .saturating_add(self.head)
            .saturating_add(2)
            .min(area.height);
        let height = ((f32::from(full) * shown).round() as u16).min(full);
        if height < 2 || wide < 6 {
            return;
        }
        let page = full.saturating_sub(2).saturating_sub(self.head).max(1);
        if self.sheet.is_open() {
            self.sheet.fit(rows.div_ceil(page).max(1));
            self.top = self
                .sheet
                .page()
                .saturating_mul(page)
                .min(rows.saturating_sub(page));
        }
        let place = Spot::MIDDLE.place(area, wide, height);
        Clear.render(place, buf);
        Block::bordered()
            .border_type(BorderType::Rounded)
            .border_style(palette.rule)
            .style(palette.base)
            .render(place, buf);
        let edge = place
            .right()
            .saturating_sub(if self.top > 0 { 3 } else { 2 });
        let x = put(buf, place.x + 2, place.y, edge, " ", palette.selected);
        let x = put(buf, x, place.y, edge, self.title, palette.selected);
        put(buf, x, place.y, edge, " ", palette.selected);
        let head = self.head.min(height - 2);
        let body = height - 2 - head;
        let top = self.top;
        let mut pen = Pen {
            buf: &mut *buf,
            left: place.x + 2,
            right: place.right().saturating_sub(2),
            keys: self.keys,
            palette,
            query: &self.query,
            typing: self.typing,
        };
        if head > 0 {
            pen.query(place.y + 1);
        }
        let shown = self.entries.iter().skip(usize::from(top));
        for (y, entry) in (place.y + 1 + head..).zip(shown.take(usize::from(body))) {
            pen.line(*entry, y);
        }
        let mark = place.right().saturating_sub(3);
        if top > 0 {
            put(buf, mark, place.y, place.right(), UP, palette.accent);
        }
        if top.saturating_add(body) < rows {
            put(
                buf,
                mark,
                place.bottom() - 1,
                place.right(),
                DOWN,
                palette.accent,
            );
        }
    }
}

struct Pen<'a> {
    buf: &'a mut Buffer,
    left: u16,
    right: u16,
    keys: u16,
    palette: &'a Palette,
    query: &'a str,
    typing: bool,
}

impl Pen<'_> {
    fn put(&mut self, x: u16, y: u16, text: &str, style: Style) -> u16 {
        put(self.buf, x, y, self.right, text, style)
    }

    fn query(&mut self, y: u16) {
        let palette = self.palette;
        let x = self.put(self.left, y, SLASH, palette.accent);
        let caret = if self.typing { width(CARET) } else { 0 };
        let room = usize::from(self.right.saturating_sub(x).saturating_sub(caret));
        let query = self.query;
        let from = query
            .char_indices()
            .map(|(at, _)| at)
            .find(|at| cells(&query[*at..]) <= room)
            .unwrap_or(query.len());
        let x = self.put(x, y, &query[from..], palette.ink);
        if self.typing {
            self.put(x, y, CARET, palette.accent);
        }
    }

    fn line(&mut self, entry: Entry, y: u16) {
        let palette = self.palette;
        let x = self.left;
        match entry {
            Entry::Heading(text) => {
                self.put(x, y, text, palette.selected);
            }
            Entry::Group(name) => {
                self.put(x + 1, y, name, palette.accent);
            }
            Entry::Row(row, elsewhere) => {
                let (key, does) = if row.live {
                    (palette.strong, palette.ink)
                } else {
                    (palette.muted, palette.muted)
                };
                let start = x + 3;
                let stop = self.right.min(start.saturating_add(self.keys));
                put(self.buf, start, y, stop, row.keys, key);
                let mut at = start.saturating_add(self.keys).saturating_add(2);
                at = self.put(at, y, row.does, does);
                if let Some(note) = row.note {
                    at = self.put(at, y, ": ", palette.muted);
                    at = self.put(at, y, note, palette.muted);
                }
                if let Some(when) = row.when {
                    at = self.put(at, y, " (", palette.muted);
                    at = self.put(at, y, when, palette.muted);
                    at = self.put(at, y, ")", palette.muted);
                }
                if elsewhere && !row.modes.is_empty() {
                    at = self.put(at, y, " · ", palette.muted);
                    self.put(at, y, row.modes, palette.muted);
                }
            }
            Entry::Gap => {}
            Entry::Nothing(text) => {
                self.put(x + 1, y, text, palette.muted);
            }
        }
    }
}

pub(crate) trait Map {
    fn hints<'k>(&'k self, now: Now<'_>, into: &mut Vec<Shown<'k>>);

    fn draw(
        &'static self,
        panel: &mut Panel<'static>,
        buf: &mut Buffer,
        area: Rect,
        palette: &Palette,
        now: Now<'_>,
        moment: Instant,
    );
}

impl<A> Map for Keymap<A> {
    fn hints<'k>(&'k self, now: Now<'_>, into: &mut Vec<Shown<'k>>) {
        self.each_hint(now, |hint| into.push(hint.into()));
    }

    fn draw(
        &'static self,
        panel: &mut Panel<'static>,
        buf: &mut Buffer,
        area: Rect,
        palette: &Palette,
        now: Now<'_>,
        moment: Instant,
    ) {
        panel.draw(buf, area, palette, self, now, moment);
    }
}
