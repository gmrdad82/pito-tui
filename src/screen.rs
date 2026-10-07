use std::any::Any;
use std::borrow::Cow;
use std::process::Command;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::mpsc::Sender;
use std::time::Instant;

use crossterm::event::{KeyEvent, MouseEvent};
use pito_footer::{Confirm, Hint, InputBar, Notice, Tone};
use ratatui::{Frame, layout::Rect, style::Style, text::Line};

use crate::activity::{Activity, Change};
use crate::command::{Heard, Run, Stream};
use crate::copy::COPY_MAX;
use crate::head::Focus;
use crate::label::Span;
use crate::layer::{Event as Layered, Modal, Takeover, Toast};
use crate::layout::Layout;
use crate::model::Item;
use crate::palette::Palette;
use crate::pick::Pick;
use crate::wake::{Wake, Waker};
use crate::words::Words;

pub type Job<E> = Box<dyn FnOnce() -> E + Send + 'static>;

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Phase<'a> {
    Ready,
    Loading(Cow<'a, str>),
    Trouble(Cow<'a, [Line<'a>]>),
}

pub trait Screen<E>: Any {
    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette);

    fn moment(&mut self, _now: Instant) {}

    fn phase(&self) -> Phase<'_> {
        Phase::Ready
    }

    fn load(&mut self) -> Option<Job<E>> {
        None
    }

    fn loaded(&mut self, _event: E, _cx: &mut Cx<'_, E>) {}

    fn sources(&self) -> &[&str] {
        &[]
    }

    fn cancel(&mut self) {}

    fn start(&mut self, _cx: &mut Cx<'_, E>) {}

    fn entered(&mut self, _cx: &mut Cx<'_, E>) {}

    fn key(&mut self, _key: KeyEvent, _cx: &mut Cx<'_, E>) {}

    fn paste(&mut self, _text: &str, _cx: &mut Cx<'_, E>) {}

    fn mouse(&mut self, _mouse: MouseEvent, _area: Rect, _cx: &mut Cx<'_, E>) {}

    fn event(&mut self, _event: E, _cx: &mut Cx<'_, E>) {}

    fn answer(&mut self, _yes: bool, _cx: &mut Cx<'_, E>) {}

    fn picked(&mut self, _choice: Option<usize>, _cx: &mut Cx<'_, E>) {}

    fn heard(&mut self, _id: u64, _heard: Heard, _cx: &mut Cx<'_, E>) {}

    fn layer(&mut self, _id: u64, _event: Layered, _cx: &mut Cx<'_, E>) {}

    fn claim(&mut self, _key: KeyEvent, _cx: &mut Cx<'_, E>) -> bool {
        false
    }

    fn back(&mut self, _cx: &mut Cx<'_, E>) {}

    fn hints(&self) -> Vec<Hint<'_>> {
        Vec::new()
    }

    fn facts(&self) -> Vec<(String, Style)> {
        Vec::new()
    }

    fn status(&self) -> Option<(String, Style)> {
        None
    }

    fn status_parts(&self, _room: u16) -> Vec<(String, Style)> {
        self.status().into_iter().collect()
    }

    fn lead(&self, _room: u16) -> Vec<(String, Style)> {
        Vec::new()
    }

    fn left(&self, _room: u16, _help: Option<&str>) -> Option<Vec<(String, Style)>> {
        None
    }

    fn after_tabs(&self) -> Vec<(String, Style)> {
        Vec::new()
    }

    fn caption(&self) -> Option<Vec<(String, Style)>> {
        None
    }

    fn notice(&self) -> Option<Notice<'_>> {
        None
    }

    fn legend(&self) -> Option<&str> {
        None
    }

    fn typing(&self) -> bool {
        false
    }

    fn input(&self) -> Option<InputBar<'_>> {
        None
    }

    fn crumb(&self) -> Option<String> {
        None
    }

    fn selected(&self) -> usize {
        0
    }

    fn busy(&self) -> usize {
        0
    }

    fn animating(&self) -> bool {
        false
    }

    fn deadline(&self) -> Option<Instant> {
        None
    }

    fn tick(&mut self, _cx: &mut Cx<'_, E>) {}
}

pub trait Band {
    fn height(&self, room: u16, now: Instant) -> u16;

    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette, now: Instant);

    fn deadline(&self, _now: Instant) -> Option<Instant> {
        None
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct Asking {
    pub(crate) screen: usize,
    pub(crate) question: Cow<'static, str>,
    pub(crate) confirm: Confirm,
}

pub(crate) struct Picking {
    pub(crate) screen: usize,
    pub(crate) pick: Pick,
}

pub(crate) struct Opened {
    pub(crate) screen: usize,
    pub(crate) modal: Modal,
}

pub(crate) type Shown = Box<dyn Fn(Item) -> bool>;

pub(crate) struct Outbox<E> {
    pub(crate) sender: Sender<Wake<E>>,
    pub(crate) jobs: Vec<(usize, u64, Job<E>)>,
    pub(crate) epoch: Arc<AtomicU64>,
    pub(crate) bump: bool,
    pub(crate) copies: Vec<String>,
    pub(crate) said: Option<(Cow<'static, str>, Tone)>,
    pub(crate) asking: Option<Asking>,
    pub(crate) picking: Option<Picking>,
    pub(crate) go: Option<usize>,
    pub(crate) changes: Vec<Change>,
    pub(crate) runs: Vec<Run>,
    pub(crate) stops: Vec<u64>,
    pub(crate) quit: bool,
    pub(crate) reload: Vec<usize>,
    pub(crate) changed: Vec<String>,
    pub(crate) palette: Option<Palette>,
    pub(crate) words: Option<Words>,
    pub(crate) hints: Option<Vec<Hint<'static>>>,
    pub(crate) title: Option<Vec<Span>>,
    pub(crate) layout: Option<Layout>,
    pub(crate) role: Option<Option<Cow<'static, str>>>,
    pub(crate) show: Option<Shown>,
    pub(crate) group: Option<usize>,
    pub(crate) toasts: Vec<Toast>,
    pub(crate) modals: Vec<Opened>,
    pub(crate) takeover: Option<(usize, Takeover)>,
    pub(crate) gate: Option<bool>,
}

impl<E> Outbox<E> {
    pub(crate) fn new(sender: Sender<Wake<E>>) -> Self {
        Outbox {
            sender,
            jobs: Vec::new(),
            epoch: Arc::new(AtomicU64::new(0)),
            bump: false,
            copies: Vec::new(),
            said: None,
            asking: None,
            picking: None,
            go: None,
            changes: Vec::new(),
            runs: Vec::new(),
            stops: Vec::new(),
            quit: false,
            reload: Vec::new(),
            changed: Vec::new(),
            palette: None,
            words: None,
            hints: None,
            title: None,
            layout: None,
            role: None,
            show: None,
            group: None,
            toasts: Vec::new(),
            modals: Vec::new(),
            takeover: None,
            gate: None,
        }
    }

    pub(crate) fn stamp(&self) -> u64 {
        self.epoch.load(Ordering::SeqCst)
    }
}

pub struct Cx<'a, E> {
    pub(crate) screen: usize,
    pub(crate) now: Instant,
    pub(crate) key: Option<KeyEvent>,
    pub(crate) focus: Focus,
    pub(crate) typing: bool,
    pub(crate) out: &'a mut Outbox<E>,
}

impl<E: Send + 'static> Cx<'_, E> {
    pub fn screen(&self) -> usize {
        self.screen
    }

    pub fn now(&self) -> Instant {
        self.now
    }

    pub fn key(&self) -> Option<KeyEvent> {
        self.key
    }

    pub fn focus(&self) -> Focus {
        self.focus
    }

    pub fn typing(&self) -> bool {
        self.typing
    }

    pub fn detach(&mut self, work: impl FnOnce() -> E + Send + 'static) {
        let stamp = self.out.stamp();
        self.out.jobs.push((self.screen, stamp, Box::new(work)));
    }

    pub fn waker(&self) -> Waker<E> {
        Waker::new(self.out.sender.clone(), self.screen).epoch(Arc::clone(&self.out.epoch))
    }

    pub fn epoch(&mut self) {
        self.out.bump = true;
    }

    pub fn say(&mut self, text: impl Into<Cow<'static, str>>, tone: Tone) {
        self.out.said = Some((text.into(), tone));
    }

    pub fn hush(&mut self) {
        self.out.said = None;
    }

    pub fn confirm(&mut self, question: impl Into<Cow<'static, str>>) {
        self.confirm_with(question, Confirm::new());
    }

    pub fn confirm_with(&mut self, question: impl Into<Cow<'static, str>>, confirm: Confirm) {
        self.out.asking = Some(Asking {
            screen: self.screen,
            question: question.into(),
            confirm,
        });
    }

    pub fn pick(&mut self, pick: Pick) {
        self.out.picking = Some(Picking {
            screen: self.screen,
            pick,
        });
    }

    pub fn copy(&mut self, text: impl Into<String>) -> bool {
        let text = text.into();
        if text.len() > COPY_MAX {
            return false;
        }
        self.out.copies.push(text);
        true
    }

    pub fn go(&mut self, screen: usize) {
        self.out.go = Some(screen);
    }

    pub fn activity(&mut self, activity: Activity) {
        self.out.changes.push(Change::Put(activity));
    }

    pub fn activities(&mut self, activities: impl IntoIterator<Item = Activity>) {
        let all = activities.into_iter().collect();
        self.out.changes.push(Change::All(all));
    }

    pub fn run(&mut self, id: u64, label: impl Into<String>, command: Command) {
        self.run_from(id, label, command, Stream::Stdout);
    }

    pub fn run_from(
        &mut self,
        id: u64,
        label: impl Into<String>,
        command: Command,
        progress: Stream,
    ) {
        let activity = Activity::new(id, label, self.now);
        self.out.changes.push(Change::Run(activity.clone()));
        self.out.runs.push(Run {
            screen: self.screen,
            activity,
            command,
            progress,
            keep: true,
        });
    }

    pub fn stop(&mut self, id: u64) {
        self.out.stops.push(id);
    }

    pub fn forget(&mut self, id: u64) {
        for run in self.out.runs.iter_mut().filter(|run| run.activity.id == id) {
            run.keep = false;
        }
        self.out.changes.push(Change::Forget(id));
    }

    pub fn band(&mut self, shown: bool) {
        self.out.changes.push(Change::Band(shown));
    }

    pub fn quit(&mut self) {
        self.out.quit = true;
    }

    pub fn reload(&mut self) {
        self.out.reload.push(self.screen);
    }

    pub fn changed(&mut self, source: impl Into<String>) {
        self.out.changed.push(source.into());
    }

    pub fn palette(&mut self, palette: Palette) {
        self.out.palette = Some(palette);
    }

    pub fn words(&mut self, words: Words) {
        self.out.words = Some(words);
    }

    pub fn hints(&mut self, hints: Vec<Hint<'static>>) {
        self.out.hints = Some(hints);
    }

    pub fn title(&mut self, text: impl Into<Cow<'static, str>>) {
        self.out.title = Some(vec![(text.into(), Style::new())]);
    }

    pub fn title_spans<T: Into<Cow<'static, str>>>(
        &mut self,
        spans: impl IntoIterator<Item = (T, Style)>,
    ) {
        let spans = spans
            .into_iter()
            .map(|(text, style)| (text.into(), style))
            .collect();
        self.out.title = Some(spans);
    }

    pub fn layout(&mut self, layout: Layout) {
        self.out.layout = Some(layout);
    }

    pub fn role(&mut self, role: Option<impl Into<Cow<'static, str>>>) {
        self.out.role = Some(role.map(Into::into));
    }

    pub fn show(&mut self, shown: impl Fn(Item) -> bool + 'static) {
        self.out.show = Some(Box::new(shown));
    }

    pub fn go_group(&mut self, group: usize) {
        self.out.group = Some(group);
    }

    pub fn toast(&mut self, toast: Toast) {
        self.out.toasts.push(toast);
    }

    pub fn open(&mut self, modal: Modal) {
        self.out.modals.retain(|opened| opened.modal.id != modal.id);
        self.out.modals.push(Opened {
            screen: self.screen,
            modal,
        });
    }

    pub fn alert(
        &mut self,
        id: u64,
        title: impl Into<Cow<'static, str>>,
        lines: impl IntoIterator<Item = Line<'static>>,
    ) {
        self.open(Modal::alert(id, title, lines));
    }

    pub fn modal(&mut self, id: u64) -> Option<&mut Modal> {
        self.out
            .modals
            .iter_mut()
            .find(|opened| opened.modal.id == id)
            .map(|opened| &mut opened.modal)
    }

    pub fn close(&mut self, id: u64) {
        self.out.modals.retain(|opened| opened.modal.id != id);
    }

    pub fn takeover(&mut self, takeover: Takeover) {
        self.out.takeover = Some((self.screen, takeover));
    }

    pub fn end_takeover(&mut self) {
        self.out.takeover = None;
    }

    pub fn gate(&mut self, shut: bool) {
        self.out.gate = Some(shut);
    }
}
