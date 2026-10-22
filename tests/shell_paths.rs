use std::cell::RefCell;
use std::fmt::Display;
use std::io;
use std::rc::Rc;
use std::time::Duration;

use pito_tui::crossterm::event::{
    Event, KeyCode, KeyEvent, KeyModifiers, MouseButton, MouseEvent, MouseEventKind,
};
use pito_tui::footer::Key;
use pito_tui::footer::{Input, InputBar};
use pito_tui::header::Section;
use pito_tui::ratatui::{
    Frame,
    buffer::Buffer,
    layout::Rect,
    style::{Color, Style},
    widgets::Paragraph,
};
use pito_tui::{
    Activities, Activity, Cx, Flow, Head, Item, Palette, Row, Screen, Tui, Wake, Words, dump,
};

type Log = Rc<RefCell<Vec<String>>>;
type Act = fn(&mut Cx<'_, u32>);

struct Probe {
    name: &'static str,
    log: Log,
    on: Vec<(char, Act)>,
    field: Option<Input>,
}

impl Probe {
    fn new(name: &'static str, log: &Log) -> Self {
        Probe {
            name,
            log: Rc::clone(log),
            on: Vec::new(),
            field: None,
        }
    }

    fn typing(mut self) -> Self {
        self.field = Some(Input::new());
        self
    }

    fn on(mut self, key: char, act: Act) -> Self {
        self.on.push((key, act));
        self
    }

    fn note(&self, what: impl Display) {
        self.log.borrow_mut().push(format!("{} {what}", self.name));
    }
}

impl Screen<u32> for Probe {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        frame.render_widget(Paragraph::new(format!("{} body", self.name)), area);
    }

    fn start(&mut self, _cx: &mut Cx<'_, u32>) {
        self.note("start");
    }

    fn entered(&mut self, _cx: &mut Cx<'_, u32>) {
        self.note("entered");
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, u32>) {
        if let KeyCode::Char(c) = key.code {
            self.note(format!("key {c}"));
            if let Some((_, act)) = self.on.iter().find(|(wanted, _)| *wanted == c) {
                act(cx);
            }
        }
    }

    fn paste(&mut self, text: &str, _cx: &mut Cx<'_, u32>) {
        self.note(format!("paste {text}"));
    }

    fn mouse(&mut self, _mouse: MouseEvent, _area: Rect, _cx: &mut Cx<'_, u32>) {
        self.note("mouse");
    }

    fn event(&mut self, event: u32, _cx: &mut Cx<'_, u32>) {
        self.note(format!("event {event}"));
    }

    fn input(&self) -> Option<InputBar<'_>> {
        self.field
            .as_ref()
            .map(|field| InputBar::new("Find", field))
    }
}

fn log() -> Log {
    Rc::new(RefCell::new(Vec::new()))
}

fn lines(log: &Log) -> Vec<String> {
    log.borrow().clone()
}

fn ch(c: char) -> KeyEvent {
    KeyEvent::new(KeyCode::Char(c), KeyModifiers::NONE)
}

fn ctrl_c() -> KeyEvent {
    KeyEvent::new(KeyCode::Char('c'), KeyModifiers::CONTROL)
}

fn click(column: u16, row: u16) -> MouseEvent {
    MouseEvent {
        kind: MouseEventKind::Down(MouseButton::Left),
        column,
        row,
        modifiers: KeyModifiers::NONE,
    }
}

const ACCENT: Color = Color::Rgb(4, 5, 6);

fn base() -> Tui<u32> {
    Tui::new("probe", "1.0.0", Color::Blue)
}

fn painted(buffer: &Buffer, color: Color) -> usize {
    buffer
        .content()
        .iter()
        .filter(|cell| cell.fg == color || cell.bg == color)
        .count()
}

#[test]
fn an_app_with_no_screens_draws_nothing_and_still_leaves_on_ctrl_c() {
    let mut tui = Tui::<u32>::new("bare", "1.0.0", Color::Blue);
    assert_eq!(tui.start(), Flow::Stay);
    assert_eq!(tui.current(), None);
    assert!(tui.names().is_empty());
    assert_eq!(tui.key(ch('x')), Flow::Stay);
    assert_eq!(tui.paste("text"), Flow::Stay);
    assert_eq!(tui.mouse(click(3, 3)), Flow::Stay);
    assert_eq!(tui.handle(Wake::Input(Event::Resize(80, 24))), Flow::Stay);
    assert_eq!(tui.event(0, 1), Flow::Stay);
    assert!(!tui.loaded(0, 1, 1));
    assert!(!tui.go(0));
    assert_eq!(tui.settle(), Flow::Stay);
    assert_eq!(tui.advance(Duration::from_secs(5)), Flow::Stay);
    assert_eq!(tui.busy(), 0);
    let text = dump::text(&tui.frame(40, 12));
    assert!(text.trim().is_empty(), "{text}");
    assert_eq!(tui.key(ctrl_c()), Flow::Stay);
    assert_eq!(tui.key(ctrl_c()), Flow::Quit);
}

#[test]
fn a_window_under_the_minimum_shows_only_the_too_small_words() {
    let log = log();
    let mut tui = base()
        .words(Words::new().too_small("Make the window bigger"))
        .min_size(50, 12)
        .screen(Section::new("Main"), Probe::new("main", &log));
    tui.start();
    for (width, height) in [(49, 12), (50, 11)] {
        let text = dump::text(&tui.frame(width, height));
        assert!(text.contains("Make the window bigger"), "{text}");
        assert!(!text.contains("main body"), "{text}");
    }
    let text = dump::text(&tui.frame(50, 12));
    assert!(text.contains("main body"), "{text}");
    assert!(!text.contains("Make the window bigger"), "{text}");
}

#[test]
fn handle_leaves_a_lost_terminal_to_the_loop_and_a_resize_and_focus_changes_reach_no_screen() {
    let log = log();
    let mut tui = base().screen(Section::new("Main"), Probe::new("main", &log));
    tui.start();
    let before = tui.frame(60, 16);
    assert_eq!(tui.handle(Wake::Lost(io::Error::other("gone"))), Flow::Stay);
    assert_eq!(tui.handle(Wake::Input(Event::Resize(100, 30))), Flow::Stay);
    assert_eq!(tui.handle(Wake::Input(Event::FocusGained)), Flow::Stay);
    assert_eq!(tui.handle(Wake::Input(Event::FocusLost)), Flow::Stay);
    assert_eq!(lines(&log), ["main start"]);
    assert_eq!(tui.frame(60, 16), before);
}

#[test]
fn a_signal_quits_even_under_an_open_confirm_and_a_paste_reaches_the_screen() {
    let log = log();
    let mut tui = base().words(Words::new().yes("Yes").no("No")).screen(
        Section::new("Main"),
        Probe::new("main", &log).on('a', |cx| cx.confirm("Delete it?")),
    );
    tui.start();
    assert_eq!(
        tui.handle(Wake::Input(Event::Paste("hello".into()))),
        Flow::Stay
    );
    tui.key(ch('a'));
    assert!(dump::text(&tui.frame(60, 16)).contains("Delete it?"));
    assert_eq!(tui.handle(Wake::Signal(15)), Flow::Quit);
    assert_eq!(tui.handle(Wake::Signal(1)), Flow::Quit);
    assert_eq!(
        lines(&log),
        ["main start", "main paste hello", "main key a"]
    );
}

#[test]
fn a_waker_for_a_screen_past_the_last_is_heard_by_nobody() {
    let log = log();
    let mut tui = base().screen(Section::new("Main"), Probe::new("main", &log));
    tui.start();
    let lost = tui.waker(9);
    assert_eq!(lost.screen(), 9);
    assert!(lost.send(4));
    assert_eq!(tui.settle(), Flow::Stay);
    assert_eq!(lines(&log), ["main start"]);
    assert!(tui.waker(0).send(4));
    assert_eq!(tui.settle(), Flow::Stay);
    assert_eq!(lines(&log), ["main start", "main event 4"]);
}

fn drilled(log: &Log) -> Tui<u32> {
    let mut tui = base()
        .screen(Section::new("One"), Probe::new("one", log))
        .screen(Section::new("Two"), Probe::new("two", log))
        .overlay(
            Section::new("Drill"),
            &[Key::Char('D')],
            Probe::new("drill", log)
                .on('h', |cx| cx.show(|item| item != Item::Tab(0)))
                .on('k', |cx| cx.show(|item| item != Item::Tab(1))),
        );
    tui.start();
    tui
}

fn heard_by(log: &Log, name: &str) -> Vec<String> {
    lines(log)
        .into_iter()
        .filter(|line| line.starts_with(&format!("{name} ")))
        .collect()
}

#[test]
fn a_tab_brought_up_under_an_overlay_starts_and_enters_once_the_overlay_key_closes_it() {
    let log = log();
    let mut tui = drilled(&log);
    tui.key(ch('D'));
    tui.key(ch('h'));
    assert_eq!(tui.current(), Some(2));
    assert!(heard_by(&log, "two").is_empty());
    tui.key(ch('D'));
    assert_eq!(tui.current(), Some(1));
    assert_eq!(heard_by(&log, "two"), ["two start", "two entered"]);
    let text = dump::text(&tui.frame(60, 16));
    assert!(text.contains("two body"), "{text}");
    tui.key(ch('D'));
    tui.key(ch('D'));
    assert_eq!(
        heard_by(&log, "two"),
        ["two start", "two entered"],
        "an overlay opened and closed over an unmoved tab enters nothing"
    );
}

#[test]
fn a_started_tab_swapped_in_under_an_overlay_hears_entered_again_when_back_leaves_it() {
    let log = log();
    let mut tui = drilled(&log);
    assert!(tui.go(1));
    assert!(tui.go(0));
    assert_eq!(heard_by(&log, "two"), ["two start", "two entered"]);
    tui.key(ch('D'));
    tui.key(ch('h'));
    assert_eq!(tui.current(), Some(2));
    tui.key(KeyEvent::new(KeyCode::Esc, KeyModifiers::NONE));
    assert_eq!(tui.current(), Some(1));
    assert_eq!(
        heard_by(&log, "two"),
        ["two start", "two entered", "two entered"]
    );
    assert_eq!(heard_by(&log, "one"), ["one start", "one entered"]);
}

#[test]
fn a_tab_moved_away_and_back_under_an_overlay_hears_nothing_when_it_closes() {
    let log = log();
    let mut tui = drilled(&log);
    tui.key(ch('D'));
    tui.key(ch('h'));
    tui.key(ch('k'));
    tui.key(ch('D'));
    assert_eq!(tui.current(), Some(0));
    assert_eq!(heard_by(&log, "one"), ["one start"]);
    assert!(heard_by(&log, "two").is_empty());
}

#[test]
fn a_move_under_an_overlay_is_forgotten_once_a_go_closes_the_overlay() {
    let log = log();
    let mut tui = drilled(&log);
    tui.key(ch('D'));
    tui.key(ch('h'));
    assert!(tui.go(1));
    assert_eq!(heard_by(&log, "two"), ["two start", "two entered"]);
    tui.key(ch('D'));
    tui.key(ch('D'));
    assert_eq!(tui.current(), Some(1));
    assert_eq!(heard_by(&log, "two"), ["two start", "two entered"]);
}

#[test]
fn an_event_before_start_starts_no_screen_and_start_keeps_the_app_first() {
    let log = log();
    let mut tui = base()
        .screen(Section::new("Main"), Probe::new("main", &log))
        .app(Probe::new("app", &log));
    assert_eq!(tui.event(0, 4), Flow::Stay);
    assert_eq!(tui.handle(Wake::Event(0, 5)), Flow::Stay);
    assert_eq!(lines(&log), ["main event 4", "main event 5"]);
    tui.start();
    assert_eq!(lines(&log)[2..], ["app start", "main start"]);
}

#[test]
fn an_activity_wake_draws_its_row_on_the_opened_board() {
    let log = log();
    let mut tui = base()
        .screen(Section::new("Main"), Probe::new("main", &log))
        .activities(Activities::new(Section::new("Jobs")).keys(&[Key::Char('o')]));
    tui.start();
    let now = std::time::Instant::now();
    assert_eq!(
        tui.handle(Wake::Activity(Activity::new(4, "indexing the feed", now))),
        Flow::Stay
    );
    tui.key(ch('o'));
    let text = dump::text(&tui.frame(80, 20));
    assert!(text.contains("indexing the feed"), "{text}");
    assert_eq!(lines(&log), ["main start"]);
}

#[test]
fn an_open_gate_a_missing_group_and_a_screen_past_the_end_change_nothing() {
    let log = log();
    let mut tui = base()
        .screen(
            Section::new("Main"),
            Probe::new("main", &log)
                .on('g', |cx| cx.gate(false))
                .on('b', |cx| cx.go_group(7))
                .on('n', |cx| cx.go(9)),
        )
        .screen(Section::new("Other"), Probe::new("other", &log))
        .gate(Probe::new("gate", &log), false);
    tui.start();
    let before = dump::text(&tui.frame(60, 16));
    for key in ['g', 'b', 'n'] {
        assert_eq!(tui.key(ch(key)), Flow::Stay);
    }
    assert_eq!(tui.current(), Some(0));
    assert_eq!(
        lines(&log),
        ["main start", "main key g", "main key b", "main key n"]
    );
    assert_eq!(dump::text(&tui.frame(60, 16)), before);
}

#[test]
fn a_monochrome_app_keeps_every_colour_off_even_from_a_palette_a_screen_sends() {
    let log = log();
    let build = |mono: bool| {
        let mut tui = Tui::<u32>::new("probe", "1.0.0", Color::Rgb(9, 8, 7))
            .monochrome(mono)
            .screen(
                Section::new("Main"),
                Probe::new("main", &log)
                    .on('p', |cx| cx.palette(Palette::new(Color::Rgb(1, 2, 3)))),
            )
            .screen(Section::new("Other"), Probe::new("other", &log));
        tui.start();
        tui
    };
    let mut plain = build(false);
    assert!(painted(&plain.frame(60, 16), Color::Rgb(9, 8, 7)) > 0);
    plain.key(ch('p'));
    let repainted = plain.frame(60, 16);
    assert!(painted(&repainted, Color::Rgb(1, 2, 3)) > 0);
    assert_eq!(painted(&repainted, Color::Rgb(9, 8, 7)), 0);

    let mut mono = build(true);
    assert_eq!(painted(&mono.frame(60, 16), Color::Rgb(9, 8, 7)), 0);
    mono.key(ch('p'));
    assert_eq!(painted(&mono.frame(60, 16), Color::Rgb(1, 2, 3)), 0);
}

#[test]
fn a_role_named_twice_paints_in_its_last_style() {
    let log = log();
    let first = Color::Rgb(200, 1, 1);
    let last = Color::Rgb(1, 200, 1);
    let mut tui = base()
        .head(Head::new([Row::Title, Row::Sections]))
        .role("busy", Style::new().fg(first))
        .role("busy", Style::new().fg(last))
        .screen(
            Section::new("Main"),
            Probe::new("main", &log)
                .on('r', |cx| cx.role(Some("busy".into())))
                .on('c', |cx| cx.role(None)),
        );
    tui.start();
    assert_eq!(painted(&tui.frame(60, 16), last), 0);
    tui.key(ch('r'));
    let busy = tui.frame(60, 16);
    assert!(painted(&busy, last) > 0);
    assert_eq!(painted(&busy, first), 0);
    tui.key(ch('c'));
    assert_eq!(painted(&tui.frame(60, 16), last), 0);
}

#[test]
fn lit_again_relights_an_open_confirm_only_while_the_guard_is_armed() {
    let log = log();
    let build = |lit: bool| {
        let mut tui = Tui::<u32>::new("probe", "1.0.0", ACCENT)
            .words(Words::new().again("again to leave").yes("Yes").no("No"))
            .quit_window(Duration::from_secs(1))
            .lit_again(lit)
            .screen(
                Section::new("Main"),
                Probe::new("main", &log).on('a', |cx| cx.confirm("Delete it?")),
            );
        tui.start();
        tui.key(ch('a'));
        tui.key(ctrl_c());
        tui
    };
    let mut lit = build(true);
    let mut dark = build(false);
    let (shone, plain) = (lit.frame(60, 16), dark.frame(60, 16));
    assert!(painted(&shone, ACCENT) > painted(&plain, ACCENT));
    assert!(dump::text(&shone).contains("again to leave"));
    assert!(!dump::text(&plain).contains("again to leave"));
    assert!(dump::text(&plain).contains("Delete it?"));
    lit.advance(Duration::from_secs(1));
    dark.advance(Duration::from_secs(1));
    let (shone, plain) = (lit.frame(60, 16), dark.frame(60, 16));
    assert_eq!(shone, plain);
    assert!(!dump::text(&plain).contains("again to leave"));
    assert!(dump::text(&plain).contains("Delete it?"));
}

#[test]
fn lit_again_relights_an_input_bar_and_leaves_a_screen_without_one_dark() {
    let log = log();
    let build = |lit: bool, typing: bool| {
        let probe = Probe::new("main", &log);
        let probe = if typing { probe.typing() } else { probe };
        let mut tui = Tui::<u32>::new("probe", "1.0.0", ACCENT)
            .words(Words::new().again("again to leave"))
            .quit_window(Duration::from_secs(1))
            .lit_again(lit)
            .screen(Section::new("Main"), probe);
        tui.start();
        tui.key(ctrl_c());
        tui.frame(60, 16)
    };
    let (shone, plain) = (build(true, true), build(false, true));
    assert!(dump::text(&shone).contains("again to leave"));
    assert!(painted(&shone, ACCENT) > painted(&plain, ACCENT));
    assert_eq!(build(true, false), build(false, false));
}

#[test]
fn a_title_a_screen_sends_replaces_the_one_the_builder_set() {
    let log = log();
    let mut tui = base().title("Inbox").screen(
        Section::new("Main"),
        Probe::new("main", &log).on('t', |cx| cx.title("Weekly report")),
    );
    tui.start();
    let text = dump::text(&tui.frame(60, 16));
    assert!(text.contains("Inbox"), "{text}");
    tui.key(ch('t'));
    let text = dump::text(&tui.frame(60, 16));
    assert!(text.contains("Weekly report"), "{text}");
    assert!(!text.contains("Inbox"), "{text}");
}
