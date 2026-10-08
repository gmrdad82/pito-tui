use std::any::Any;
use std::borrow::Cow;
use std::io;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::mpsc::{self, Receiver, RecvTimeoutError, Sender};
use std::thread;
use std::time::{Duration, Instant};

use crossterm::event::{Event, KeyEvent, KeyEventKind, MouseButton, MouseEvent, MouseEventKind};
use pito_footer::{
    Answer, Footer, Guard, Help, Hint, Key, Mode, QuitGuard, Tone as Said, WINDOW, Wording,
};
use pito_header::{Action, Group, Header, Nav, NavKeys, Section, Step as Move};
use pito_list::Step;
use ratatui::layout::Rect;
use ratatui::style::{Color, Style};

use crate::activity::{Activities, Activity, Board, Change, State};
use crate::command::{self, Exit, Heard, Report, Run};
use crate::copy;
use crate::head::{Focus, Head, Numbers, Row};
use crate::label::{Label, Span};
use crate::layer::{Event as Layered, Outcome, Toast};
use crate::layout::Layout;
use crate::model::{Item, Model};
use crate::pace::{Pace, wait_until};
use crate::palette::Palette;
use crate::screen::{Band, Cx, Job, Outbox, Phase, Screen};
use crate::signal;
use crate::term::{Modes, Term, restore};
use crate::wake::{Wake, Waker, listen};
use crate::words::Words;

const SETTLE: usize = 64;
const QUIT: &[Key] = &[Key::Ctrl('c')];
const STOP: &[Key] = &[Key::Esc];

pub type HeaderLook = for<'a> fn(Header<'a>) -> Header<'a>;
pub type FooterLook = for<'a> fn(Footer<'a>, u16) -> Footer<'a>;

type KeyHook<E> = Box<dyn FnMut(KeyEvent, &mut Cx<'_, E>) -> bool>;
type DrawHook = Box<dyn FnMut(Duration)>;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Flow {
    Stay,
    Quit,
}

pub(crate) enum Role {
    Tab,
    Overlay(Nav, &'static [Key]),
    Gate,
    App,
}

pub(crate) struct Slot<E> {
    pub(crate) screen: Box<dyn Screen<E>>,
    pub(crate) role: Role,
    pub(crate) started: bool,
    pub(crate) stale: bool,
    pub(crate) generation: u64,
    pub(crate) reading: Option<u64>,
    pub(crate) since: Option<Instant>,
    pub(crate) glass: Option<Instant>,
    pub(crate) label: String,
    pub(crate) ticked: Option<Instant>,
}

impl<E> Slot<E> {
    fn new(screen: Box<dyn Screen<E>>, role: Role) -> Self {
        Slot {
            screen,
            role,
            started: false,
            stale: true,
            generation: 0,
            reading: None,
            since: None,
            glass: None,
            label: String::new(),
            ticked: None,
        }
    }
}

enum Digit {
    Nav,
    Pass,
    Done(Flow),
}

pub struct Tui<E> {
    pub(crate) name: Cow<'static, str>,
    pub(crate) version: Cow<'static, str>,
    pub(crate) title: Vec<Span>,
    pub(crate) palette: Palette,
    pub(crate) drawn: Palette,
    mono: bool,
    roles: Vec<(Cow<'static, str>, Style)>,
    role: Option<Cow<'static, str>>,
    pub(crate) words: Words,
    pub(crate) hints: Vec<Hint<'static>>,
    pub(crate) min: (u16, u16),
    pub(crate) model: Model,
    pub(crate) head: Option<Head>,
    pub(crate) layout: Layout,
    pub(crate) focus: Focus,
    focus_keys: (&'static [Key], &'static [Key]),
    pub(crate) screens: Vec<Slot<E>>,
    pub(crate) open: Option<usize>,
    pub(crate) board: Option<usize>,
    pub(crate) band: Option<Box<dyn Band>>,
    pub(crate) top_band: Option<Box<dyn Band>>,
    pub(crate) gated: bool,
    pressed: Option<KeyEvent>,
    through: &'static [Key],
    running: Arc<AtomicUsize>,
    commands: Vec<command::Handle>,
    pub(crate) help: Option<Help>,
    pub(crate) quit: QuitGuard,
    mode: Mode,
    window: Duration,
    triggers: &'static [Key],
    stop: &'static [Key],
    sticky: bool,
    pub(crate) quit_tone: Said,
    pub(crate) quit_hints: bool,
    eager: bool,
    pub(crate) keep_facts: bool,
    pub(crate) lit_again: bool,
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
    pub(crate) rows: Vec<(Row, Rect)>,
    pub(crate) toasts: Vec<(Instant, Toast)>,
    pub(crate) taken: Option<Instant>,
    pub(crate) delay: Duration,
    pub(crate) least: Duration,
    pub(crate) version_fit: bool,
}

impl<E: Send + 'static> Tui<E> {
    pub fn new(
        name: impl Into<Cow<'static, str>>,
        version: impl Into<Cow<'static, str>>,
        accent: Color,
    ) -> Self {
        let (sender, inbox) = mpsc::channel();
        let words = Words::new();
        let name = name.into();
        let palette = Palette::new(accent);
        let mut tui = Tui {
            title: vec![(name.clone(), Style::new())],
            name,
            version: version.into(),
            palette,
            drawn: palette,
            mono: false,
            roles: Vec::new(),
            role: None,
            quit: QuitGuard::new(Wording::new(words.again.clone())),
            words,
            hints: Vec::new(),
            min: (40, 12),
            model: Model::new(),
            head: None,
            layout: Layout::new(),
            focus: Focus::Content,
            focus_keys: (&[], &[]),
            screens: Vec::new(),
            open: None,
            board: None,
            band: None,
            top_band: None,
            gated: false,
            pressed: None,
            through: &[],
            running: Arc::new(AtomicUsize::new(0)),
            commands: Vec::new(),
            help: Some(Help::new(true)),
            mode: Mode::Ask,
            window: WINDOW,
            triggers: QUIT,
            stop: STOP,
            sticky: false,
            quit_tone: Said::Accent,
            quit_hints: true,
            eager: false,
            keep_facts: false,
            lit_again: true,
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
            rows: Vec::new(),
            toasts: Vec::new(),
            taken: None,
            delay: Duration::ZERO,
            least: Duration::ZERO,
            version_fit: false,
        };
        tui.rearm();
        tui
    }

    pub fn palette(mut self, palette: Palette) -> Self {
        self.set_palette(palette);
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

    pub fn title(mut self, text: impl Into<Cow<'static, str>>) -> Self {
        self.title = vec![(text.into(), Style::new())];
        self
    }

    pub fn title_spans<T: Into<Cow<'static, str>>>(
        mut self,
        spans: impl IntoIterator<Item = (T, Style)>,
    ) -> Self {
        self.title = spans
            .into_iter()
            .map(|(text, style)| (text.into(), style))
            .collect();
        self
    }

    pub fn layout(mut self, layout: Layout) -> Self {
        self.layout = layout;
        self
    }

    pub fn head(mut self, head: Head) -> Self {
        self.head = Some(head);
        self
    }

    pub fn role(mut self, name: impl Into<Cow<'static, str>>, style: Style) -> Self {
        let name = name.into();
        self.roles.retain(|(kept, _)| *kept != name);
        self.roles.push((name, style));
        self.repaint();
        self
    }

    pub fn monochrome(mut self, mono: bool) -> Self {
        self.mono = mono;
        self.repaint();
        self
    }

    pub fn group(mut self, label: impl Into<Label>) -> Self {
        self.model.add_group(label.into());
        self
    }

    pub fn group_keys(mut self, keys: &'static [Key]) -> Self {
        self.model.group_keys(keys);
        self
    }

    pub fn screen(mut self, label: impl Into<Label>, screen: impl Screen<E>) -> Self {
        let index = self.model.tabs.len();
        self.screens
            .insert(index, Slot::new(Box::new(screen), Role::Tab));
        if let Some(board) = self.board.as_mut() {
            *board += 1;
        }
        self.model.add_tab(label.into(), index);
        self
    }

    pub fn show(mut self, shown: impl Fn(Item) -> bool) -> Self {
        self.model.show(&shown);
        self
    }

    pub fn overlay(
        mut self,
        section: Section,
        keys: &'static [Key],
        screen: impl Screen<E>,
    ) -> Self {
        let role = Role::Overlay(lone(section), keys);
        self.screens.push(Slot::new(Box::new(screen), role));
        self
    }

    pub fn app(mut self, screen: impl Screen<E>) -> Self {
        self.screens.retain(|slot| !matches!(slot.role, Role::App));
        self.reboard();
        self.screens.push(Slot::new(Box::new(screen), Role::App));
        self
    }

    pub fn gate(mut self, screen: impl Screen<E>, shut: bool) -> Self {
        self.screens.retain(|slot| !matches!(slot.role, Role::Gate));
        self.reboard();
        self.screens.push(Slot::new(Box::new(screen), Role::Gate));
        self.gated = shut;
        self
    }

    fn reboard(&mut self) {
        let board = self.screens.iter().position(|slot| {
            let any: &dyn Any = slot.screen.as_ref();
            any.is::<Board>()
        });
        if self.board.is_some() {
            self.board = board;
        }
    }

    pub fn activities(mut self, words: Activities) -> Self {
        let role = Role::Overlay(lone(words.section.clone()), words.keys);
        match self.board {
            Some(index) => {
                if let Some(board) = self.board_mut() {
                    board.words = words;
                }
                self.screens[index].role = role;
            }
            None => {
                self.board = Some(self.screens.len());
                let board = Board::new(words, self.moment);
                self.screens.push(Slot::new(Box::new(board), role));
            }
        }
        self
    }

    pub fn band(mut self, band: impl Band + 'static) -> Self {
        self.band = Some(Box::new(band));
        self
    }

    pub fn top_band(mut self, band: impl Band + 'static) -> Self {
        self.top_band = Some(Box::new(band));
        self
    }

    pub(crate) fn board(&self) -> Option<&Board> {
        let slot = self.screens.get(self.board?)?;
        let any: &dyn Any = slot.screen.as_ref();
        any.downcast_ref::<Board>()
    }

    fn board_mut(&mut self) -> Option<&mut Board> {
        let slot = self.screens.get_mut(self.board?)?;
        let any: &mut dyn Any = slot.screen.as_mut();
        any.downcast_mut::<Board>()
    }

    pub fn nav_keys(mut self, keys: NavKeys) -> Self {
        self.model.set_keys(keys);
        self
    }

    pub fn focus_keys(mut self, next: &'static [Key], previous: &'static [Key]) -> Self {
        self.focus_keys = (next, previous);
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

    pub fn quit_sticky(mut self, sticky: bool) -> Self {
        self.sticky = sticky;
        self
    }

    pub fn quit_tone(mut self, tone: Said) -> Self {
        self.quit_tone = tone;
        self
    }

    pub fn quit_hints(mut self, shown: bool) -> Self {
        self.quit_hints = shown;
        self
    }

    pub fn stop_keys(mut self, keys: &'static [Key]) -> Self {
        self.stop = keys;
        self
    }

    pub fn through_keys(mut self, keys: &'static [Key]) -> Self {
        self.through = keys;
        self
    }

    pub fn eager(mut self, eager: bool) -> Self {
        self.eager = eager;
        self
    }

    pub fn keep_facts(mut self, keep: bool) -> Self {
        self.keep_facts = keep;
        self
    }

    pub fn lit_again(mut self, lit: bool) -> Self {
        self.lit_again = lit;
        self
    }

    pub fn hourglass_timing(mut self, delay: Duration, least: Duration) -> Self {
        self.delay = delay;
        self.least = least;
        self
    }

    pub fn version_fit(mut self, fit: bool) -> Self {
        self.version_fit = fit;
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

    fn rearm(&mut self) {
        self.quit = QuitGuard::new(Wording::new(self.words.again.clone()))
            .mode(self.mode)
            .window(self.window)
            .triggers(self.triggers);
    }

    fn repaint(&mut self) {
        let mut palette = self.palette;
        let role = self
            .role
            .as_ref()
            .and_then(|name| self.roles.iter().find(|(kept, _)| kept == name));
        if let Some((_, style)) = role {
            palette.title = *style;
            palette.glass = *style;
            palette.shimmer = *style;
        }
        self.drawn = if self.mono { palette.mono() } else { palette };
    }

    pub fn set_palette(&mut self, palette: Palette) {
        self.palette = palette;
        self.repaint();
    }

    pub fn set_words(&mut self, words: Words) {
        *self.quit.wording_mut() = Wording::new(words.again.clone());
        self.words = words;
    }

    pub fn set_layout(&mut self, layout: Layout) {
        self.layout = layout;
    }

    pub fn current_palette(&self) -> &Palette {
        &self.palette
    }

    pub(crate) fn slot_of(&self, wanted: fn(&Role) -> bool) -> Option<usize> {
        self.screens.iter().position(|slot| wanted(&slot.role))
    }

    pub(crate) fn gate_slot(&self) -> Option<usize> {
        self.slot_of(|role| matches!(role, Role::Gate))
    }

    pub(crate) fn app_slot(&self) -> Option<usize> {
        self.slot_of(|role| matches!(role, Role::App))
    }

    pub fn current(&self) -> Option<usize> {
        if self.gated
            && let Some(gate) = self.gate_slot()
        {
            return Some(gate);
        }
        self.open.or_else(|| self.model.current_screen())
    }

    pub fn names(&self) -> Vec<&str> {
        self.screens
            .iter()
            .enumerate()
            .map(|(index, slot)| match &slot.role {
                Role::Tab => self
                    .model
                    .tabs
                    .iter()
                    .find(|tab| tab.screen == index)
                    .map_or("", |tab| tab.label.text()),
                Role::Overlay(nav, _) => nav.section().map_or("", Section::name),
                Role::Gate | Role::App => "",
            })
            .collect()
    }

    pub(crate) fn walkable(&self, index: usize) -> bool {
        self.screens
            .get(index)
            .is_some_and(|slot| matches!(slot.role, Role::Tab | Role::Overlay(..)))
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
        Waker::new(self.out.sender.clone(), screen).epoch(Arc::clone(&self.out.epoch))
    }

    pub fn go(&mut self, screen: usize) -> bool {
        match self.screens.get(screen).map(|slot| &slot.role) {
            Some(Role::Tab) => {
                if !self.model.go_screen(screen) {
                    return false;
                }
                self.open = None;
            }
            Some(Role::Overlay(..)) => self.open = Some(screen),
            Some(Role::Gate | Role::App) | None => return false,
        }
        self.focus = Focus::Content;
        self.enter();
        true
    }

    pub(crate) fn reveal(&mut self) {
        self.gated = false;
    }

    pub fn changed(&mut self, source: &str) {
        for slot in &mut self.screens {
            if slot.screen.sources().contains(&source) {
                slot.stale = true;
            }
        }
    }

    pub fn start(&mut self) {
        if let Some(app) = self.app_slot() {
            self.begin(app);
        }
        if let Some(index) = self.current() {
            self.begin(index);
        }
        self.follow();
    }

    pub fn busy(&self) -> usize {
        let screens: usize = self.screens.iter().map(|slot| slot.screen.busy()).sum();
        screens + self.running.load(Ordering::SeqCst)
    }

    pub fn key(&mut self, key: KeyEvent) -> Flow {
        if key.kind == KeyEventKind::Release {
            return Flow::Stay;
        }
        self.pressed = Some(key);
        let flow = self.route(key);
        self.pressed = None;
        flow
    }

    fn overlay_for(&self, key: Key) -> Option<usize> {
        self.screens
            .iter()
            .position(|slot| matches!(&slot.role, Role::Overlay(_, keys) if keys.contains(&key)))
    }

    pub(crate) fn typing_now(&self) -> bool {
        if let Some(top) = self.out.modals.last() {
            return top.modal.value(top.modal.focused()).is_some();
        }
        self.current()
            .and_then(|index| self.screens.get(index))
            .is_some_and(|slot| slot.screen.typing() || matches!(slot.role, Role::Gate))
    }

    fn route(&mut self, key: KeyEvent) -> Flow {
        let busy = self.busy();
        let pressed: Key = key.into();
        let sticky = self.sticky
            && self.quit.armed()
            && self.quit.asking().is_none()
            && !self.triggers.contains(&pressed);
        if !sticky {
            match self.quit.key(pressed, self.moment, busy > 0) {
                Guard::Quit => return Flow::Quit,
                Guard::Held => {
                    self.word_quit();
                    return Flow::Stay;
                }
                _ => {}
            }
        }
        self.out.said = None;
        self.follow();
        if let Some((owner, takeover)) = self.out.takeover.as_ref() {
            if takeover.cancel.contains(&pressed) {
                let (owner, id) = (*owner, takeover.id);
                self.out.takeover = None;
                self.call(owner, |screen, cx| screen.layer(id, Layered::Closed, cx));
                return self.apply();
            }
            return Flow::Stay;
        }
        if !self.out.modals.is_empty() {
            return self.modal_key(key);
        }
        let through = self.through.contains(&pressed);
        if let Some(asking) = self.out.asking.as_mut().filter(|_| !through) {
            if let Some(answer) = asking.confirm.key(pressed) {
                let screen = asking.screen;
                self.out.asking = None;
                let yes = matches!(answer, Answer::Yes);
                self.call(screen, |screen, cx| screen.answer(yes, cx));
            }
            return self.apply();
        }
        if let Some(picking) = self.out.picking.as_mut() {
            let choice = if picking.pick.cancel.contains(&pressed) {
                Some(None)
            } else if let Step::Open(index) = picking.pick.rows.key(key.into()) {
                Some(Some(index))
            } else {
                None
            };
            if let Some(choice) = choice {
                let screen = picking.screen;
                self.out.picking = None;
                self.call(screen, |screen, cx| screen.picked(choice, cx));
            }
            return self.apply();
        }
        if let Some(app) = self.app_slot()
            && self.call(app, |screen, cx| screen.claim(key, cx)) == Some(true)
        {
            return self.apply();
        }
        let Some(index) = self.current() else {
            return Flow::Stay;
        };
        if self.gated {
            self.call(index, |screen, cx| screen.key(key, cx));
            return self.apply();
        }
        if self.screens[index].screen.typing() {
            self.call(index, |screen, cx| screen.key(key, cx));
            return self.apply();
        }
        if self.help.as_mut().is_some_and(|help| help.key(pressed)) {
            return Flow::Stay;
        }
        if through && self.out.asking.is_some() {
            return self.press(index, key);
        }
        if let Some(flow) = self.focused(pressed) {
            return flow;
        }
        if let Some(overlay) = self.overlay_for(pressed) {
            if self.open == Some(overlay) {
                self.open = None;
            } else {
                self.go(overlay);
            }
            return self.apply();
        }
        if let Some(open) = self.open {
            if self.model.keys.back.contains(&key.into())
                && !(self.loading(open) && self.stop.contains(&pressed))
            {
                self.leave(open);
                return self.apply();
            }
        } else {
            if let Some(heading) = self.model.heading_for(pressed) {
                if self.model.go_heading(heading) {
                    self.enter();
                }
                return self.apply();
            }
            match self.digit(pressed) {
                Digit::Done(flow) => return flow,
                Digit::Pass => return self.press(index, key),
                Digit::Nav => {}
            }
            if let Some(action) = self.model.nav.action(key.into()) {
                self.navigate(action);
                return self.apply();
            }
        }
        if self.loading(index) && self.stop.contains(&pressed) {
            self.cancel(index);
            return Flow::Stay;
        }
        self.press(index, key)
    }

    fn digit(&mut self, key: Key) -> Digit {
        let Some(head) = self.head.as_ref() else {
            return Digit::Nav;
        };
        let Key::Char(digit @ '0'..='9') = key else {
            return Digit::Nav;
        };
        if !self.model.keys.digits {
            return Digit::Nav;
        }
        let number = if digit == '0' {
            10
        } else {
            digit as usize - '0' as usize
        };
        let lone = head.lone;
        let swallow = self.model.keys.swallow_digits;
        let missing = if swallow {
            Digit::Done(Flow::Stay)
        } else {
            Digit::Pass
        };
        match head.numbers {
            Numbers::Off => Digit::Pass,
            Numbers::Across => {
                let Some(tab) = self.model.visible.get(number - 1) else {
                    return Digit::Nav;
                };
                let heading = self.model.tabs[*tab].group;
                let group = self.model.groups.iter().position(|kept| *kept == heading);
                if lone && group.is_some_and(|group| self.model.lone(group)) {
                    return missing;
                }
                Digit::Nav
            }
            Numbers::InGroup => {
                let group = self.model.nav.place().group;
                let count = self.model.group_tabs().len();
                if (lone && count == 1) || number > count {
                    return missing;
                }
                if self.model.nav.go_to(group, number - 1).is_some() {
                    self.enter();
                }
                Digit::Done(self.apply())
            }
        }
    }

    pub(crate) fn ring(&self) -> Vec<Focus> {
        let mut ring = Vec::new();
        let Some(head) = self.head.as_ref() else {
            return ring;
        };
        if self.open.is_some() || self.gated {
            return ring;
        }
        let slot = self
            .current()
            .and_then(|index| self.screens.get(index))
            .is_some_and(|slot| !slot.screen.after_tabs().is_empty());
        for row in &head.rows {
            match row {
                Row::Groups if self.model.groups.len() > 1 => ring.push(Focus::Groups),
                Row::Sections if self.model.nav.count() > 0 => {
                    ring.push(Focus::Sections);
                    if slot {
                        ring.push(Focus::Slot);
                    }
                }
                _ => {}
            }
        }
        ring
    }

    fn focused(&mut self, key: Key) -> Option<Flow> {
        let ring = self.ring();
        if ring.is_empty() {
            self.focus = Focus::Content;
            return None;
        }
        let at = ring.iter().position(|focus| *focus == self.focus);
        if at.is_none() {
            self.focus = Focus::Content;
        }
        let stops = ring.len() + 1;
        let index = at.unwrap_or(ring.len());
        let step = |by: isize| {
            let next = (index as isize + by).rem_euclid(stops as isize) as usize;
            ring.get(next).copied().unwrap_or(Focus::Content)
        };
        let (next, previous) = self.focus_keys;
        if next.contains(&key) {
            self.focus = step(1);
            return Some(Flow::Stay);
        }
        if previous.contains(&key) {
            self.focus = step(-1);
            return Some(Flow::Stay);
        }
        let action = match (self.focus, key) {
            (Focus::Content, _) => return None,
            (_, Key::Esc | Key::Enter) => {
                self.focus = Focus::Content;
                return Some(Flow::Stay);
            }
            (_, Key::Up) => {
                if index > 0 {
                    self.focus = ring[index - 1];
                }
                return Some(Flow::Stay);
            }
            (_, Key::Down) => {
                self.focus = step(1);
                return Some(Flow::Stay);
            }
            (Focus::Groups, Key::Left) => Action::PrevGroup,
            (Focus::Groups, Key::Right) => Action::NextGroup,
            (Focus::Sections, Key::Left) => Action::PrevSection,
            (Focus::Sections, Key::Right) => Action::NextSection,
            _ => return None,
        };
        self.navigate(action);
        Some(self.apply())
    }

    fn modal_key(&mut self, key: KeyEvent) -> Flow {
        let Some(top) = self.out.modals.last_mut() else {
            return Flow::Stay;
        };
        let (owner, id) = (top.screen, top.modal.id);
        match top.modal.key(key) {
            Outcome::Stay => Flow::Stay,
            Outcome::Heard(event) => {
                self.call(owner, |screen, cx| screen.layer(id, event, cx));
                self.apply()
            }
            Outcome::Close => {
                self.out.modals.pop();
                self.call(owner, |screen, cx| screen.layer(id, Layered::Closed, cx));
                self.apply()
            }
        }
    }

    fn press(&mut self, index: usize, key: KeyEvent) -> Flow {
        if let Some(mut hook) = self.on_key.take() {
            let typing = self.typing_now();
            let mut cx = Cx {
                screen: index,
                now: self.moment,
                key: self.pressed,
                focus: self.focus,
                typing,
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
        if self.out.takeover.is_some()
            || self.out.asking.is_some()
            || self.out.picking.is_some()
            || self.quit.asking().is_some()
        {
            return;
        }
        if let Some(top) = self.out.modals.last_mut() {
            let (owner, id) = (top.screen, top.modal.id);
            if let Outcome::Heard(event) = top.modal.paste(text) {
                self.call(owner, |screen, cx| screen.layer(id, event, cx));
                self.apply();
            }
            return;
        }
        if let Some(index) = self.current() {
            self.call(index, |screen, cx| screen.paste(text, cx));
            self.apply();
        }
    }

    pub fn mouse(&mut self, mouse: MouseEvent) {
        if self.out.takeover.is_some() || !self.out.modals.is_empty() {
            return;
        }
        let Some(index) = self.current() else {
            return;
        };
        let (column, row) = (mouse.column, mouse.row);
        let down = mouse.kind == MouseEventKind::Down(MouseButton::Left);
        if down && !self.gated && self.head.is_some() {
            let hit = self
                .rows
                .iter()
                .find(|(_, line)| line.contains((column, row).into()))
                .copied();
            if let Some((kind, line)) = hit {
                self.click(kind, line, column);
                return;
            }
        }
        if down && !self.gated && self.head.is_none() && self.top.contains((column, row).into()) {
            let facts = self.screens[index].screen.facts();
            let title = self.title_text();
            let place = self
                .header(&title, &facts, &[], &[])
                .hit(self.top, column, row);
            if let Some(place) = place
                && self.model.nav.go_to(place.group, place.section).is_some()
            {
                self.open = None;
                self.enter();
            }
            return;
        }
        let area = self.content;
        self.call(index, |screen, cx| screen.mouse(mouse, area, cx));
        self.apply();
    }

    fn click(&mut self, kind: Row, line: Rect, column: u16) {
        let moved = match kind {
            Row::Groups => self
                .group_labels()
                .hit(line, column)
                .and_then(|group| self.model.groups.get(group).copied())
                .is_some_and(|heading| self.model.go_heading(heading)),
            Row::Sections if self.open.is_none() && self.model.nav.depth() == 0 => {
                let group = self.model.nav.place().group;
                let tail = self.tail();
                self.section_labels(&tail)
                    .hit(line, column)
                    .is_some_and(|section| self.model.nav.go_to(group, section).is_some())
            }
            _ => false,
        };
        if moved {
            self.open = None;
            self.enter();
        }
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
            Wake::Stamped(stamp, screen, event) => {
                if stamp == self.out.stamp() {
                    self.event(screen, event);
                }
            }
            Wake::Loaded(screen, generation, event) => {
                self.loaded(screen, generation, event);
            }
            Wake::Activity(activity) => {
                self.out.changes.push(Change::Put(activity));
                self.apply();
            }
            Wake::Command(screen, report) => return self.heard(screen, report),
            Wake::Signal(_) => return Flow::Quit,
            _ => {}
        }
        Flow::Stay
    }

    fn heard(&mut self, screen: usize, report: Report) -> Flow {
        let Report { activity, heard } = report;
        let id = activity.id;
        let mut end = None;
        for heard in heard {
            if let Heard::Exit(exit) = &heard {
                end = Some(exit.state());
            }
            self.call(screen, |target, cx| target.heard(id, heard, cx));
        }
        self.out.changes.push(Change::Ran(activity));
        self.out
            .changes
            .extend(end.map(|state| Change::End(id, state)));
        self.apply()
    }

    fn navigate(&mut self, action: Action) {
        match action {
            Action::Back => {
                if let Some(Move::Back { .. }) = self.model.nav.apply(Action::Back)
                    && let Some(index) = self.current()
                {
                    self.call(index, |screen, cx| screen.back(cx));
                }
            }
            action => {
                if let Some(Move::Moved(_)) = self.model.nav.apply(action) {
                    self.enter();
                }
            }
        }
    }

    fn leave(&mut self, index: usize) {
        let Some(Role::Overlay(nav, _)) = self.screens.get_mut(index).map(|slot| &mut slot.role)
        else {
            return;
        };
        if nav.back().is_some() {
            self.call(index, |screen, cx| screen.back(cx));
        } else {
            self.open = None;
        }
    }

    fn enter(&mut self) {
        if let Some(index) = self.current() {
            self.begin(index);
            self.call(index, |screen, cx| screen.entered(cx));
            self.apply();
        }
    }

    fn begin(&mut self, index: usize) {
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
        let slot = &mut self.screens[index];
        let crumb = slot.screen.crumb();
        let nav = match &mut slot.role {
            Role::Overlay(nav, _) => nav,
            Role::Tab => &mut self.model.nav,
            Role::Gate | Role::App => return,
        };
        if nav.title() == crumb.as_deref() {
            return;
        }
        let selected = slot.screen.selected();
        nav.close();
        if let Some(crumb) = crumb {
            nav.open(crumb, selected);
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
        let typing = self.typing_now();
        let slot = self.screens.get_mut(index)?;
        let mut cx = Cx {
            screen: index,
            now: self.moment,
            key: self.pressed,
            focus: self.focus,
            typing,
            out: &mut self.out,
        };
        Some(work(slot.screen.as_mut(), &mut cx))
    }

    fn apply(&mut self) -> Flow {
        if let Some(palette) = self.out.palette.take() {
            self.palette = palette;
            self.repaint();
        }
        if let Some(role) = self.out.role.take() {
            self.role = role;
            self.repaint();
        }
        if let Some(words) = self.out.words.take() {
            self.set_words(words);
        }
        if let Some(hints) = self.out.hints.take() {
            self.hints = hints;
        }
        if let Some(title) = self.out.title.take() {
            self.title = title;
        }
        if let Some(layout) = self.out.layout.take() {
            self.layout = layout;
        }
        if std::mem::take(&mut self.out.bump) {
            self.out.epoch.fetch_add(1, Ordering::SeqCst);
            self.out.jobs.clear();
            for slot in &mut self.screens {
                slot.generation += 1;
                slot.reading = None;
                slot.stale = true;
            }
        }
        for index in std::mem::take(&mut self.out.reload) {
            if let Some(slot) = self.screens.get_mut(index) {
                slot.stale = true;
            }
        }
        for source in std::mem::take(&mut self.out.changed) {
            self.changed(&source);
        }
        let moment = self.moment;
        for change in std::mem::take(&mut self.out.changes) {
            if let Change::Forget(id) = change {
                for handle in self.commands.iter().filter(|handle| handle.id == id) {
                    handle.forget();
                }
            }
            if let Some(board) = self.board_mut() {
                board.change(change, moment);
            }
        }
        for id in std::mem::take(&mut self.out.stops) {
            let stopped = self.stop(id);
            self.show_stopped(stopped);
        }
        for toast in std::mem::take(&mut self.out.toasts) {
            self.toasts.push((moment + toast.lasting, toast));
        }
        match (&self.out.takeover, self.taken) {
            (Some(_), None) => self.taken = Some(moment),
            (None, Some(_)) => self.taken = None,
            _ => {}
        }
        if let Some(shown) = self.out.show.take()
            && self.model.show(&*shown)
            && self.open.is_none()
        {
            self.focus = Focus::Content;
            self.enter();
        }
        if let Some(heading) = self.out.group.take()
            && self.model.go_heading(heading)
        {
            self.open = None;
            self.focus = Focus::Content;
            self.enter();
        }
        if let Some(shut) = self.out.gate.take()
            && shut != self.gated
            && self.gate_slot().is_some()
        {
            self.gated = shut;
            self.focus = Focus::Content;
            self.enter();
        }
        if let Some(screen) = self.out.go.take() {
            self.go(screen);
        }
        if std::mem::take(&mut self.out.quit) {
            return Flow::Quit;
        }
        Flow::Stay
    }

    fn stop(&mut self, id: u64) -> Vec<Activity> {
        let moment = self.moment;
        let mut stopped: Vec<Activity> = self
            .commands
            .iter()
            .filter(|handle| handle.id == id)
            .filter_map(|handle| handle.stop(moment))
            .collect();
        stopped.extend(self.unqueue(|run| run.activity.id == id));
        stopped
    }

    fn unqueue(&mut self, which: impl Fn(&Run) -> bool) -> Vec<Activity> {
        let moment = self.moment;
        let (queued, runs): (Vec<Run>, Vec<Run>) = std::mem::take(&mut self.out.runs)
            .into_iter()
            .partition(which);
        self.out.runs = runs;
        queued
            .into_iter()
            .map(|run| {
                let mut activity = run.activity;
                command::halt(&mut activity, moment);
                let report = Report {
                    activity: activity.clone(),
                    heard: vec![Heard::Exit(Exit::Stopped)],
                };
                let _ = self.out.sender.send(Wake::Command(run.screen, report));
                activity
            })
            .collect()
    }

    fn show_stopped(&mut self, stopped: Vec<Activity>) {
        let moment = self.moment;
        let Some(board) = self.board_mut() else {
            return;
        };
        for activity in stopped {
            let id = activity.id;
            board.change(Change::Ran(activity), moment);
            board.change(Change::End(id, State::Stopped), moment);
        }
    }

    fn end(&mut self) {
        let moment = self.tick();
        let mut stopped = command::end(&self.commands, moment);
        stopped.extend(self.unqueue(|_| true));
        self.show_stopped(stopped);
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
        for (screen, stamp, job) in self.out.jobs.drain(..) {
            let sender = self.out.sender.clone();
            thread::spawn(move || {
                let _ = sender.send(Wake::Stamped(stamp, screen, job()));
            });
        }
        self.commands.retain(|handle| !handle.over());
        let board = self.board.is_some();
        for mut run in self.out.runs.drain(..) {
            run.keep &= board;
            let sender = self.out.sender.clone();
            let handle = command::spawn(run, sender, Arc::clone(&self.running));
            self.commands.extend(handle);
        }
    }

    pub fn settle(&mut self) {
        for _ in 0..SETTLE {
            let mut woke = false;
            for (screen, generation, job) in self.reads() {
                self.loaded(screen, generation, job());
                woke = true;
            }
            for (screen, stamp, job) in std::mem::take(&mut self.out.jobs) {
                let event = job();
                if stamp == self.out.stamp() {
                    self.event(screen, event);
                }
                woke = true;
            }
            while let Ok(wake) = self.inbox.try_recv() {
                self.handle(wake);
                woke = true;
            }
            if self.ticks().is_some() {
                woke = true;
            }
            if !woke {
                return;
            }
        }
    }

    pub fn advance(&mut self, by: Duration) -> Flow {
        self.moment += by;
        self.quit.tick(self.moment);
        self.ticks().unwrap_or(Flow::Stay)
    }

    pub(crate) fn tick(&mut self) -> Instant {
        self.moment = Instant::now();
        self.quit.tick(self.moment);
        self.moment
    }

    fn due(&self, slot: &Slot<E>) -> Option<Instant> {
        if !(slot.started || self.eager) {
            return None;
        }
        slot.screen
            .deadline()
            .filter(|deadline| slot.ticked != Some(*deadline))
    }

    pub(crate) fn ticks(&mut self) -> Option<Flow> {
        let moment = self.moment;
        let before = self.toasts.len();
        self.toasts.retain(|(until, _)| *until > moment);
        let mut flow = (self.toasts.len() != before).then_some(Flow::Stay);
        for index in 0..self.screens.len() {
            let Some(deadline) = self.due(&self.screens[index]) else {
                continue;
            };
            if deadline > self.moment {
                continue;
            }
            self.screens[index].ticked = Some(deadline);
            self.call(index, |screen, cx| screen.tick(cx));
            let quit = self.apply() == Flow::Quit || flow == Some(Flow::Quit);
            flow = Some(if quit { Flow::Quit } else { Flow::Stay });
        }
        flow
    }

    pub(crate) fn animating(&self) -> bool {
        if self.out.takeover.is_some() {
            return true;
        }
        self.current()
            .and_then(|index| self.screens.get(index))
            .is_some_and(|slot| slot.screen.animating() || slot.glass.is_some())
    }

    fn glass_due(&self) -> Option<Instant> {
        let slot = self.screens.get(self.current()?)?;
        let loading = matches!(slot.screen.phase(), Phase::Loading(_));
        match (loading, slot.since, slot.glass) {
            (true, Some(since), None) => Some(since + self.delay),
            (false, _, Some(shown)) => Some(shown + self.least),
            _ => None,
        }
    }

    pub(crate) fn deadlines(&self) -> [Option<Instant>; 5] {
        let screens = self.screens.iter().filter_map(|slot| self.due(slot)).min();
        let open = self.open.is_some() && self.open == self.board;
        let board = self.board().and_then(|board| board.wake(self.moment, open));
        let bands = [&self.band, &self.top_band]
            .into_iter()
            .flatten()
            .filter_map(|band| band.deadline(self.moment));
        let bands = bands.chain(board).min();
        let toasts = self.toasts.iter().map(|(until, _)| *until).min();
        [
            self.quit.deadline(),
            screens,
            bands,
            toasts,
            self.glass_due(),
        ]
    }

    pub fn run(mut self) -> io::Result<()> {
        if std::env::var_os("NO_COLOR").is_some_and(|value| !value.is_empty()) {
            self.mono = true;
            self.repaint();
        }
        let mut term = Term::enter(self.modes)?;
        listen(self.out.sender.clone());
        self.run_in(&mut term)
    }

    pub fn run_in(&mut self, term: &mut Term) -> io::Result<()> {
        let sender = self.out.sender.clone();
        let live = signal::live(move |signal| {
            let _ = sender.send(Wake::Signal(signal));
        });
        let ran = self.serve(term);
        self.end();
        if let Some(signal) = live.end() {
            restore();
            signal::die(signal);
        }
        ran
    }

    fn serve(&mut self, term: &mut Term) -> io::Result<()> {
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
            if self.ticks() == Some(Flow::Quit) {
                return Ok(());
            }
            self.pump();
            for text in std::mem::take(&mut self.out.copies) {
                copy::copy(&text);
            }
        }
    }
}

fn lone(section: Section) -> Nav {
    Nav::new(vec![Group::new("").section(section)]).keys(NavKeys::NONE)
}
