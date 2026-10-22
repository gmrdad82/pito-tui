use std::process::Command;
use std::time::{Duration, Instant};

use pito_tui::command::Heard;
use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::ratatui::{Frame, layout::Rect, style::Color};
use pito_tui::{Cx, Flow, Job, Palette, Screen, Tui, Wake};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum When {
    Key,
    Tick,
    Event,
    Loaded,
    Heard,
    Never,
}

struct Quitter {
    when: When,
    due: Instant,
    reads: bool,
    seen: Vec<&'static str>,
}

impl Quitter {
    fn new(when: When) -> Self {
        Quitter {
            when,
            due: Instant::now(),
            reads: false,
            seen: Vec::new(),
        }
    }

    fn reading(mut self) -> Self {
        self.reads = true;
        self
    }

    fn hit(&mut self, what: &'static str, when: When, cx: &mut Cx<'_, u32>) {
        self.seen.push(what);
        if self.when == when {
            cx.quit();
        }
    }
}

impl Screen<u32> for Quitter {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, u32>) {
        match key.code {
            KeyCode::Char('r') => cx.run(5, "sim", Command::new("true")),
            KeyCode::Char('s') => cx.stop(5),
            _ => self.hit("key", When::Key, cx),
        }
    }

    fn load(&mut self) -> Option<Job<u32>> {
        if !self.reads {
            return None;
        }
        self.reads = false;
        Some(Box::new(|| 7))
    }

    fn deadline(&self) -> Option<Instant> {
        matches!(self.when, When::Tick | When::Never).then_some(self.due)
    }

    fn tick(&mut self, cx: &mut Cx<'_, u32>) {
        self.hit("tick", When::Tick, cx);
    }

    fn event(&mut self, _event: u32, cx: &mut Cx<'_, u32>) {
        self.hit("event", When::Event, cx);
    }

    fn loaded(&mut self, _event: u32, cx: &mut Cx<'_, u32>) {
        self.hit("loaded", When::Loaded, cx);
    }

    fn heard(&mut self, _id: u64, _heard: Heard, cx: &mut Cx<'_, u32>) {
        self.hit("heard", When::Heard, cx);
    }
}

fn press(code: KeyCode) -> KeyEvent {
    KeyEvent::new(code, KeyModifiers::NONE)
}

fn tui(screen: Quitter) -> Tui<u32> {
    let mut tui = Tui::<u32>::new("quit", "1.0.0", Color::Blue).screen("One", screen);
    tui.start();
    tui
}

fn seen(tui: &Tui<u32>) -> Vec<&'static str> {
    tui.find::<Quitter>()
        .map(|screen| screen.seen.clone())
        .unwrap_or_default()
}

#[test]
fn a_screen_that_quits_on_a_key_quits() {
    let mut tui = tui(Quitter::new(When::Key));
    assert_eq!(tui.key(press(KeyCode::Char('x'))), Flow::Quit);
    assert_eq!(seen(&tui), ["key"]);
}

#[test]
fn a_screen_that_quits_on_its_tick_quits() {
    let mut tui = tui(Quitter::new(When::Tick));
    assert_eq!(tui.advance(Duration::from_millis(1)), Flow::Quit);
    assert_eq!(seen(&tui), ["tick"]);
}

#[test]
fn a_screen_that_quits_on_an_event_quits_called_or_woken() {
    let mut called = tui(Quitter::new(When::Event));
    assert_eq!(called.event(0, 1), Flow::Quit);
    assert_eq!(seen(&called), ["event"]);

    let mut woken = tui(Quitter::new(When::Event));
    assert_eq!(woken.handle(Wake::Event(0, 1)), Flow::Quit);
    assert_eq!(seen(&woken), ["event"]);

    let mut stamped = tui(Quitter::new(When::Event));
    assert_eq!(stamped.handle(Wake::Stamped(0, 0, 1)), Flow::Quit);
    assert_eq!(seen(&stamped), ["event"]);
}

#[test]
fn a_stale_stamped_event_never_reaches_the_screen_and_stays() {
    let mut tui = tui(Quitter::new(When::Event));
    assert_eq!(tui.handle(Wake::Stamped(9, 0, 1)), Flow::Stay);
    assert!(seen(&tui).is_empty());
}

#[test]
fn an_event_for_no_screen_stays() {
    let mut tui = tui(Quitter::new(When::Event));
    assert_eq!(tui.event(4, 1), Flow::Stay);
    assert_eq!(tui.handle(Wake::Event(4, 1)), Flow::Stay);
    assert!(seen(&tui).is_empty());
}

#[test]
fn a_screen_that_quits_on_its_load_quits_landed_or_woken() {
    let mut landed = tui(Quitter::new(When::Loaded));
    assert_eq!(landed.landed(0, 0, 1), Some(Flow::Quit));
    assert_eq!(seen(&landed), ["loaded"]);

    let mut woken = tui(Quitter::new(When::Loaded));
    assert_eq!(woken.handle(Wake::Loaded(0, 0, 1)), Flow::Quit);
    assert_eq!(seen(&woken), ["loaded"]);
}

#[test]
fn a_load_read_while_settling_quits_the_settle() {
    let mut tui = tui(Quitter::new(When::Loaded).reading());
    assert_eq!(tui.settle(), Flow::Quit);
    assert_eq!(seen(&tui), ["loaded"]);
}

#[test]
fn a_late_load_is_dropped_stays_and_loaded_still_answers_whether_it_landed() {
    let mut tui = tui(Quitter::new(When::Loaded));
    assert_eq!(tui.landed(0, 9, 1), None);
    assert_eq!(tui.handle(Wake::Loaded(0, 9, 1)), Flow::Stay);
    assert!(!tui.loaded(0, 9, 1));
    assert!(!tui.loaded(3, 0, 1));
    assert_eq!(tui.landed(3, 0, 1), None);
    assert!(seen(&tui).is_empty());
    assert!(tui.loaded(0, 0, 1));
    assert_eq!(seen(&tui), ["loaded"]);
}

#[test]
fn a_screen_that_quits_on_a_command_it_heard_quits_the_settle() {
    let mut tui = tui(Quitter::new(When::Heard));
    assert_eq!(tui.key(press(KeyCode::Char('r'))), Flow::Stay);
    assert_eq!(tui.key(press(KeyCode::Char('s'))), Flow::Stay);
    assert_eq!(tui.settle(), Flow::Quit);
    assert_eq!(seen(&tui), ["heard"]);
}

#[test]
fn a_screen_that_never_quits_keeps_running_through_every_path() {
    let mut tui = tui(Quitter::new(When::Never).reading());
    assert_eq!(tui.settle(), Flow::Stay);
    assert_eq!(tui.key(press(KeyCode::Char('x'))), Flow::Stay);
    assert_eq!(tui.advance(Duration::from_millis(1)), Flow::Stay);
    assert_eq!(tui.event(0, 1), Flow::Stay);
    assert_eq!(tui.handle(Wake::Event(0, 1)), Flow::Stay);
    assert_eq!(tui.handle(Wake::Loaded(0, 1, 1)), Flow::Stay);
    assert_eq!(tui.key(press(KeyCode::Char('r'))), Flow::Stay);
    assert_eq!(tui.key(press(KeyCode::Char('s'))), Flow::Stay);
    assert_eq!(tui.settle(), Flow::Stay);
    let seen = seen(&tui);
    for what in ["key", "tick", "event", "loaded", "heard"] {
        assert!(seen.contains(&what), "{what} never reached the screen: {seen:?}");
    }
    assert!(tui.current().is_some());
    assert_eq!(tui.key(press(KeyCode::Char('x'))), Flow::Stay);
}
