use std::any::Any;
use std::borrow::Cow;
use std::io;
use std::sync::mpsc::{self, Receiver, RecvTimeoutError, Sender};
use std::thread;
use std::time::{Duration, Instant};

use crossterm::event::{Event, KeyEvent, KeyEventKind, MouseButton, MouseEvent, MouseEventKind};
use pito_footer::{Answer, Footer, Guard, Help, Hint, Key, Mode, QuitGuard, WINDOW, Wording};
use pito_header::{Action, Group, Header, Nav, NavKeys, Section, Step};
use ratatui::layout::Rect;
use ratatui::style::Color;

use crate::copy;
use crate::pace::{Pace, wait_until};
use crate::palette::Palette;
use crate::screen::{Cx, Job, Outbox, Phase, Screen};
use crate::term::{Modes, Term};
use crate::wake::{Wake, Waker, listen};
use crate::words::Words;

const SETTLE: usize = 64;
const QUIT: &[Key] = &[Key::Ctrl('c')];
const STOP: &[Key] = &[Key::Esc];

pub type HeaderLook = for<'a> fn(Header<'a>) -> Header<'a>;
pub type FooterLook = for<'a> fn(Footer<'a>) -> Footer<'a>;

type KeyHook<E> = Box<dyn FnMut(KeyEvent, &mut Cx<'_, E>) -> bool>;
type DrawHook = Box<dyn FnMut(Duration)>;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Flow {
    Stay,
    Quit,
}

pub(crate) struct Slot<E> {
    pub(crate) screen: Box<dyn Screen<E>>,
    pub(crate) started: bool,
    pub(crate) stale: bool,
    pub(crate) generation: u64,
    pub(crate) reading: Option<u64>,
    pub(crate) since: Option<Instant>,
}

impl<E> Slot<E> {
    fn new(screen: Box<dyn Screen<E>>) -> Self {
        Slot {
            screen,
            started: false,
            stale: true,
            generation: 0,
            reading: None,
            since: None,
        }
    }
}

pub struct Tui<E> {
    pub(crate) name: Cow<'static, str>,
    pub(crate) version: Cow<'static, str>,
    pub(crate) palette: Palette,
    pub(crate) words: Words,
    pub(crate) hints: Vec<Hint<'static>>,
    pub(crate) min: (u16, u16),
    groups: Vec<Group>,
    pub(crate) nav: Nav,
    nav_keys: NavKeys,
    pub(crate) screens: Vec<Slot<E>>,
    pub(crate) help: Option<Help>,
    pub(crate) quit: QuitGuard,
    mode: Mode,
    window: Duration,
    triggers: &'static [Key],
    stop: &'static [Key],
    eager: bool,
    pub(crate) out: Outbox<E>,
    pub(crate) inbox: Receiver<Wake<E>>,
    pub(crate) moment: Instant,
    modes: Modes,
    pub(crate) header_look: Option<HeaderLook>,
    pub(crate) footer_look: Option<FooterLook>,
    on_key: Option<KeyHook<E>>,
    after: Option<DrawHook>,
    pub(crate) content: Rect,
    pub(crate) top: Rect,
}

impl<E: Send + 'static> Tui<E> {
    pub fn new(
        name: impl Into<Cow<'static, str>>,
        version: impl Into<Cow<'static, str>>,
        accent: Color,
    ) -> Self {
        let (sender, inbox) = mpsc::channel();
        let words = Words::new();
        let mut tui = Tui {
            name: name.into(),
            version: version.into(),
            palette: Palette::new(accent),
            quit: QuitGuard::new(Wording::new(words.again.clone())),
            words,
            hints: Vec::new(),
            min: (40, 12),
            groups: Vec::new(),
            nav: Nav::new(Vec::new()),
            nav_keys: NavKeys::HEY,
            screens: Vec::new(),
            help: Some(Help::new(true)),
            mode: Mode::Ask,
            window: WINDOW,
            triggers: QUIT,
            stop: STOP,
            eager: false,
            out: Outbox::new(sender),
            inbox,
            moment: Instant::now(),
            modes: Modes::new(),
            header_look: None,
            footer_look: None,
            on_key: None,
            after: None,
            content: Rect::default(),
            top: Rect::default(),
        };
        tui.rearm();
        tui
    }

    pub fn palette(mut self, palette: Palette) -> Self {
        self.palette = palette;
        self
    }

    pub fn words(mut self, words: Words) -> Self {
        self.set_words(words);
        self
    }

    pub fn hints(mut self, hints: impl IntoIterator<Item = Hint<'static>>) -> Self {
        self.hints = hints.into_iter().collect();
        self
    }

    pub fn min_size(mut self, width: u16, height: u16) -> Self {
        self.min = (width, height);
        self
    }

    pub fn group(mut self, group: Group) -> Self {
        self.groups.push(group);
        self.renav();
        self
    }

    pub fn screen(mut self, section: Section, screen: impl Screen<E>) -> Self {
        let group = self.groups.pop().unwrap_or_else(|| Group::new(""));
        self.groups.push(group.section(section));
        self.screens.push(Slot::new(Box::new(screen)));
        self.renav();
        self
    }

    pub fn nav_keys(mut self, keys: NavKeys) -> Self {
        self.nav_keys = keys;
        self.renav();
        self
    }

    pub fn help(mut self, help: Option<Help>) -> Self {
        self.help = help;
        self
    }

    pub fn quit(mut self, mode: Mode) -> Self {
        self.mode = mode;
        self.rearm();
        self
    }

    pub fn quit_window(mut self, window: Duration) -> Self {
        self.window = window;
        self.rearm();
        self
    }

    pub fn quit_keys(mut self, keys: &'static [Key]) -> Self {
        self.triggers = keys;
        self.rearm();
        self
    }

    pub fn stop_keys(mut self, keys: &'static [Key]) -> Self {
        self.stop = keys;
        self
    }

    pub fn eager(mut self, eager: bool) -> Self {
        self.eager = eager;
        self
    }

    pub fn modes(mut self, modes: Modes) -> Self {
        self.modes = modes;
        self
    }

    pub fn header_look(mut self, look: HeaderLook) -> Self {
        self.header_look = Some(look);
        self
    }

    pub fn footer_look(mut self, look: FooterLook) -> Self {
        self.footer_look = Some(look);
        self
    }

    pub fn on_key(mut self, hook: impl FnMut(KeyEvent, &mut Cx<'_, E>) -> bool + 'static) -> Self {
        self.on_key = Some(Box::new(hook));
        self
    }

    pub fn after_draw(mut self, hook: impl FnMut(Duration) + 'static) -> Self {
        self.after = Some(Box::new(hook));
        self
    }

    fn renav(&mut self) {
        self.nav = Nav::new(self.groups.clone()).keys(self.nav_keys);
    }

    fn rearm(&mut self) {
        self.quit = QuitGuard::new(Wording::new(self.words.again.clone()))
            .mode(self.mode)
            .window(self.window)
            .triggers(self.triggers);
    }

    pub fn set_palette(&mut self, palette: Palette) {
        self.palette = palette;
    }

    pub fn set_words(&mut self, words: Words) {
        *self.quit.wording_mut() = Wording::new(words.again.clone());
        self.words = words;
    }

    pub fn current_palette(&self) -> &Palette {
        &self.palette
    }

    pub fn current(&self) -> Option<usize> {
        self.nav.place().number.checked_sub(1)
    }

    pub fn names(&self) -> Vec<&str> {
        self.nav
            .groups()
            .iter()
            .flat_map(|group| group.sections().iter().map(Section::name))
            .collect()
    }

    pub fn find<T: Screen<E>>(&self) -> Option<&T> {
        self.screens.iter().find_map(|slot| {
            let any: &dyn Any = slot.screen.as_ref();
            any.downcast_ref::<T>()
        })
    }

    pub fn find_mut<T: Screen<E>>(&mut self) -> Option<&mut T> {
        self.screens.iter_mut().find_map(|slot| {
            let any: &mut dyn Any = slot.screen.as_mut();
            any.downcast_mut::<T>()
        })
    }

    pub fn sender(&self) -> Sender<Wake<E>> {
        self.out.sender.clone()
    }

    pub fn waker(&self, screen: usize) -> Waker<E> {
        Waker::new(self.out.sender.clone(), screen)
    }

    pub fn go(&mut self, screen: usize) -> bool {
        if self.nav.go(screen + 1).is_none() {
            return false;
        }
        self.enter();
        true
    }

    pub fn changed(&mut self, source: &str) {
        for slot in &mut self.screens {
            if slot.screen.sources().contains(&source) {
                slot.stale = true;
            }
        }
    }

    pub fn start(&mut self) {
        self.enter();
        self.follow();
    }

    pub fn busy(&self) -> usize {
        self.screens.iter().map(|slot| slot.screen.busy()).sum()
    }

    pub fn key(&mut self, key: KeyEvent) -> Flow {
        if key.kind == KeyEventKind::Release {
            return Flow::Stay;
        }
        let busy = self.busy();
        match self.quit.key(key.into(), self.moment, busy > 0) {
            Guard::Quit => return Flow::Quit,
            Guard::Held => {
                self.word_quit();
                return Flow::Stay;
            }
            _ => {}
        }
        self.out.said = None;
        self.follow();
        if let Some(asking) = self.out.asking.as_mut() {
            if let Some(answer) = asking.confirm.key(key.into()) {
                let screen = asking.screen;
                self.out.asking = None;
                let yes = matches!(answer, Answer::Yes);
                self.call(screen, |screen, cx| screen.answer(yes, cx));
            }
            return self.apply();
        }
        let Some(index) = self.current() else {
            return Flow::Stay;
        };
        if self.screens[index].screen.typing() {
            self.call(index, |screen, cx| screen.key(key, cx));
            return self.apply();
        }
        if self.help.as_mut().is_some_and(|help| help.key(key.into())) {
            return Flow::Stay;
        }
        if let Some(action) = self.nav.action(key.into()) {
            self.navigate(action);
            return self.apply();
        }
        if self.loading(index) && self.stop.contains(&key.into()) {
            self.cancel(index);
            return Flow::Stay;
        }
        if let Some(mut hook) = self.on_key.take() {
            let mut cx = Cx {
                screen: index,
                now: self.moment,
                out: &mut self.out,
            };
            let taken = hook(key, &mut cx);
            self.on_key = Some(hook);
            if taken {
                return self.apply();
            }
        }
        self.call(index, |screen, cx| screen.key(key, cx));
        self.apply()
    }

    pub fn paste(&mut self, text: &str) {
        if self.out.asking.is_some() || self.quit.asking().is_some() {
            return;
        }
        if let Some(index) = self.current() {
            self.call(index, |screen, cx| screen.paste(text, cx));
            self.apply();
        }
    }

    pub fn mouse(&mut self, mouse: MouseEvent) {
        let Some(index) = self.current() else {
            return;
        };
        let (column, row) = (mouse.column, mouse.row);
        if mouse.kind == MouseEventKind::Down(MouseButton::Left)
            && self.top.contains((column, row).into())
        {
            let facts = self.screens[index].screen.facts();
            let place = self.header(&facts, None).hit(self.top, column, row);
            if let Some(place) = place
                && self.nav.go_to(place.group, place.section).is_some()
            {
                self.enter();
            }
            return;
        }
        let area = self.content;
        self.call(index, |screen, cx| screen.mouse(mouse, area, cx));
        self.apply();
    }

    pub fn event(&mut self, screen: usize, event: E) {
        self.call(screen, |target, cx| target.event(event, cx));
        self.apply();
    }

    pub fn loaded(&mut self, screen: usize, generation: u64, event: E) -> bool {
        let Some(slot) = self.screens.get_mut(screen) else {
            return false;
        };
        if slot.reading == Some(generation) {
            slot.reading = None;
        }
        if generation != slot.generation {
            return false;
        }
        self.call(screen, |target, cx| target.loaded(event, cx));
        self.apply();
        true
    }

    pub fn handle(&mut self, wake: Wake<E>) -> Flow {
        match wake {
            Wake::Input(Event::Key(key)) => return self.key(key),
            Wake::Input(Event::Paste(text)) => self.paste(&text),
            Wake::Input(Event::Mouse(mouse)) => self.mouse(mouse),
            Wake::Event(screen, event) => self.event(screen, event),
            Wake::Loaded(screen, generation, event) => {
                self.loaded(screen, generation, event);
            }
            _ => {}
        }
        Flow::Stay
    }

    fn navigate(&mut self, action: Action) {
        match action {
            Action::Back => {
                if let Some(Step::Back { .. }) = self.nav.apply(Action::Back)
                    && let Some(index) = self.current()
                {
                    self.call(index, |screen, cx| screen.back(cx));
                }
            }
            action => {
                if let Some(Step::Moved(_)) = self.nav.apply(action) {
                    self.enter();
                }
            }
        }
    }

    fn enter(&mut self) {
        let Some(index) = self.current() else {
            return;
        };
        if self.screens[index].started {
            return;
        }
        self.screens[index].started = true;
        self.call(index, |screen, cx| screen.start(cx));
        self.apply();
    }

    pub(crate) fn follow(&mut self) {
        let Some(index) = self.current() else {
            return;
        };
        let screen = &self.screens[index].screen;
        let crumb = screen.crumb();
        if self.nav.title() == crumb.as_deref() {
            return;
        }
        let selected = screen.selected();
        self.nav.close();
        if let Some(crumb) = crumb {
            self.nav.open(crumb, selected);
        }
    }

    pub(crate) fn word_quit(&mut self) {
        if self.quit.asking().is_none() {
            return;
        }
        let ask = (self.words.busy)(self.busy());
        self.quit.wording_mut().ask = ask.into();
    }

    pub(crate) fn loading(&self, index: usize) -> bool {
        self.screens
            .get(index)
            .is_some_and(|slot| matches!(slot.screen.phase(), Phase::Loading(_)))
    }

    fn cancel(&mut self, index: usize) {
        let slot = &mut self.screens[index];
        slot.generation += 1;
        slot.reading = None;
        slot.screen.cancel();
    }

    fn call<R>(
        &mut self,
        index: usize,
        work: impl FnOnce(&mut dyn Screen<E>, &mut Cx<'_, E>) -> R,
    ) -> Option<R> {
        let slot = self.screens.get_mut(index)?;
        let mut cx = Cx {
            screen: index,
            now: self.moment,
            out: &mut self.out,
        };
        Some(work(slot.screen.as_mut(), &mut cx))
    }

    fn apply(&mut self) -> Flow {
        if let Some(palette) = self.out.palette.take() {
            self.palette = palette;
        }
        if let Some(words) = self.out.words.take() {
            self.set_words(words);
        }
        if let Some(hints) = self.out.hints.take() {
            self.hints = hints;
        }
        for index in std::mem::take(&mut self.out.reload) {
            if let Some(slot) = self.screens.get_mut(index) {
                slot.stale = true;
            }
        }
        for source in std::mem::take(&mut self.out.changed) {
            self.changed(&source);
        }
        if let Some(screen) = self.out.go.take() {
            self.go(screen);
        }
        if std::mem::take(&mut self.out.quit) {
            return Flow::Quit;
        }
        Flow::Stay
    }

    pub(crate) fn reads(&mut self) -> Vec<(usize, u64, Job<E>)> {
        let mut reads = Vec::new();
        for (index, slot) in self.screens.iter_mut().enumerate() {
            if !slot.stale || slot.reading.is_some() || !(slot.started || self.eager) {
                continue;
            }
            slot.stale = false;
            if let Some(job) = slot.screen.load() {
                slot.generation += 1;
                slot.reading = Some(slot.generation);
                reads.push((index, slot.generation, job));
            }
        }
        reads
    }

    pub fn pump(&mut self) {
        for (screen, generation, job) in self.reads() {
            let sender = self.out.sender.clone();
            thread::spawn(move || {
                let _ = sender.send(Wake::Loaded(screen, generation, job()));
            });
        }
        for (screen, job) in self.out.jobs.drain(..) {
            let sender = self.out.sender.clone();
            thread::spawn(move || {
                let _ = sender.send(Wake::Event(screen, job()));
            });
        }
    }

    pub fn settle(&mut self) {
        for _ in 0..SETTLE {
            let mut woke = false;
            for (screen, generation, job) in self.reads() {
                self.loaded(screen, generation, job());
                woke = true;
            }
            for (screen, job) in std::mem::take(&mut self.out.jobs) {
                self.event(screen, job());
                woke = true;
            }
            while let Ok(wake) = self.inbox.try_recv() {
                self.handle(wake);
                woke = true;
            }
            if !woke {
                return;
            }
        }
    }

    pub(crate) fn tick(&mut self) -> Instant {
        self.moment = Instant::now();
        self.quit.tick(self.moment);
        self.moment
    }

    pub(crate) fn animating(&self) -> bool {
        self.current()
            .and_then(|index| self.screens.get(index))
            .is_some_and(|slot| {
                slot.screen.animating() || matches!(slot.screen.phase(), Phase::Loading(_))
            })
    }

    pub(crate) fn deadlines(&self) -> [Option<Instant>; 2] {
        let screen = self
            .current()
            .and_then(|index| self.screens.get(index))
            .and_then(|slot| slot.screen.deadline());
        [self.quit.deadline(), screen]
    }

    pub fn run(mut self) -> io::Result<()> {
        let mut term = Term::enter(self.modes)?;
        listen(self.out.sender.clone());
        self.run_in(&mut term)
    }

    pub fn run_in(&mut self, term: &mut Term) -> io::Result<()> {
        self.tick();
        self.start();
        self.pump();
        let mut pace = Pace::default();
        let mut dirty = true;
        loop {
            let moment = self.tick();
            if dirty && pace.ready(moment) {
                let began = Instant::now();
                term.draw(|frame| self.draw(frame))?;
                pace.drew(moment);
                dirty = false;
                if let Some(after) = self.after.as_mut() {
                    after(began.elapsed());
                }
            }
            let moment = self.tick();
            dirty = dirty || self.animating();
            let until = wait_until(moment, dirty, &pace, &self.deadlines());
            let first = match until {
                Some(at) => match self
                    .inbox
                    .recv_timeout(at.saturating_duration_since(moment))
                {
                    Ok(wake) => Some(wake),
                    Err(RecvTimeoutError::Timeout) => None,
                    Err(RecvTimeoutError::Disconnected) => return Ok(()),
                },
                None => match self.inbox.recv() {
                    Ok(wake) => Some(wake),
                    Err(_) => return Ok(()),
                },
            };
            dirty = true;
            let mut next = first;
            while let Some(wake) = next {
                self.tick();
                match wake {
                    Wake::Lost(error) => return Err(error),
                    wake => {
                        if self.handle(wake) == Flow::Quit {
                            return Ok(());
                        }
                    }
                }
                next = self.inbox.try_recv().ok();
            }
            self.tick();
            self.pump();
            for text in std::mem::take(&mut self.out.copies) {
                copy::copy(&text);
            }
        }
    }
}
