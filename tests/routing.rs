use std::cell::RefCell;
use std::rc::Rc;
use std::time::Duration;

use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyEventKind, KeyModifiers};
use pito_tui::footer::{Key, Mode, WINDOW};
use pito_tui::header::{NavKeys, Section};
use pito_tui::layer::Event as Layered;
use pito_tui::ratatui::{Frame, layout::Rect, style::Color, widgets::Paragraph};
use pito_tui::{
    Activities, Cx, Flow, Focus, Head, Item, Job, Modal, Numbers, Palette, Phase, Pick, Row,
    Screen, Takeover, Tui, Words, dump,
};

type Act = fn(&mut Cx<'_, u32>);

#[derive(Default)]
struct Rec<const ID: u8> {
    keys: Vec<KeyEvent>,
    focus: Vec<Focus>,
    answers: Vec<bool>,
    picked: Vec<Option<usize>>,
    layers: Vec<(u64, Layered)>,
    backs: usize,
    started: usize,
    entered: usize,
    typing: bool,
    loading: bool,
    cancelled: usize,
    crumb: Option<String>,
    on: Vec<(char, Act)>,
    events: Vec<(u32, Act)>,
}

impl<const ID: u8> Rec<ID> {
    fn on(mut self, key: char, act: Act) -> Self {
        self.on.push((key, act));
        self
    }

    fn event_does(mut self, event: u32, act: Act) -> Self {
        self.events.push((event, act));
        self
    }

    fn codes(&self) -> Vec<KeyCode> {
        self.keys.iter().map(|key| key.code).collect()
    }
}

impl<const ID: u8> Screen<u32> for Rec<ID> {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        frame.render_widget(Paragraph::new(format!("body {ID}")), area);
    }

    fn start(&mut self, _cx: &mut Cx<'_, u32>) {
        self.started += 1;
    }

    fn entered(&mut self, _cx: &mut Cx<'_, u32>) {
        self.entered += 1;
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, u32>) {
        self.keys.push(key);
        self.focus.push(cx.focus());
        if let KeyCode::Char(c) = key.code
            && let Some((_, act)) = self.on.iter().find(|(wanted, _)| *wanted == c)
        {
            act(cx);
        }
    }

    fn event(&mut self, event: u32, cx: &mut Cx<'_, u32>) {
        if let Some((_, act)) = self.events.iter().find(|(wanted, _)| *wanted == event) {
            act(cx);
        }
    }

    fn answer(&mut self, yes: bool, _cx: &mut Cx<'_, u32>) {
        self.answers.push(yes);
    }

    fn picked(&mut self, choice: Option<usize>, _cx: &mut Cx<'_, u32>) {
        self.picked.push(choice);
    }

    fn layer(&mut self, id: u64, event: Layered, _cx: &mut Cx<'_, u32>) {
        self.layers.push((id, event));
    }

    fn back(&mut self, _cx: &mut Cx<'_, u32>) {
        self.backs += 1;
        self.crumb = None;
    }

    fn crumb(&self) -> Option<String> {
        self.crumb.clone()
    }

    fn typing(&self) -> bool {
        self.typing
    }

    fn phase(&self) -> Phase<'_> {
        if self.loading {
            Phase::Loading("Reading...".into())
        } else {
            Phase::Ready
        }
    }

    fn load(&mut self) -> Option<Job<u32>> {
        None
    }

    fn cancel(&mut self) {
        self.loading = false;
        self.cancelled += 1;
    }
}

#[derive(Default)]
struct Claimer {
    claims: Vec<char>,
    claimed: Vec<KeyCode>,
}

impl Screen<u32> for Claimer {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn claim(&mut self, key: KeyEvent, _cx: &mut Cx<'_, u32>) -> bool {
        let KeyCode::Char(c) = key.code else {
            return false;
        };
        if !self.claims.contains(&c) {
            return false;
        }
        self.claimed.push(key.code);
        true
    }
}

#[derive(Default)]
struct Door {
    keys: Vec<KeyCode>,
    started: usize,
}

impl Screen<u32> for Door {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        frame.render_widget(Paragraph::new("the door"), area);
    }

    fn start(&mut self, _cx: &mut Cx<'_, u32>) {
        self.started += 1;
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, u32>) {
        self.keys.push(key.code);
        if key.code == KeyCode::Char('o') {
            cx.gate(false);
        }
    }
}

struct Blank;

impl Screen<u32> for Blank {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}
}

fn press(code: KeyCode) -> KeyEvent {
    KeyEvent::new(code, KeyModifiers::NONE)
}

fn ch(c: char) -> KeyEvent {
    press(KeyCode::Char(c))
}

fn ctrl(c: char) -> KeyEvent {
    KeyEvent::new(KeyCode::Char(c), KeyModifiers::CONTROL)
}

fn text(tui: &mut Tui<u32>) -> String {
    dump::text(&tui.frame(72, 20))
}

fn words() -> Words {
    Words::new()
        .again("again to leave")
        .yes("Yes")
        .no("No")
        .help("? keys")
        .busy(|running| format!("{running} running, leave?"))
}

fn base() -> Tui<u32> {
    Tui::new("probe", "1.2.3", Color::Blue).words(words())
}

fn two(one: Rec<1>) -> Tui<u32> {
    let mut tui = base()
        .screen(Section::new("One"), one)
        .screen(Section::new("Two"), Rec::<2>::default());
    tui.start();
    tui
}

fn one(tui: &mut Tui<u32>) -> &mut Rec<1> {
    tui.find_mut::<Rec<1>>().unwrap()
}

fn second(tui: &mut Tui<u32>) -> &mut Rec<2> {
    tui.find_mut::<Rec<2>>().unwrap()
}

#[test]
fn a_key_release_is_ignored_even_for_the_quit_guard() {
    let mut tui = two(Rec::default());
    let release = |code| KeyEvent::new_with_kind(code, KeyModifiers::NONE, KeyEventKind::Release);
    assert_eq!(tui.key(release(KeyCode::Char('2'))), Flow::Stay);
    assert_eq!(tui.current(), Some(0));
    assert!(one(&mut tui).keys.is_empty());
    let up = KeyEvent::new_with_kind(
        KeyCode::Char('c'),
        KeyModifiers::CONTROL,
        KeyEventKind::Release,
    );
    tui.key(up);
    tui.key(up);
    assert!(!text(&mut tui).contains("again to leave"));
    assert_eq!(tui.key(ctrl('c')), Flow::Stay, "a release never armed it");
    assert_eq!(tui.key(ctrl('c')), Flow::Quit);
}

#[test]
fn the_first_ctrl_c_arms_and_a_second_inside_the_window_quits() {
    let mut tui = two(Rec::default());
    assert_eq!(tui.key(ctrl('c')), Flow::Stay);
    assert!(text(&mut tui).contains("again to leave"));
    assert!(one(&mut tui).keys.is_empty());
    tui.advance(WINDOW - Duration::from_millis(1));
    assert!(text(&mut tui).contains("again to leave"));
    assert_eq!(tui.key(ctrl('c')), Flow::Quit);
}

#[test]
fn another_key_disarms_the_guard_and_reaches_the_screen() {
    let mut tui = two(Rec::default());
    tui.key(ctrl('c'));
    assert_eq!(tui.key(ch('z')), Flow::Stay);
    assert_eq!(one(&mut tui).codes(), [KeyCode::Char('z')]);
    assert!(!text(&mut tui).contains("again to leave"));
    assert_eq!(tui.key(ctrl('c')), Flow::Stay, "the guard starts over");
    assert_eq!(tui.key(ctrl('c')), Flow::Quit);
}

#[test]
fn the_window_running_out_disarms_the_guard() {
    let mut tui = two(Rec::default());
    tui.key(ctrl('c'));
    tui.advance(WINDOW);
    assert!(!text(&mut tui).contains("again to leave"));
    assert_eq!(tui.key(ctrl('c')), Flow::Stay);
    assert_eq!(tui.key(ctrl('c')), Flow::Quit);

    let mut short = base()
        .quit_window(Duration::from_millis(500))
        .screen(Section::new("One"), Rec::<1>::default());
    short.start();
    short.key(ctrl('c'));
    short.advance(Duration::from_millis(499));
    assert!(text(&mut short).contains("again to leave"));
    short.advance(Duration::from_millis(1));
    assert!(!text(&mut short).contains("again to leave"));
    assert_eq!(short.key(ctrl('c')), Flow::Stay);
}

#[test]
fn a_sticky_guard_stays_armed_through_other_keys_which_still_reach_the_screen() {
    let mut tui = base()
        .quit_sticky(true)
        .screen(Section::new("One"), Rec::<1>::default());
    tui.start();
    tui.key(ctrl('c'));
    tui.key(ch('z'));
    assert_eq!(one(&mut tui).codes(), [KeyCode::Char('z')]);
    assert!(text(&mut tui).contains("again to leave"));
    assert_eq!(tui.key(ctrl('c')), Flow::Quit);
}

#[test]
fn once_mode_quits_on_the_first_press_and_other_quit_keys_free_ctrl_c() {
    let mut once = base()
        .quit(Mode::Once)
        .screen(Section::new("One"), Rec::<1>::default());
    once.start();
    assert_eq!(once.key(ctrl('c')), Flow::Quit);

    let mut keyed = base()
        .quit_keys(&[Key::Char('Q')])
        .screen(Section::new("One"), Rec::<1>::default());
    keyed.start();
    assert_eq!(keyed.key(ctrl('c')), Flow::Stay);
    assert_eq!(keyed.key(ctrl('c')), Flow::Stay);
    assert_eq!(one(&mut keyed).keys, [ctrl('c'), ctrl('c')]);
    assert_eq!(keyed.key(ch('Q')), Flow::Stay);
    assert_eq!(keyed.key(ch('Q')), Flow::Quit);
}

#[test]
fn while_work_runs_the_second_ctrl_c_asks_and_no_or_ctrl_c_keeps_the_app() {
    struct Busy;
    impl Screen<u32> for Busy {
        fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}
        fn busy(&self) -> usize {
            3
        }
    }
    let mut tui = base()
        .screen(Section::new("One"), Rec::<1>::default())
        .screen(Section::new("Busy"), Busy);
    tui.start();
    tui.key(ctrl('c'));
    assert_eq!(tui.key(ctrl('c')), Flow::Stay);
    assert!(text(&mut tui).contains("3 running, leave?"));
    assert_eq!(tui.key(ch('n')), Flow::Stay);
    assert!(!text(&mut tui).contains("3 running, leave?"));
    assert!(
        one(&mut tui).keys.is_empty(),
        "the answer never reaches the screen"
    );

    tui.key(ctrl('c'));
    tui.key(ctrl('c'));
    assert_eq!(tui.key(ch('2')), Flow::Stay, "a stray key leaves it asking");
    assert_eq!(tui.current(), Some(0));
    assert_eq!(tui.key(ctrl('c')), Flow::Stay, "ctrl+c answers no");
    assert!(!text(&mut tui).contains("3 running, leave?"));
    tui.key(ctrl('c'));
    tui.key(ctrl('c'));
    assert_eq!(tui.key(ch('y')), Flow::Quit);
}

#[test]
fn the_quit_guard_comes_before_a_modal_a_confirm_and_a_gate() {
    let acts: [(char, Act); 3] = [
        ('m', |cx| cx.open(Modal::new(4, "Rename"))),
        ('d', |cx| cx.confirm("Delete it?")),
        ('g', |cx| cx.gate(true)),
    ];
    for (key, act) in acts {
        let mut tui = base()
            .gate(Door::default(), false)
            .screen(Section::new("One"), Rec::<1>::default().on(key, act));
        tui.start();
        tui.key(ch(key));
        assert_eq!(tui.key(ctrl('c')), Flow::Stay, "{key}");
        assert!(text(&mut tui).contains("again to leave"), "{key}");
        assert_eq!(tui.key(ctrl('c')), Flow::Quit, "{key}");
    }
}

#[test]
fn a_ctrl_c_under_a_takeover_arms_the_guard_and_still_draws_its_words() {
    let mut tui = two(Rec::default().on('t', |cx| {
        cx.takeover(Takeover::new(9, "Working hard").cancel(&[Key::Esc]));
    }));
    tui.key(ch('t'));
    assert_eq!(tui.key(ctrl('c')), Flow::Stay);
    let armed = text(&mut tui);
    assert!(armed.contains("Working hard"), "{armed}");
    assert!(armed.contains("again to leave"), "{armed}");
    assert!(one(&mut tui).layers.is_empty());
    assert_eq!(tui.key(ctrl('c')), Flow::Quit);
}

#[test]
fn a_takeover_comes_before_the_modal_under_it() {
    let mut tui = two(Rec::default().on('b', |cx| {
        cx.open(Modal::new(4, "Rename"));
        cx.takeover(Takeover::new(9, "Working hard").cancel(&[Key::Esc]));
    }));
    tui.key(ch('b'));
    for key in [
        ch('x'),
        press(KeyCode::Enter),
        press(KeyCode::Down),
        ch('q'),
    ] {
        assert_eq!(tui.key(key), Flow::Stay);
    }
    assert!(one(&mut tui).layers.is_empty());
    assert!(text(&mut tui).contains("Working hard"));
    tui.key(press(KeyCode::Esc));
    assert_eq!(one(&mut tui).layers, [(9, Layered::Closed)]);
    let under = text(&mut tui);
    assert!(
        under.contains("Rename") && !under.contains("Working hard"),
        "{under}"
    );
    tui.key(press(KeyCode::Esc));
    assert_eq!(
        one(&mut tui).layers,
        [(9, Layered::Closed), (4, Layered::Closed)]
    );
    assert_eq!(one(&mut tui).codes(), [KeyCode::Char('b')]);
}

#[test]
fn a_takeover_hears_only_its_cancel_keys() {
    let mut tui = two(Rec::default().on('t', |cx| {
        cx.takeover(Takeover::new(9, "Working hard").cancel(&[Key::Esc]));
    }));
    tui.key(ch('t'));
    assert!(text(&mut tui).contains("Working hard"));
    assert!(!text(&mut tui).contains("body 1"));
    for key in [
        ch('2'),
        ch(']'),
        ch('d'),
        ch('?'),
        press(KeyCode::Tab),
        ch('q'),
    ] {
        assert_eq!(tui.key(key), Flow::Stay);
    }
    assert_eq!(tui.current(), Some(0));
    assert_eq!(one(&mut tui).codes(), [KeyCode::Char('t')]);
    assert!(one(&mut tui).layers.is_empty());
    tui.key(press(KeyCode::Esc));
    assert_eq!(one(&mut tui).layers, [(9, Layered::Closed)]);
    assert!(text(&mut tui).contains("body 1"));
    tui.key(ch('2'));
    assert_eq!(tui.current(), Some(1));
}

#[test]
fn a_takeover_without_cancel_keys_ends_only_from_the_app() {
    let mut tui = two(Rec::default()
        .on('t', |cx| cx.takeover(Takeover::new(9, "Working hard")))
        .event_does(1, |cx| cx.end_takeover()));
    tui.key(ch('t'));
    tui.key(press(KeyCode::Esc));
    tui.key(ch('q'));
    assert!(text(&mut tui).contains("Working hard"));
    tui.event(0, 1);
    assert!(!text(&mut tui).contains("Working hard"));
    assert!(
        one(&mut tui).layers.is_empty(),
        "ending it from the app sends no Closed"
    );
    tui.key(ch('2'));
    assert_eq!(tui.current(), Some(1));
}

#[test]
fn the_modal_on_top_comes_before_an_open_confirm() {
    let mut tui = two(Rec::default().on('b', |cx| {
        cx.confirm("Delete it?");
        cx.open(Modal::new(4, "Rename"));
    }));
    tui.key(ch('b'));
    tui.key(ch('y'));
    tui.key(ch('2'));
    assert!(one(&mut tui).answers.is_empty());
    assert_eq!(tui.current(), Some(0));
    tui.key(press(KeyCode::Esc));
    assert_eq!(one(&mut tui).layers, [(4, Layered::Closed)]);
    assert!(
        one(&mut tui).answers.is_empty(),
        "esc closed the modal only"
    );
    tui.key(ch('y'));
    assert_eq!(one(&mut tui).answers, [true]);
    assert_eq!(one(&mut tui).codes(), [KeyCode::Char('b')]);
}

#[test]
fn an_open_confirm_comes_before_an_open_pick() {
    let mut tui = two(Rec::default().on('c', |cx| {
        cx.confirm("Delete it?");
        cx.pick(Pick::new("Choose", ["alpha", "beta"]));
    }));
    tui.key(ch('c'));
    tui.key(ch('j'));
    assert!(one(&mut tui).picked.is_empty());
    tui.key(ch('n'));
    assert_eq!(one(&mut tui).answers, [false]);
    assert!(one(&mut tui).picked.is_empty());
    tui.key(ch('j'));
    tui.key(press(KeyCode::Enter));
    assert_eq!(one(&mut tui).picked, [Some(1)]);
}

#[test]
fn an_open_pick_comes_before_the_app_claim_and_the_nav() {
    let mut tui = base()
        .app(Claimer {
            claims: vec!['x'],
            ..Claimer::default()
        })
        .screen(
            Section::new("One"),
            Rec::<1>::default().on('p', |cx| cx.pick(Pick::new("Choose", ["alpha", "beta"]))),
        )
        .screen(Section::new("Two"), Rec::<2>::default());
    tui.start();
    tui.key(ch('p'));
    tui.key(ch('x'));
    tui.key(ch('2'));
    assert!(tui.find::<Claimer>().unwrap().claimed.is_empty());
    assert_eq!(tui.current(), Some(0));
    tui.key(ch('q'));
    assert_eq!(one(&mut tui).picked, [None]);
    tui.key(ch('x'));
    assert_eq!(tui.find::<Claimer>().unwrap().claimed, [KeyCode::Char('x')]);
    assert_eq!(one(&mut tui).codes(), [KeyCode::Char('p')]);
}

#[test]
fn an_open_confirm_keeps_keys_from_the_claim_unless_they_are_let_through() {
    let heard = Rc::new(RefCell::new(Vec::new()));
    let seen = Rc::clone(&heard);
    let mut tui = base()
        .app(Claimer {
            claims: vec!['x'],
            ..Claimer::default()
        })
        .through_keys(&[Key::Ctrl('r'), Key::Ctrl('t')])
        .on_key(move |key, _cx| {
            seen.borrow_mut().push(key.code);
            key == ctrl('t')
        })
        .screen(
            Section::new("One"),
            Rec::<1>::default().on('d', |cx| cx.confirm("Delete it?")),
        );
    tui.start();
    tui.key(ch('d'));
    tui.key(ch('x'));
    assert!(tui.find::<Claimer>().unwrap().claimed.is_empty());
    tui.key(ctrl('r'));
    assert_eq!(one(&mut tui).keys, [ch('d'), ctrl('r')]);
    assert_eq!(*heard.borrow(), [KeyCode::Char('d'), KeyCode::Char('r')]);
    tui.key(ctrl('t'));
    assert_eq!(one(&mut tui).keys, [ch('d'), ctrl('r')], "on_key took it");
    assert_eq!(heard.borrow().len(), 3);
    assert!(
        text(&mut tui).contains("Delete it?"),
        "the confirm stays open"
    );
    tui.key(ch('y'));
    assert_eq!(one(&mut tui).answers, [true]);
    assert_eq!(heard.borrow().len(), 3, "the answer never reaches on_key");
}

#[test]
fn the_app_claim_comes_before_a_gate_a_typing_screen_help_and_the_nav() {
    let claimer = || Claimer {
        claims: vec!['x', '2', '?'],
        ..Claimer::default()
    };
    let mut typing = base()
        .app(claimer())
        .screen(
            Section::new("One"),
            Rec::<1> {
                typing: true,
                ..Rec::default()
            },
        )
        .screen(Section::new("Two"), Rec::<2>::default());
    typing.start();
    for key in ['x', '2', '?', 'a'] {
        typing.key(ch(key));
    }
    assert_eq!(
        typing.find::<Claimer>().unwrap().claimed,
        [KeyCode::Char('x'), KeyCode::Char('2'), KeyCode::Char('?')]
    );
    assert_eq!(one(&mut typing).codes(), [KeyCode::Char('a')]);
    assert_eq!(typing.current(), Some(0));

    let mut gated = base()
        .app(claimer())
        .gate(Door::default(), true)
        .screen(Section::new("One"), Rec::<1>::default());
    gated.start();
    gated.key(ch('x'));
    gated.key(ch('y'));
    assert_eq!(
        gated.find::<Claimer>().unwrap().claimed,
        [KeyCode::Char('x')]
    );
    assert_eq!(gated.find::<Door>().unwrap().keys, [KeyCode::Char('y')]);
}

#[test]
fn a_shut_gate_takes_every_other_key_and_opens_from_its_own_screen() {
    let mut tui = base()
        .gate(Door::default(), true)
        .screen(Section::new("One"), Rec::<1>::default())
        .screen(Section::new("Two"), Rec::<2>::default());
    tui.start();
    let gate = tui.current().unwrap();
    assert_eq!(tui.names()[gate], "");
    assert_eq!(tui.find::<Door>().unwrap().started, 1);
    assert_eq!(one(&mut tui).started, 0, "a shut gate hides the first tab");
    assert!(text(&mut tui).contains("the door"));
    for key in [ch('2'), ch(']'), press(KeyCode::Tab), ch('?'), ch('q')] {
        tui.key(key);
    }
    assert_eq!(tui.current(), Some(gate));
    assert_eq!(tui.find::<Door>().unwrap().keys.len(), 5);
    assert!(!tui.go(gate), "the gate is never a nav target");
    tui.key(ch('o'));
    assert_eq!(tui.current(), Some(0));
    assert_eq!(one(&mut tui).started, 1);
    assert!(text(&mut tui).contains("body 1"));
    tui.key(ch('2'));
    assert_eq!(tui.current(), Some(1));
    assert_eq!(tui.find::<Door>().unwrap().keys.len(), 6);
}

#[test]
fn a_typing_screen_hears_letters_digits_and_nav_keys_as_text_but_not_ctrl_c() {
    let mut tui = two(Rec {
        typing: true,
        ..Rec::default()
    });
    let keys = [
        ch('2'),
        ch(']'),
        ch('['),
        press(KeyCode::Tab),
        ch('?'),
        ch('q'),
        press(KeyCode::Esc),
    ];
    for key in keys {
        tui.key(key);
    }
    assert_eq!(tui.current(), Some(0));
    assert_eq!(one(&mut tui).keys, keys);
    tui.key(ctrl('c'));
    assert_eq!(one(&mut tui).keys.len(), keys.len());
    assert!(text(&mut tui).contains("again to leave"));

    one(&mut tui).typing = false;
    tui.key(ch('2'));
    assert_eq!(tui.current(), Some(1));
}

#[test]
fn without_help_the_help_key_reaches_the_screen() {
    let mut bare = base()
        .help(None)
        .screen(Section::new("One"), Rec::<1>::default());
    bare.start();
    bare.key(ch('?'));
    assert_eq!(one(&mut bare).codes(), [KeyCode::Char('?')]);
}

#[test]
fn an_overlay_key_comes_before_the_nav_keys_and_toggles_the_overlay() {
    let mut tui = base()
        .screen(Section::new("One"), Rec::<1>::default())
        .screen(Section::new("Two"), Rec::<2>::default())
        .overlay(Section::new("Ops"), &[Key::Char(']')], Rec::<3>::default());
    tui.start();
    tui.key(ch(']'));
    assert_eq!(tui.current(), Some(2));
    assert_eq!(tui.find::<Rec<3>>().unwrap().started, 1);
    tui.key(ch(']'));
    assert_eq!(tui.current(), Some(0), "the same key closes it");
    tui.key(ch('2'));
    tui.key(ch(']'));
    tui.key(ch('1'));
    assert_eq!(
        tui.current(),
        Some(2),
        "digits do not move under an overlay"
    );
    assert_eq!(tui.find::<Rec<3>>().unwrap().codes(), [KeyCode::Char('1')]);
    tui.key(ch(']'));
    assert_eq!(
        tui.current(),
        Some(1),
        "it closes back to the tab it covered"
    );
}

#[test]
fn the_header_focus_keys_come_after_help_and_before_overlays_and_the_nav() {
    let mut tui = base()
        .head(Head::new([Row::Title, Row::Sections]))
        .focus_keys(&[Key::Down, Key::Char('?')], &[Key::Up])
        .screen(Section::new("One"), Rec::<1>::default())
        .screen(Section::new("Two"), Rec::<2>::default())
        .screen(Section::new("Three"), Rec::<3>::default())
        .overlay(Section::new("Ops"), &[Key::Up], Rec::<4>::default());
    tui.start();
    tui.key(press(KeyCode::Right));
    assert_eq!(tui.current(), Some(0));
    assert_eq!(one(&mut tui).focus, [Focus::Content]);

    tui.key(ch('?'));
    assert!(text(&mut tui).contains("? keys"), "help comes first");
    tui.key(ch('?'));

    tui.key(press(KeyCode::Down));
    tui.key(press(KeyCode::Right));
    assert_eq!(tui.current(), Some(1));
    tui.key(press(KeyCode::Right));
    tui.key(press(KeyCode::Left));
    assert_eq!(tui.current(), Some(1));
    assert!(second(&mut tui).keys.is_empty());
    tui.key(ch('z'));
    assert_eq!(second(&mut tui).focus, [Focus::Sections]);
    tui.key(press(KeyCode::Esc));
    assert_eq!(
        tui.current(),
        Some(1),
        "esc gives focus back, it does not leave"
    );
    tui.key(press(KeyCode::Right));
    assert_eq!(tui.current(), Some(1));
    assert_eq!(second(&mut tui).focus, [Focus::Sections, Focus::Content]);

    tui.key(press(KeyCode::Down));
    tui.key(press(KeyCode::Enter));
    tui.key(ch('z'));
    assert_eq!(second(&mut tui).focus.last(), Some(&Focus::Content));
    assert_eq!(
        second(&mut tui).codes(),
        [KeyCode::Char('z'), KeyCode::Right, KeyCode::Char('z')]
    );

    tui.key(press(KeyCode::Down));
    tui.key(press(KeyCode::Up));
    assert_eq!(
        tui.current(),
        Some(1),
        "the focus key wins over the overlay key"
    );
    assert_eq!(tui.find::<Rec<4>>().unwrap().started, 0);
    tui.key(ch('3'));
    assert_eq!(tui.current(), Some(2));
}

#[test]
fn nav_keys_come_before_the_apps_keys_and_those_before_the_screen() {
    let mut tui = base()
        .on_key(|key, cx| {
            if key.code == KeyCode::Char('k') {
                cx.say("taken", pito_tui::footer::Tone::Good);
                return true;
            }
            false
        })
        .screen(Section::new("One"), Rec::<1>::default())
        .screen(Section::new("Two"), Rec::<2>::default());
    tui.start();
    tui.key(ch('2'));
    assert_eq!(tui.current(), Some(1));
    assert!(second(&mut tui).keys.is_empty());
    tui.key(ch('k'));
    assert!(text(&mut tui).contains("taken"));
    assert!(second(&mut tui).keys.is_empty());
    tui.key(ch('z'));
    assert_eq!(second(&mut tui).codes(), [KeyCode::Char('z')]);
    assert!(!text(&mut tui).contains("taken"), "the next key hushes it");
}

#[test]
fn the_stop_key_stops_a_load_before_the_app_and_the_screen_hear_it() {
    let mut tui = base()
        .on_key(|key, cx| {
            if key.code == KeyCode::Esc {
                cx.say("app heard esc", pito_tui::footer::Tone::Good);
            }
            false
        })
        .screen(
            Section::new("One"),
            Rec::<1> {
                loading: true,
                ..Rec::default()
            },
        );
    tui.start();
    tui.key(press(KeyCode::Esc));
    assert_eq!(one(&mut tui).cancelled, 1);
    assert!(one(&mut tui).keys.is_empty());
    assert!(!text(&mut tui).contains("app heard esc"));
    tui.key(press(KeyCode::Esc));
    assert_eq!(one(&mut tui).cancelled, 1, "nothing loads now");
    assert_eq!(one(&mut tui).codes(), [KeyCode::Esc]);
    assert!(text(&mut tui).contains("app heard esc"));

    let mut stops = base().stop_keys(&[Key::Char('s')]).screen(
        Section::new("One"),
        Rec::<1> {
            loading: true,
            ..Rec::default()
        },
    );
    stops.start();
    stops.key(press(KeyCode::Esc));
    assert_eq!(one(&mut stops).cancelled, 0);
    stops.key(ch('s'));
    assert_eq!(one(&mut stops).cancelled, 1);
}

fn grouped(keys: NavKeys, head: Option<Head>) -> Tui<u32> {
    let tui = base().nav_keys(keys);
    let tui = match head {
        Some(head) => tui.head(head),
        None => tui,
    };
    let mut tui = tui
        .group("Work")
        .group_keys(&[Key::Char('w')])
        .screen(Section::new("One"), Rec::<1>::default())
        .screen(Section::new("Two"), Rec::<2>::default())
        .group("Tools")
        .group_keys(&[Key::Char('t')])
        .screen(Section::new("Three"), Rec::<3>::default())
        .screen(Section::new("Four"), Rec::<4>::default())
        .group("Solo")
        .screen(Section::new("Five"), Rec::<5>::default());
    tui.start();
    tui
}

#[test]
fn digits_count_across_groups_and_past_the_last_tab_reach_the_screen() {
    let mut tui = grouped(NavKeys::HEY, None);
    tui.key(ch('3'));
    assert_eq!(tui.current(), Some(2));
    tui.key(ch('5'));
    assert_eq!(tui.current(), Some(4));
    tui.key(ch('1'));
    assert_eq!(tui.current(), Some(0));
    tui.key(ch('9'));
    assert_eq!(tui.current(), Some(0));
    assert_eq!(one(&mut tui).codes(), [KeyCode::Char('9')]);
    tui.key(ch('0'));
    assert_eq!(
        one(&mut tui).codes(),
        [KeyCode::Char('9'), KeyCode::Char('0')]
    );

    let mut swallowed = grouped(NavKeys::HEY.swallow_digits(true), None);
    swallowed.key(ch('9'));
    assert!(one(&mut swallowed).keys.is_empty());

    let mut off = grouped(NavKeys::HEY.digits(false), None);
    off.key(ch('3'));
    assert_eq!(off.current(), Some(0));
    assert_eq!(one(&mut off).codes(), [KeyCode::Char('3')]);
}

#[test]
fn zero_reaches_the_tenth_tab_or_the_first_with_zero_first() {
    let build = |keys: NavKeys| {
        let mut tui = base().nav_keys(keys);
        for _ in 0..11 {
            tui = tui.screen(Section::new("Tab"), Blank);
        }
        tui.start();
        tui
    };
    let mut ten = build(NavKeys::HEY);
    ten.key(ch('0'));
    assert_eq!(ten.current(), Some(9));
    ten.key(ch('1'));
    assert_eq!(ten.current(), Some(0));

    let mut zero = build(NavKeys::HEY.zero_first(true));
    zero.key(ch('3'));
    assert_eq!(zero.current(), Some(3));
    zero.key(ch('0'));
    assert_eq!(zero.current(), Some(0));
    zero.key(ch('9'));
    assert_eq!(zero.current(), Some(9));
    zero.key(ch(']'));
    assert_eq!(
        zero.current(),
        Some(10),
        "the eleventh tab is reached by the nav keys"
    );
}

#[test]
fn digits_count_inside_the_group_with_an_in_group_head() {
    let head = || Head::new([Row::Title, Row::Groups, Row::Sections]).numbers(Numbers::InGroup);
    let mut tui = grouped(NavKeys::HEY, Some(head()));
    tui.key(ch('2'));
    assert_eq!(tui.current(), Some(1));
    tui.key(press(KeyCode::Tab));
    assert_eq!(tui.current(), Some(2));
    tui.key(ch('2'));
    assert_eq!(tui.current(), Some(3));
    tui.key(ch('1'));
    assert_eq!(tui.current(), Some(2));
    tui.key(ch('3'));
    assert_eq!(tui.current(), Some(2), "past the group's tabs");
    assert_eq!(tui.find::<Rec<3>>().unwrap().codes(), [KeyCode::Char('3')]);

    let mut zero = grouped(NavKeys::HEY.zero_first(true), Some(head()));
    zero.key(ch('1'));
    assert_eq!(zero.current(), Some(1));
    zero.key(ch('0'));
    assert_eq!(zero.current(), Some(0));

    let mut lone = grouped(NavKeys::HEY, Some(head().lone(true)));
    lone.key(KeyEvent::new(KeyCode::BackTab, KeyModifiers::SHIFT));
    assert_eq!(lone.current(), Some(4));
    lone.key(ch('1'));
    assert_eq!(
        lone.find::<Rec<5>>().unwrap().codes(),
        [KeyCode::Char('1')],
        "a lone tab draws no number and takes none"
    );

    let mut off = grouped(NavKeys::HEY, Some(head().numbers(Numbers::Off)));
    off.key(ch('2'));
    assert_eq!(off.current(), Some(0));
    assert_eq!(one(&mut off).codes(), [KeyCode::Char('2')]);
}

#[test]
fn an_across_head_with_lone_leaves_a_lone_groups_digit_to_the_screen() {
    let head = Head::new([Row::Title, Row::Groups, Row::Sections]).lone(true);
    let mut tui = grouped(NavKeys::HEY, Some(head));
    tui.key(ch('4'));
    assert_eq!(tui.current(), Some(3));
    tui.key(ch('5'));
    assert_eq!(tui.current(), Some(3));
    assert_eq!(tui.find::<Rec<4>>().unwrap().codes(), [KeyCode::Char('5')]);
}

#[test]
fn group_keys_come_before_the_digits_and_the_brackets() {
    let mut tui = base()
        .group("Work")
        .screen(Section::new("One"), Rec::<1>::default())
        .screen(Section::new("Two"), Rec::<2>::default())
        .group("Tools")
        .group_keys(&[Key::Char('2'), Key::Char(']')])
        .screen(Section::new("Three"), Rec::<3>::default());
    tui.start();
    tui.key(ch('2'));
    assert_eq!(
        tui.current(),
        Some(2),
        "2 is the group's key, not the second tab"
    );
    tui.key(ch('1'));
    tui.key(ch(']'));
    assert_eq!(
        tui.current(),
        Some(2),
        "] is the group's key, not the next tab"
    );
    assert!(one(&mut tui).keys.is_empty() && second(&mut tui).started == 0);
}

#[test]
fn brackets_step_inside_a_group_and_wrap_without_leaving_it() {
    let mut tui = grouped(NavKeys::HEY, None);
    tui.key(ch(']'));
    assert_eq!(tui.current(), Some(1));
    tui.key(ch(']'));
    assert_eq!(tui.current(), Some(0));
    tui.key(ch('['));
    assert_eq!(tui.current(), Some(1));
    assert_eq!(second(&mut tui).entered, 2);
}

#[test]
fn tab_moves_between_tabs_when_there_is_one_group() {
    let mut tui = two(Rec::default());
    tui.key(press(KeyCode::Tab));
    assert_eq!(tui.current(), Some(1));
    tui.key(press(KeyCode::Tab));
    assert_eq!(tui.current(), Some(0));
    tui.key(KeyEvent::new(KeyCode::BackTab, KeyModifiers::SHIFT));
    assert_eq!(tui.current(), Some(1));
}

#[test]
fn each_group_remembers_its_last_tab_for_tab_and_its_own_keys() {
    let mut tui = grouped(NavKeys::HEY, None);
    tui.key(ch(']'));
    tui.key(press(KeyCode::Tab));
    assert_eq!(tui.current(), Some(2));
    tui.key(ch(']'));
    assert_eq!(tui.current(), Some(3));
    tui.key(press(KeyCode::Tab));
    assert_eq!(tui.current(), Some(4));
    tui.key(press(KeyCode::Tab));
    assert_eq!(tui.current(), Some(1), "work comes back on its second tab");
    tui.key(ch('t'));
    assert_eq!(tui.current(), Some(3));
    tui.key(ch('w'));
    assert_eq!(tui.current(), Some(1));
    assert!(tui.find::<Rec<4>>().unwrap().keys.is_empty());
}

#[test]
fn back_keys_close_an_overlay_unless_it_loads_or_drilled_in() {
    let mut tui = base()
        .screen(Section::new("One"), Rec::<1>::default())
        .overlay(Section::new("Ops"), &[Key::Char('O')], Rec::<3>::default());
    tui.start();
    for back in [press(KeyCode::Esc), ch('q')] {
        tui.key(ch('O'));
        assert_eq!(tui.current(), Some(1));
        tui.key(back);
        assert_eq!(tui.current(), Some(0));
    }
    assert!(tui.find::<Rec<3>>().unwrap().keys.is_empty());

    tui.key(ch('O'));
    tui.find_mut::<Rec<3>>().unwrap().loading = true;
    tui.key(press(KeyCode::Esc));
    assert_eq!(tui.current(), Some(1), "esc stops the load first");
    assert_eq!(tui.find::<Rec<3>>().unwrap().cancelled, 1);
    tui.find_mut::<Rec<3>>().unwrap().loading = true;
    tui.key(ch('q'));
    assert_eq!(tui.current(), Some(0), "q still leaves while it loads");

    tui.key(ch('O'));
    tui.find_mut::<Rec<3>>().unwrap().loading = false;
    tui.find_mut::<Rec<3>>().unwrap().crumb = Some("detail".into());
    tui.frame(72, 20);
    tui.key(press(KeyCode::Esc));
    assert_eq!(tui.current(), Some(1), "esc leaves the drill first");
    assert_eq!(tui.find::<Rec<3>>().unwrap().backs, 1);
    tui.key(press(KeyCode::Esc));
    assert_eq!(tui.current(), Some(0));
    assert_eq!(tui.find::<Rec<3>>().unwrap().cancelled, 1);
}

#[test]
fn back_keys_on_a_drilled_in_tab_call_back_and_otherwise_reach_the_screen() {
    let mut tui = two(Rec {
        crumb: Some("detail".into()),
        ..Rec::default()
    });
    tui.frame(72, 20);
    assert!(text(&mut tui).contains("detail"));
    tui.key(ch('q'));
    assert_eq!(one(&mut tui).backs, 1);
    assert!(one(&mut tui).keys.is_empty());
    one(&mut tui).crumb = Some("detail".into());
    tui.frame(72, 20);
    tui.key(press(KeyCode::Esc));
    assert_eq!(one(&mut tui).backs, 2);
    assert!(one(&mut tui).keys.is_empty());
    tui.key(press(KeyCode::Esc));
    assert_eq!(one(&mut tui).backs, 2);
    assert_eq!(one(&mut tui).codes(), [KeyCode::Esc]);
}

#[test]
fn hidden_tabs_drop_out_of_the_nav_and_the_numbers() {
    let mut tui = base()
        .screen(
            Section::new("One"),
            Rec::<1>::default()
                .on('h', |cx| cx.show(|item| item != Item::Tab(1)))
                .on('u', |cx| cx.show(|_| true)),
        )
        .screen(Section::new("Two"), Rec::<2>::default())
        .screen(Section::new("Three"), Rec::<3>::default());
    tui.start();
    tui.key(ch('h'));
    assert!(!text(&mut tui).contains("Two"));
    assert_eq!(tui.names(), ["One", "Two", "Three"]);
    tui.key(ch(']'));
    assert_eq!(tui.current(), Some(2));
    tui.key(ch('1'));
    tui.key(ch('2'));
    assert_eq!(tui.current(), Some(2), "the hidden tab gave up its number");
    assert!(!tui.go(1));
    assert_eq!(second(&mut tui).started, 0);
    tui.key(ch('1'));
    tui.key(ch('u'));
    assert!(text(&mut tui).contains("Two"));
    tui.key(ch('2'));
    assert_eq!(tui.current(), Some(1));
}

#[test]
fn hiding_the_shown_tab_moves_on_and_enters_the_next_one() {
    let mut tui = two(Rec::default().on('h', |cx| cx.show(|item| item != Item::Tab(0))));
    tui.key(ch('h'));
    assert_eq!(tui.current(), Some(1));
    assert_eq!(second(&mut tui).started, 1);
    assert_eq!(second(&mut tui).entered, 1);
}

#[test]
fn a_hidden_group_leaves_the_group_keys_and_the_builder_can_hide_one() {
    let mut tui = base()
        .group("Work")
        .screen(Section::new("One"), Rec::<1>::default())
        .group("Tools")
        .group_keys(&[Key::Char('t')])
        .screen(Section::new("Two"), Rec::<2>::default())
        .screen(Section::new("Three"), Rec::<3>::default())
        .show(|item| item != Item::Group(1));
    tui.start();
    assert!(!text(&mut tui).contains("Tools"));
    tui.key(press(KeyCode::Tab));
    assert_eq!(tui.current(), Some(0), "one group left, no tab to move to");
    tui.key(ch('t'));
    assert_eq!(tui.current(), Some(0));
    assert!(
        one(&mut tui).keys.is_empty(),
        "a hidden group's key is still swallowed"
    );
    assert!(!tui.go(2));
}

#[test]
fn go_group_from_a_screen_returns_to_that_groups_last_tab() {
    let mut tui = base()
        .group("Work")
        .screen(
            Section::new("One"),
            Rec::<1>::default().on('g', |cx| cx.go_group(1)),
        )
        .group("Tools")
        .screen(Section::new("Two"), Rec::<2>::default())
        .screen(Section::new("Three"), Rec::<3>::default())
        .screen(
            Section::new("Four"),
            Rec::<4>::default().on('h', |cx| cx.show(|item| item != Item::Tab(1))),
        );
    tui.start();
    tui.key(ch('4'));
    tui.key(ch('1'));
    tui.key(ch('g'));
    assert_eq!(tui.current(), Some(3));
    tui.key(ch('h'));
    assert_eq!(tui.current(), Some(3));
    tui.key(ch('1'));
    assert_eq!(tui.current(), Some(0));
    tui.key(ch('g'));
    assert_eq!(
        tui.current(),
        Some(3),
        "a rebuild keeps the last tab, now the second of two"
    );
}

#[test]
fn go_takes_tabs_and_overlays_but_not_the_gate_the_app_or_nothing() {
    let mut tui = base()
        .screen(Section::new("One"), Rec::<1>::default())
        .overlay(Section::new("Ops"), &[Key::Char('O')], Rec::<3>::default())
        .gate(Door::default(), false)
        .app(Claimer::default())
        .screen(Section::new("Two"), Rec::<2>::default());
    tui.start();
    assert_eq!(tui.names(), ["One", "Two", "Ops", "", ""]);
    assert!(tui.go(1));
    assert_eq!(second(&mut tui).started, 1);
    assert!(tui.go(2));
    assert_eq!(tui.current(), Some(2));
    assert!(!tui.go(3));
    assert!(!tui.go(4));
    assert!(!tui.go(5));
    assert_eq!(tui.current(), Some(2));
    assert!(tui.go(0));
    assert_eq!(tui.current(), Some(0));
    assert_eq!(one(&mut tui).entered, 1);
}

#[test]
fn go_from_a_screen_takes_tabs_and_overlays_and_comes_after_show() {
    let mut tui = base()
        .gate(Door::default(), false)
        .screen(
            Section::new("One"),
            Rec::<1>::default()
                .on('g', |cx| cx.go(1))
                .on('o', |cx| cx.go(3))
                .on('x', |cx| cx.go(2))
                .on('h', |cx| {
                    cx.go(1);
                    cx.show(|item| item != Item::Tab(1));
                }),
        )
        .screen(Section::new("Two"), Rec::<2>::default())
        .overlay(Section::new("Ops"), &[Key::Char('O')], Rec::<3>::default());
    tui.start();
    assert_eq!(tui.names(), ["One", "Two", "", "Ops"]);
    tui.key(ch('g'));
    assert_eq!(tui.current(), Some(1));
    assert_eq!(second(&mut tui).started, 1);
    tui.key(ch('1'));
    tui.key(ch('o'));
    assert_eq!(tui.current(), Some(3));
    assert_eq!(tui.find::<Rec<3>>().unwrap().started, 1);
    tui.key(press(KeyCode::Esc));
    assert_eq!(tui.current(), Some(0));
    tui.key(ch('x'));
    assert_eq!(tui.current(), Some(0), "the gate is never a go target");
    assert_eq!(tui.find::<Door>().unwrap().started, 0);
    assert!(!text(&mut tui).contains("the door"));
    tui.key(ch('h'));
    assert_eq!(tui.current(), Some(0), "the tab was hidden before go ran");
}

#[test]
fn tabs_keep_their_order_and_the_activity_board_comes_after_every_tab_and_overlay() {
    let jobs = || Activities::new(Section::new("Jobs")).keys(&[Key::Char('o')]);
    let mut tui = base()
        .overlay(Section::new("Ops"), &[Key::Char('O')], Rec::<3>::default())
        .activities(jobs())
        .screen(Section::new("One"), Rec::<1>::default())
        .app(Claimer::default())
        .screen(Section::new("Two"), Rec::<2>::default())
        .screen(Section::new("Four"), Rec::<4>::default());
    assert_eq!(tui.names(), ["One", "Two", "Four", "Ops", "Jobs", ""]);
    tui.start();
    tui.key(ch('3'));
    assert_eq!(tui.find::<Rec<4>>().unwrap().started, 1);
    tui.key(ch('o'));
    assert_eq!(tui.current().map(|index| tui.names()[index]), Some("Jobs"));
    tui.key(ch('o'));
    tui.key(ch('O'));
    assert_eq!(tui.current().map(|index| tui.names()[index]), Some("Ops"));
}

#[test]
fn the_activity_board_stays_after_an_overlay_added_after_it() {
    let jobs = || Activities::new(Section::new("Jobs")).keys(&[Key::Char('o')]);
    let mut tui = base()
        .activities(jobs())
        .screen(Section::new("One"), Rec::<1>::default())
        .overlay(Section::new("Ops"), &[Key::Char('O')], Rec::<3>::default())
        .app(Claimer::default())
        .screen(Section::new("Two"), Rec::<2>::default())
        .overlay(Section::new("Logs"), &[Key::Char('L')], Rec::<4>::default());
    assert_eq!(tui.names(), ["One", "Two", "Ops", "Logs", "Jobs", ""]);
    tui.start();
    tui.key(ch('o'));
    assert_eq!(tui.current().map(|index| tui.names()[index]), Some("Jobs"));
    tui.key(ch('o'));
    tui.key(ch('L'));
    assert_eq!(tui.current(), Some(3));
    assert_eq!(tui.find::<Rec<4>>().unwrap().started, 1);
    assert!(tui.go(2));
    assert_eq!(tui.find::<Rec<3>>().unwrap().started, 1);
}
