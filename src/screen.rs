use std::any::Any;
use std::borrow::Cow;
use std::sync::mpsc::Sender;
use std::time::Instant;

use crossterm::event::{KeyEvent, MouseEvent};
use pito_footer::{Confirm, Hint, InputBar, Notice, Tone};
use ratatui::{Frame, layout::Rect, style::Style, text::Line};

use crate::activity::{Activity, Change};
use crate::copy::COPY_MAX;
use crate::palette::Palette;
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

    fn key(&mut self, _key: KeyEvent, _cx: &mut Cx<'_, E>) {}

    fn paste(&mut self, _text: &str, _cx: &mut Cx<'_, E>) {}

    fn mouse(&mut self, _mouse: MouseEvent, _area: Rect, _cx: &mut Cx<'_, E>) {}

    fn event(&mut self, _event: E, _cx: &mut Cx<'_, E>) {}

    fn answer(&mut self, _yes: bool, _cx: &mut Cx<'_, E>) {}

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

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct Asking {
    pub(crate) screen: usize,
    pub(crate) question: Cow<'static, str>,
    pub(crate) confirm: Confirm,
}

pub(crate) struct Outbox<E> {
    pub(crate) sender: Sender<Wake<E>>,
    pub(crate) jobs: Vec<(usize, Job<E>)>,
    pub(crate) copies: Vec<String>,
    pub(crate) said: Option<(Cow<'static, str>, Tone)>,
    pub(crate) asking: Option<Asking>,
    pub(crate) go: Option<usize>,
    pub(crate) changes: Vec<Change>,
    pub(crate) quit: bool,
    pub(crate) reload: Vec<usize>,
    pub(crate) changed: Vec<String>,
    pub(crate) palette: Option<Palette>,
    pub(crate) words: Option<Words>,
    pub(crate) hints: Option<Vec<Hint<'static>>>,
}

impl<E> Outbox<E> {
    pub(crate) fn new(sender: Sender<Wake<E>>) -> Self {
        Outbox {
            sender,
            jobs: Vec::new(),
            copies: Vec::new(),
            said: None,
            asking: None,
            go: None,
            changes: Vec::new(),
            quit: false,
            reload: Vec::new(),
            changed: Vec::new(),
            palette: None,
            words: None,
            hints: None,
        }
    }
}

pub struct Cx<'a, E> {
    pub(crate) screen: usize,
    pub(crate) now: Instant,
    pub(crate) out: &'a mut Outbox<E>,
}

impl<E: Send + 'static> Cx<'_, E> {
    pub fn screen(&self) -> usize {
        self.screen
    }

    pub fn now(&self) -> Instant {
        self.now
    }

    pub fn detach(&mut self, work: impl FnOnce() -> E + Send + 'static) {
        self.out.jobs.push((self.screen, Box::new(work)));
    }

    pub fn waker(&self) -> Waker<E> {
        Waker::new(self.out.sender.clone(), self.screen)
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

    pub fn forget(&mut self, id: u64) {
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
}
