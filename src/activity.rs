use std::borrow::Cow;
use std::collections::HashMap;
use std::fmt;
use std::sync::Arc;
use std::time::{Duration, Instant};

use crossterm::event::KeyEvent;
use pito_footer::{Hint, InputBar, Key};
use pito_header::Section;
use pito_list::{Cell, Column, List, Mark, Row, Step};
use ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::Paragraph,
};

use crate::chrome::message;
use crate::log::{Lines, Log, Turn};
use crate::palette::Palette;
use crate::progress::{SPIN, bar, share, spinner};
use crate::screen::{Cx, Screen};
use crate::text::{cells, clip, fit};

pub const LINGER: Duration = Duration::from_secs(3);
const BAR: usize = 12;
const GAUGE: usize = BAR + 6;
const GAP: usize = 2;
const NARROW: usize = 60;
const SECOND: Duration = Duration::from_secs(1);

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
#[non_exhaustive]
pub enum State {
    #[default]
    Running,
    Waiting,
    Done,
    Failed,
    Stopped,
}

impl State {
    pub fn finished(self) -> bool {
        matches!(self, State::Done | State::Failed | State::Stopped)
    }
}

#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub struct Activity {
    pub id: u64,
    pub label: String,
    pub state: State,
    pub progress: Option<f64>,
    pub estimate: bool,
    pub started: Instant,
    pub ended: Option<Instant>,
    pub status: String,
    pub detail: Lines,
}

impl Activity {
    pub fn new(id: u64, label: impl Into<String>, started: Instant) -> Self {
        Activity {
            id,
            label: label.into(),
            state: State::Running,
            progress: None,
            estimate: false,
            started,
            ended: None,
            status: String::new(),
            detail: Arc::new(Vec::new()),
        }
    }

    pub fn state(mut self, state: State) -> Self {
        self.state = state;
        self
    }

    pub fn progress(mut self, fraction: impl Into<Option<f64>>) -> Self {
        self.progress = fraction.into();
        self
    }

    pub fn estimate(mut self, estimate: bool) -> Self {
        self.estimate = estimate;
        self
    }

    pub fn ended(mut self, at: Instant) -> Self {
        self.ended = Some(at);
        self
    }

    pub fn status(mut self, text: impl Into<String>) -> Self {
        self.status = text.into();
        self
    }

    pub fn detail(mut self, lines: impl IntoIterator<Item = Line<'static>>) -> Self {
        self.detail = Arc::new(lines.into_iter().collect());
        self
    }

    pub fn shared(mut self, lines: Lines) -> Self {
        self.detail = lines;
        self
    }

    pub fn took(&self, now: Instant) -> Duration {
        self.ended
            .unwrap_or(now)
            .saturating_duration_since(self.started)
    }

    fn shown(&self, now: Instant) -> bool {
        !self.state.finished() || self.ended.is_none_or(|ended| now < ended + LINGER)
    }

    fn wake(&self, now: Instant) -> Option<Instant> {
        if self.state.finished() {
            return self
                .ended
                .map(|ended| ended + LINGER)
                .filter(|leaves| *leaves > now);
        }
        let step = if self.state == State::Running {
            SPIN
        } else {
            SECOND
        };
        let gone = now.saturating_duration_since(self.started).as_nanos();
        let steps = u32::try_from(gone / step.as_nanos() + 1).ok()?;
        Some(self.started + step * steps)
    }
}

type Count = Arc<dyn Fn(usize) -> String + Send + Sync>;
type Elapsed = Arc<dyn Fn(Duration) -> String + Send + Sync>;
type Facts = Arc<dyn Fn(&[Activity]) -> Vec<(String, Style)> + Send + Sync>;

#[derive(Clone)]
#[non_exhaustive]
pub struct Activities {
    pub(crate) section: Section,
    pub(crate) keys: &'static [Key],
    pub(crate) band: bool,
    pub(crate) hints: Vec<Hint<'static>>,
    pub(crate) detail_hints: Vec<Hint<'static>>,
    pub(crate) log: Log,
    pub(crate) empty: Cow<'static, str>,
    pub(crate) more: Count,
    pub(crate) elapsed: Elapsed,
    pub(crate) facts: Facts,
}

impl Activities {
    pub fn new(section: Section) -> Self {
        Activities {
            section,
            keys: &[],
            band: true,
            hints: Vec::new(),
            detail_hints: Vec::new(),
            log: Log::new(""),
            empty: Cow::Borrowed(""),
            more: Arc::new(|_| String::new()),
            elapsed: Arc::new(|_| String::new()),
            facts: Arc::new(|_| Vec::new()),
        }
    }

    pub fn keys(mut self, keys: &'static [Key]) -> Self {
        self.keys = keys;
        self
    }

    pub fn band(mut self, band: bool) -> Self {
        self.band = band;
        self
    }

    pub fn hints(mut self, hints: impl IntoIterator<Item = Hint<'static>>) -> Self {
        self.hints = hints.into_iter().collect();
        self
    }

    pub fn detail_hints(mut self, hints: impl IntoIterator<Item = Hint<'static>>) -> Self {
        self.detail_hints = hints.into_iter().collect();
        self
    }

    pub fn log(mut self, log: Log) -> Self {
        self.log = log;
        self
    }

    pub fn empty(mut self, text: impl Into<Cow<'static, str>>) -> Self {
        self.empty = text.into();
        self
    }

    pub fn more(mut self, words: impl Fn(usize) -> String + Send + Sync + 'static) -> Self {
        self.more = Arc::new(words);
        self
    }

    pub fn elapsed(mut self, words: impl Fn(Duration) -> String + Send + Sync + 'static) -> Self {
        self.elapsed = Arc::new(words);
        self
    }

    pub fn facts(
        mut self,
        facts: impl Fn(&[Activity]) -> Vec<(String, Style)> + Send + Sync + 'static,
    ) -> Self {
        self.facts = Arc::new(facts);
        self
    }
}

impl fmt::Debug for Activities {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("Activities")
            .field("section", &self.section.name())
            .field("keys", &self.keys)
            .field("band", &self.band)
            .field("empty", &self.empty)
            .finish_non_exhaustive()
    }
}

#[derive(Debug, Clone, PartialEq)]
pub(crate) enum Change {
    Put(Activity),
    All(Vec<Activity>),
    Forget(u64),
    Band(bool),
    Run(Activity),
    Ran(Activity),
    End(u64, State),
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Owner {
    Shell,
    App,
}

pub(crate) struct Board {
    pub(crate) words: Activities,
    pub(crate) list: Vec<Activity>,
    runs: HashMap<u64, Owner>,
    rows: List,
    ids: Vec<u64>,
    open: Option<u64>,
    log: Log,
    now: Instant,
    palette: Palette,
}

fn gauge(activity: &Activity) -> String {
    match activity.progress.filter(|_| !activity.state.finished()) {
        Some(fraction) => format!(
            "{} {:>5}",
            bar(fraction, BAR),
            share(fraction, activity.estimate)
        ),
        None => String::new(),
    }
}

fn mark(activity: &Activity, now: Instant, palette: &Palette) -> (&'static str, Style) {
    match activity.state {
        State::Running => (
            spinner(now.saturating_duration_since(activity.started)),
            palette.accent,
        ),
        State::Waiting => ("⧗", palette.muted),
        State::Done => ("✓", palette.good),
        State::Failed => ("✗", palette.bad),
        State::Stopped => ("■", palette.muted),
    }
}

fn metered(activity: &Activity, palette: &Palette) -> Style {
    if activity.estimate {
        palette.muted
    } else {
        palette.accent
    }
}

fn tone(activity: &Activity, palette: &Palette) -> Style {
    match activity.state {
        State::Done => palette.good,
        State::Failed => palette.bad,
        _ => palette.muted,
    }
}

fn newest(list: &[Activity]) -> Vec<&Activity> {
    let mut sorted: Vec<&Activity> = list.iter().collect();
    sorted.sort_by(|a, b| b.started.cmp(&a.started).then(b.id.cmp(&a.id)));
    sorted
}

impl Board {
    pub(crate) fn new(words: Activities, now: Instant) -> Self {
        Board {
            log: words.log.clone(),
            words,
            list: Vec::new(),
            runs: HashMap::new(),
            rows: List::new(),
            ids: Vec::new(),
            open: None,
            now,
            palette: Palette::new(Color::Reset),
        }
    }

    fn kept(&self, id: u64) -> Option<usize> {
        self.list.iter().position(|kept| kept.id == id)
    }

    fn stamp(&self, mut activity: Activity, now: Instant) -> Activity {
        if activity.state.finished() && activity.ended.is_none() {
            let before = self.kept(activity.id).and_then(|at| self.list[at].ended);
            activity.ended = before.or(Some(now));
        }
        activity
    }

    fn place(&mut self, activity: Activity, now: Instant) {
        let activity = self.stamp(activity, now);
        match self.kept(activity.id) {
            Some(at) => self.list[at] = activity,
            None => self.list.push(activity),
        }
    }

    fn put(&mut self, mut activity: Activity, now: Instant) {
        if let Some(owner) = self.runs.get_mut(&activity.id) {
            *owner = Owner::App;
            if let Some(at) = self.kept(activity.id) {
                activity.detail = Arc::clone(&self.list[at].detail);
            }
        }
        self.place(activity, now);
    }

    fn run(&mut self, mut activity: Activity, now: Instant) {
        let waiting = self
            .kept(activity.id)
            .map(|at| &self.list[at])
            .filter(|kept| !kept.state.finished());
        if let Some(kept) = waiting {
            activity.started = kept.started;
        }
        self.runs.insert(activity.id, Owner::Shell);
        self.place(activity, now);
    }

    fn ran(&mut self, activity: Activity) {
        let (Some(at), Some(&owner)) = (self.kept(activity.id), self.runs.get(&activity.id)) else {
            return;
        };
        let kept = &mut self.list[at];
        kept.detail = activity.detail;
        if owner == Owner::Shell {
            kept.state = activity.state;
            kept.progress = activity.progress;
            kept.status = activity.status;
            kept.ended = activity.ended;
        }
    }

    fn end(&mut self, id: u64, state: State, now: Instant) {
        let Some(at) = self.kept(id).filter(|_| self.runs.contains_key(&id)) else {
            return;
        };
        let kept = &mut self.list[at];
        if !kept.state.finished() {
            kept.state = state;
            kept.progress = None;
            kept.ended = Some(now);
        }
    }

    pub(crate) fn change(&mut self, change: Change, now: Instant) {
        match change {
            Change::Put(activity) => self.put(activity, now),
            Change::All(list) => {
                let runs = &self.runs;
                self.list.retain(|kept| runs.contains_key(&kept.id));
                for activity in list {
                    self.put(activity, now);
                }
            }
            Change::Forget(id) => {
                self.list.retain(|kept| kept.id != id);
                self.runs.remove(&id);
            }
            Change::Band(band) => self.words.band = band,
            Change::Run(activity) => self.run(activity, now),
            Change::Ran(activity) => self.ran(activity),
            Change::End(id, state) => self.end(id, state, now),
        }
        if self
            .open
            .is_some_and(|id| self.list.iter().all(|kept| kept.id != id))
        {
            self.open = None;
        }
    }

    fn opened(&self) -> Option<&Activity> {
        let id = self.open?;
        self.list.iter().find(|kept| kept.id == id)
    }

    pub(crate) fn banded(&self, now: Instant) -> Vec<&Activity> {
        if !self.words.band {
            return Vec::new();
        }
        let mut shown = newest(&self.list);
        shown.retain(|activity| activity.shown(now));
        shown
    }

    pub(crate) fn wake(&self, now: Instant, open: bool) -> Option<Instant> {
        let shown = if open {
            self.list.iter().collect()
        } else {
            self.banded(now)
        };
        shown
            .into_iter()
            .filter_map(|activity| activity.wake(now))
            .min()
    }

    pub(crate) fn band(&self, frame: &mut Frame, area: Rect, palette: &Palette, now: Instant) {
        let shown = self.banded(now);
        let room = usize::from(area.height);
        if shown.is_empty() || room == 0 {
            return;
        }
        let hidden = shown.len().saturating_sub(room);
        let more = if hidden > 0 {
            (self.words.more)(hidden + 1)
        } else {
            String::new()
        };
        let kept = if more.is_empty() { room } else { room - 1 };
        let shown = &shown[..shown.len().min(kept)];
        let width = usize::from(area.width);
        let label = shown
            .iter()
            .map(|activity| cells(&activity.label))
            .max()
            .unwrap_or(0)
            .min(width / 3);
        let took: Vec<String> = shown
            .iter()
            .map(|activity| (self.words.elapsed)(activity.took(now)))
            .collect();
        let time = took.iter().map(|text| cells(text)).max().unwrap_or(0);
        let gauged = width >= NARROW
            && shown
                .iter()
                .any(|activity| activity.progress.is_some() && !activity.state.finished());
        let mut lines: Vec<Line> = shown
            .iter()
            .zip(&took)
            .map(|(activity, took)| {
                let (sign, sign_style) = mark(activity, now, palette);
                let fade = |style: Style| {
                    if activity.state.finished() {
                        style.add_modifier(Modifier::DIM)
                    } else {
                        style
                    }
                };
                let mut spans = vec![
                    Span::styled(format!("{sign} "), fade(sign_style)),
                    Span::styled(
                        fit(&activity.label, label),
                        fade(palette.ink.add_modifier(Modifier::BOLD)),
                    ),
                    Span::raw(" ".repeat(GAP)),
                ];
                let mut used = 2 + label + GAP;
                if gauged {
                    spans.push(Span::styled(
                        fit(&gauge(activity), GAUGE),
                        fade(metered(activity, palette)),
                    ));
                    spans.push(Span::raw(" ".repeat(GAP)));
                    used += GAUGE + GAP;
                }
                let rest = width.saturating_sub(used + GAP + time);
                spans.push(Span::styled(
                    fit(&activity.status, rest),
                    fade(tone(activity, palette)),
                ));
                spans.push(Span::styled(
                    format!("{}{:>time$}", " ".repeat(GAP), clip(took, time)),
                    fade(palette.muted),
                ));
                Line::from(spans)
            })
            .collect();
        if !more.is_empty() {
            lines.push(Line::styled(clip(&more, width), palette.muted));
        }
        frame.render_widget(Paragraph::new(lines), area);
    }

    fn sync(&mut self) {
        let palette = &self.palette;
        let selected = self
            .rows
            .selected()
            .and_then(|index| self.ids.get(index))
            .copied();
        let sorted = newest(&self.list);
        self.ids = sorted.iter().map(|activity| activity.id).collect();
        let rows: Vec<Row> = sorted
            .iter()
            .map(|activity| {
                let (sign, style) = mark(activity, self.now, palette);
                Row::new([
                    Cell::new(activity.label.clone()),
                    Cell::new(gauge(activity)).style(metered(activity, palette)),
                    Cell::new(activity.status.clone()).style(tone(activity, palette)),
                    Cell::new((self.words.elapsed)(activity.took(self.now))).faint(),
                ])
                .mark(Mark::new(sign).style(style))
            })
            .collect();
        self.rows.set_rows(rows);
        if let Some(index) = selected.and_then(|id| self.ids.iter().position(|kept| *kept == id)) {
            self.rows.select(index);
        }
    }
}

impl<E: Send + 'static> Screen<E> for Board {
    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        self.palette = *palette;
        if let Some(lines) = self.opened().map(|activity| Arc::clone(&activity.detail)) {
            self.log.set_lines(lines);
            self.log.draw(frame, area, palette);
            return;
        }
        if self.list.is_empty() {
            let line = Line::styled(self.words.empty.to_string(), palette.ink);
            message(frame, area, line);
            return;
        }
        self.sync();
        let columns = [
            Column::new("", 6, 0).fit(28).pinned(),
            Column::new("", 0, 0).fit(18).priority(1),
            Column::new("", 8, 0).flex(),
            Column::new("", 2, 0).fit(12).right().pinned(),
        ];
        let view = palette.view(&mut self.rows, &columns).header(false);
        frame.render_widget(view, area);
    }

    fn moment(&mut self, now: Instant) {
        self.now = now;
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, E>) {
        if let Some(lines) = self.opened().map(|activity| Arc::clone(&activity.detail)) {
            self.log.set_lines(lines);
            if let Turn::Copy(text) = self.log.key(key) {
                cx.copy(text);
            }
            return;
        }
        self.sync();
        if let Step::Open(index) = self.rows.key(key.into()) {
            self.open = self.ids.get(index).copied();
            self.log = self.words.log.clone();
        }
    }

    fn paste(&mut self, text: &str, _cx: &mut Cx<'_, E>) {
        if self.open.is_some() {
            self.log.paste(text);
        }
    }

    fn typing(&self) -> bool {
        self.open.is_some() && self.log.typing()
    }

    fn input(&self) -> Option<InputBar<'_>> {
        self.open.and_then(|_| self.log.bar())
    }

    fn back(&mut self, _cx: &mut Cx<'_, E>) {
        self.open = None;
    }

    fn crumb(&self) -> Option<String> {
        self.opened().map(|activity| activity.label.clone())
    }

    fn selected(&self) -> usize {
        self.rows.selected().unwrap_or(0)
    }

    fn hints(&self) -> Vec<Hint<'_>> {
        if self.open.is_some() {
            self.words.detail_hints.clone()
        } else {
            self.words.hints.clone()
        }
    }

    fn facts(&self) -> Vec<(String, Style)> {
        (self.words.facts)(&self.list)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn kept(board: &Board, id: u64) -> &Activity {
        board.list.iter().find(|kept| kept.id == id).unwrap()
    }

    fn ran(id: u64, status: &str, line: &str, now: Instant) -> Activity {
        Activity::new(id, "sim", now)
            .status(status)
            .detail([Line::from(line.to_string())])
    }

    #[test]
    fn an_app_that_posts_for_a_command_takes_its_band_and_the_command_keeps_its_detail() {
        let now = Instant::now();
        let later = now + SECOND;
        let mut board = Board::new(Activities::new(Section::new("Jobs")), now);
        board.change(
            Change::Put(Activity::new(1, "sim", now).state(State::Waiting)),
            now,
        );
        board.change(Change::Put(Activity::new(9, "mine", now)), now);
        board.change(Change::Run(Activity::new(1, "sim", later)), later);
        board.change(Change::Run(Activity::new(2, "sim", later)), later);
        assert_eq!(kept(&board, 1).started, now);
        assert_eq!(kept(&board, 2).started, later);

        board.change(Change::Ran(ran(1, "build", "one", later)), later);
        assert_eq!(kept(&board, 1).status, "build");

        let mine = Activity::new(1, "sim", now).status("week 3").progress(0.25);
        board.change(Change::Put(mine), later);
        assert_eq!(kept(&board, 1).detail[0].to_string(), "one");
        board.change(Change::Ran(ran(1, "run", "two", later)), later);
        let shown = kept(&board, 1);
        assert_eq!(
            (shown.status.as_str(), shown.progress),
            ("week 3", Some(0.25))
        );
        assert_eq!(shown.detail[0].to_string(), "two");

        board.change(Change::All(vec![Activity::new(7, "other", now)]), later);
        let mut ids: Vec<u64> = board.list.iter().map(|kept| kept.id).collect();
        ids.sort_unstable();
        assert_eq!(ids, [1, 2, 7]);

        let stopped = Activity::new(1, "sim", now)
            .state(State::Stopped)
            .status("stopped from the lab");
        board.change(Change::Put(stopped), later);
        board.change(Change::End(1, State::Failed), later);
        assert_eq!(kept(&board, 1).state, State::Stopped);
        board.change(Change::Put(Activity::new(2, "sim", now)), later);
        board.change(Change::End(2, State::Done), later);
        assert_eq!(
            (kept(&board, 2).state, kept(&board, 2).ended),
            (State::Done, Some(later))
        );

        board.change(Change::Forget(1), later);
        board.change(Change::Ran(ran(1, "run", "three", later)), later);
        assert!(board.list.iter().all(|kept| kept.id != 1));
    }
}
