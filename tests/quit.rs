use std::process::Command;
use std::time::{Duration, Instant};

use pito_tui::command::Heard;
use pito_tui::crossterm::event::{
    Event, KeyCode, KeyEvent, KeyModifiers, MouseEvent, MouseEventKind,
};
use pito_tui::ratatui::{Frame, layout::Rect, style::Color};
use pito_tui::{Cx, Flow, Job, Palette, Screen, Tui, Wake};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum When {
    Key,
    Tick,
    Event,
    Loaded,
    Heard,
    Paste,
    Mouse,
    Entered,
    GoThenQuit,
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
            KeyCode::Char('g') => {
                self.seen.push("go");
                cx.go(1);
                if self.when == When::GoThenQuit {
                    cx.quit();
                }
            }
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

    fn entered(&mut self, cx: &mut Cx<'_, u32>) {
        self.hit("entered", When::Entered, cx);
    }

    fn paste(&mut self, _text: &str, cx: &mut Cx<'_, u32>) {
        self.hit("paste", When::Paste, cx);
    }

    fn mouse(&mut self, _mouse: MouseEvent, _area: Rect, cx: &mut Cx<'_, u32>) {
        self.hit("mouse", When::Mouse, cx);
    }
}

fn moved() -> MouseEvent {
    MouseEvent {
        kind: MouseEventKind::Moved,
        column: 1,
        row: 10,
        modifiers: KeyModifiers::NONE,
    }
}

struct Door {
    quits: bool,
    entered: usize,
}

impl Screen<u32> for Door {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn entered(&mut self, cx: &mut Cx<'_, u32>) {
        self.entered += 1;
        if self.quits {
            cx.quit();
        }
    }
}

fn pair(first: Quitter, quits: bool) -> Tui<u32> {
    let mut tui = Tui::<u32>::new("quit", "1.0.0", Color::Blue)
        .screen("One", first)
        .screen("Two", Door { quits, entered: 0 });
    tui.start();
    tui
}

fn entered(tui: &Tui<u32>) -> Option<usize> {
    tui.find::<Door>().map(|door| door.entered)
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
    assert_eq!(
        seen(&tui).iter().filter(|what| **what == "loaded").count(),
        2,
        "the woken load of the current generation reached the screen"
    );
    assert_eq!(tui.key(press(KeyCode::Char('r'))), Flow::Stay);
    assert_eq!(tui.key(press(KeyCode::Char('s'))), Flow::Stay);
    assert_eq!(tui.settle(), Flow::Stay);
    let seen = seen(&tui);
    for what in ["key", "tick", "event", "loaded", "heard"] {
        assert!(
            seen.contains(&what),
            "{what} never reached the screen: {seen:?}"
        );
    }
    assert!(tui.current().is_some());
    assert_eq!(tui.key(press(KeyCode::Char('x'))), Flow::Stay);
}

struct Reader {
    reads: bool,
    landed: usize,
}

impl Screen<u32> for Reader {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn load(&mut self) -> Option<Job<u32>> {
        if !self.reads {
            return None;
        }
        self.reads = false;
        Some(Box::new(|| 8))
    }

    fn loaded(&mut self, _event: u32, _cx: &mut Cx<'_, u32>) {
        self.landed += 1;
    }
}

#[test]
fn a_settle_that_quits_still_lands_every_read_it_took() {
    let mut tui = Tui::<u32>::new("quit", "1.0.0", Color::Blue)
        .eager(true)
        .screen("One", Quitter::new(When::Loaded).reading())
        .screen(
            "Two",
            Reader {
                reads: true,
                landed: 0,
            },
        );
    tui.start();
    assert_eq!(tui.settle(), Flow::Quit);
    assert_eq!(seen(&tui), ["loaded"]);
    assert_eq!(tui.find::<Reader>().map(|reader| reader.landed), Some(1));
}

#[test]
fn a_screen_that_quits_on_a_paste_quits_called_or_woken() {
    let mut called = tui(Quitter::new(When::Paste));
    assert_eq!(called.paste("hello"), Flow::Quit);
    assert_eq!(seen(&called), ["paste"]);

    let mut woken = tui(Quitter::new(When::Paste));
    assert_eq!(
        woken.handle(Wake::Input(Event::Paste("hello".into()))),
        Flow::Quit
    );
    assert_eq!(seen(&woken), ["paste"]);
}

#[test]
fn a_screen_that_quits_on_the_mouse_quits_called_or_woken() {
    let mut called = tui(Quitter::new(When::Mouse));
    assert_eq!(called.mouse(moved()), Flow::Quit);
    assert_eq!(seen(&called), ["mouse"]);

    let mut woken = tui(Quitter::new(When::Mouse));
    assert_eq!(woken.handle(Wake::Input(Event::Mouse(moved()))), Flow::Quit);
    assert_eq!(seen(&woken), ["mouse"]);
}

#[test]
fn a_paste_or_a_mouse_move_that_does_not_quit_stays() {
    let mut tui = tui(Quitter::new(When::Never));
    assert_eq!(tui.paste("hello"), Flow::Stay);
    assert_eq!(tui.mouse(moved()), Flow::Stay);
    assert_eq!(seen(&tui), ["paste", "mouse"]);
}

#[test]
fn a_handler_that_goes_and_quits_in_one_call_quits() {
    let mut tui = pair(Quitter::new(When::GoThenQuit), false);
    assert_eq!(tui.key(press(KeyCode::Char('g'))), Flow::Quit);
    assert_eq!(seen(&tui), ["go"]);
    assert_eq!(entered(&tui), Some(1));
}

#[test]
fn a_handler_that_only_goes_stays() {
    let mut tui = pair(Quitter::new(When::Never), false);
    assert_eq!(tui.key(press(KeyCode::Char('g'))), Flow::Stay);
    assert_eq!(entered(&tui), Some(1));
    assert_eq!(tui.current(), Some(1));
}

#[test]
fn a_screen_that_quits_as_it_is_entered_quits_the_key_that_led_there() {
    let mut tui = pair(Quitter::new(When::Never), true);
    assert_eq!(tui.key(press(KeyCode::Char('g'))), Flow::Quit);
    assert_eq!(entered(&tui), Some(1));
}
