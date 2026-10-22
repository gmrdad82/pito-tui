use std::time::Duration;

use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::footer::{Hint, Key, Tone as Said};
use pito_tui::header::Section;
use pito_tui::layer::Event as Layered;
use pito_tui::ratatui::{
    Frame,
    buffer::Buffer,
    layout::Rect,
    style::{Color, Style},
    text::Line,
    widgets::Paragraph,
};
use pito_tui::{
    Choice, Cx, Flow, Head, Job, Layout, Modal, Palette, Phase, Pick, Row, Screen, Takeover, Toast,
    Tone, Tui, Wake, Words, dump,
};

type Act = fn(&mut Cx<'_, u32>);

#[derive(Default)]
struct Script {
    keys: Vec<KeyCode>,
    answers: Vec<bool>,
    picked: Vec<Option<usize>>,
    layers: Vec<(u64, Layered)>,
    events: Vec<u32>,
    loads: u32,
    loaded: Vec<u32>,
    reads: bool,
    waiting: bool,
    trouble: Vec<String>,
    on: Vec<(char, Act)>,
    acts: Vec<(u32, Act)>,
}

impl Script {
    fn on(mut self, key: char, act: Act) -> Self {
        self.on.push((key, act));
        self
    }

    fn event_does(mut self, event: u32, act: Act) -> Self {
        self.acts.push((event, act));
        self
    }

    fn reading(mut self) -> Self {
        self.reads = true;
        self
    }
}

impl Screen<u32> for Script {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        frame.render_widget(Paragraph::new("script body"), area);
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, u32>) {
        self.keys.push(key.code);
        if let KeyCode::Char(c) = key.code
            && let Some((_, act)) = self.on.iter().find(|(wanted, _)| *wanted == c)
        {
            act(cx);
        }
    }

    fn event(&mut self, event: u32, cx: &mut Cx<'_, u32>) {
        self.events.push(event);
        if let Some((_, act)) = self.acts.iter().find(|(wanted, _)| *wanted == event) {
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

    fn sources(&self) -> &[&str] {
        &["feed"]
    }

    fn phase(&self) -> Phase<'_> {
        if !self.trouble.is_empty() {
            let lines: Vec<Line<'_>> = self
                .trouble
                .iter()
                .map(|line| Line::from(line.as_str()))
                .collect();
            return Phase::Trouble(lines.into());
        }
        if self.waiting {
            Phase::Loading("Reading the feed...".into())
        } else {
            Phase::Ready
        }
    }

    fn load(&mut self) -> Option<Job<u32>> {
        if !self.reads {
            return None;
        }
        self.loads += 1;
        self.waiting = true;
        let answer = self.loads;
        Some(Box::new(move || answer))
    }

    fn loaded(&mut self, answer: u32, _cx: &mut Cx<'_, u32>) {
        self.waiting = false;
        self.loaded.push(answer);
    }

    fn cancel(&mut self) {
        self.waiting = false;
    }
}

struct Other;

impl Screen<u32> for Other {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        frame.render_widget(Paragraph::new("other body"), area);
    }
}

fn press(code: KeyCode) -> KeyEvent {
    KeyEvent::new(code, KeyModifiers::NONE)
}

fn ch(c: char) -> KeyEvent {
    press(KeyCode::Char(c))
}

fn ctrl_c() -> KeyEvent {
    KeyEvent::new(KeyCode::Char('c'), KeyModifiers::CONTROL)
}

fn base() -> Tui<u32> {
    Tui::new("probe", "1.2.3", Color::Blue)
        .words(Words::new().again("again to leave").yes("Yes").no("No"))
}

fn app(script: Script) -> Tui<u32> {
    let mut tui = base()
        .screen(Section::new("Main"), script)
        .screen(Section::new("Other"), Other);
    tui.start();
    tui
}

fn script(tui: &mut Tui<u32>) -> &mut Script {
    tui.find_mut::<Script>().unwrap()
}

fn shown(tui: &mut Tui<u32>) -> String {
    dump::text(&tui.frame(72, 20))
}

fn colours(buffer: &Buffer, text: &str) -> Vec<Color> {
    let area = buffer.area;
    for y in area.top()..area.bottom() {
        let row: Vec<&str> = (area.left()..area.right())
            .map(|x| buffer[(x, y)].symbol())
            .collect();
        let line: String = row.concat();
        if let Some(at) = line.find(text) {
            let x = area.left() + u16::try_from(line[..at].chars().count()).unwrap();
            let wide = u16::try_from(text.chars().count()).unwrap();
            return (x..x + wide)
                .filter(|x| buffer[(*x, y)].symbol() != " ")
                .map(|x| buffer[(x, y)].fg)
                .collect();
        }
    }
    Vec::new()
}

fn row_of(text: &str, wanted: &str) -> Option<usize> {
    text.lines().position(|line| line.contains(wanted))
}

#[test]
fn a_said_line_shows_until_the_next_key_and_hush_takes_it_back() {
    let mut tui = app(Script::default()
        .on('s', |cx| cx.say("Saved the draft", Said::Good))
        .on('h', |cx| {
            cx.say("Never seen", Said::Good);
            cx.hush();
        }));
    tui.key(ch('s'));
    assert!(shown(&mut tui).contains("Saved the draft"));
    tui.advance(Duration::from_secs(30));
    assert!(
        shown(&mut tui).contains("Saved the draft"),
        "time alone keeps it"
    );
    tui.key(ch('z'));
    assert!(!shown(&mut tui).contains("Saved the draft"));
    tui.key(ch('h'));
    assert!(!shown(&mut tui).contains("Never seen"));
}

#[test]
fn a_confirm_asks_in_the_footer_and_answers_yes_no_and_esc() {
    let mut tui = app(Script::default().on('d', |cx| cx.confirm("Drop the draft?")));
    tui.key(ch('d'));
    let asking = shown(&mut tui);
    assert!(asking.contains("Drop the draft?"), "{asking}");
    assert!(asking.contains("Yes") && asking.contains("No"), "{asking}");
    tui.key(ch('y'));
    assert!(!shown(&mut tui).contains("Drop the draft?"));
    for no in [ch('n'), press(KeyCode::Esc)] {
        tui.key(ch('d'));
        tui.key(no);
    }
    assert_eq!(script(&mut tui).answers, [true, false, false]);
    assert_eq!(script(&mut tui).keys, [KeyCode::Char('d'); 3]);
}

#[test]
fn a_pick_draws_its_rows_and_answers_the_row_or_none() {
    let mut tui = app(Script::default().on('p', |cx| {
        cx.pick(Pick::new("Choose a feed", ["alpha", "beta", "gamma"]).selected(1));
    }));
    tui.key(ch('p'));
    let picking = shown(&mut tui);
    for word in ["Choose a feed", "alpha", "beta", "gamma"] {
        assert!(picking.contains(word), "{picking}");
    }
    tui.key(ch('j'));
    tui.key(press(KeyCode::Enter));
    assert!(!shown(&mut tui).contains("Choose a feed"));
    tui.key(ch('p'));
    tui.key(press(KeyCode::Enter));
    tui.key(ch('p'));
    tui.key(press(KeyCode::Esc));
    tui.key(ch('p'));
    tui.key(ch('q'));
    assert_eq!(script(&mut tui).picked, [Some(2), Some(1), None, None]);
    assert_eq!(script(&mut tui).keys.len(), 4, "only the opening keys");
}

#[test]
fn a_title_and_title_spans_replace_the_app_name_with_their_styles() {
    let red = Style::new().fg(Color::Red);
    let mut tui = base().head(Head::new([Row::Title, Row::Sections])).screen(
        Section::new("Main"),
        Script::default()
            .on('t', |cx| cx.title("Renamed"))
            .on('s', |cx| {
                cx.title_spans([
                    ("Red", Style::new().fg(Color::Red)),
                    ("Plain", Style::new()),
                ]);
            }),
    );
    tui.start();
    let title = |tui: &mut Tui<u32>| shown(tui).lines().nth(1).unwrap_or_default().to_string();
    assert!(title(&mut tui).contains("probe"));
    tui.key(ch('t'));
    let renamed = title(&mut tui);
    assert!(
        renamed.contains("Renamed") && !renamed.contains("probe"),
        "{renamed}"
    );
    assert!(shown(&mut tui).contains("probe v1.2.3"));
    tui.key(ch('s'));
    let frame = tui.frame(72, 20);
    assert!(dump::text(&frame).contains("RedPlain"));
    let reds = colours(&frame, "Red");
    let plains = colours(&frame, "Plain");
    assert!(!reds.is_empty() && !plains.is_empty());
    assert!(reds.iter().all(|fg| *fg == red.fg.unwrap()));
    assert!(plains.iter().all(|fg| *fg != Color::Red));
}

#[test]
fn a_layout_from_a_screen_moves_the_header_down() {
    let mut tui = app(Script::default().on('l', |cx| cx.layout(Layout::new().top(3))));
    assert_eq!(row_of(&shown(&mut tui), "probe"), Some(1));
    tui.key(ch('l'));
    assert_eq!(row_of(&shown(&mut tui), "probe"), Some(3));
}

#[test]
fn a_role_paints_the_title_in_its_style_until_it_is_cleared() {
    let mut tui = base()
        .role("admin", Style::new().fg(Color::Magenta))
        .head(Head::new([Row::Title, Row::Sections]))
        .screen(
            Section::new("Main"),
            Script::default()
                .on('r', |cx| cx.role(Some("admin".into())))
                .on('u', |cx| cx.role(None))
                .on('x', |cx| cx.role(Some("nobody".into()))),
        );
    tui.start();
    let magenta = |tui: &mut Tui<u32>| {
        let frame = tui.frame(72, 20);
        let fg = colours(&frame, "probe");
        !fg.is_empty() && fg.iter().all(|fg| *fg == Color::Magenta)
    };
    assert!(!magenta(&mut tui));
    tui.key(ch('r'));
    assert!(magenta(&mut tui));
    tui.key(ch('u'));
    assert!(!magenta(&mut tui));
    tui.key(ch('x'));
    assert!(!magenta(&mut tui), "an unknown role keeps the palette");
}

#[test]
fn a_palette_from_a_screen_becomes_the_apps_palette_and_repaints_the_title() {
    let mut tui = base().head(Head::new([Row::Title, Row::Sections])).screen(
        Section::new("Main"),
        Script::default().on('p', |cx| cx.palette(Palette::new(Color::Red))),
    );
    tui.start();
    let title = |tui: &mut Tui<u32>| colours(&tui.frame(72, 20), "probe");
    let blue = title(&mut tui);
    assert!(
        !blue.is_empty() && blue.iter().all(|fg| *fg == Color::Blue),
        "{blue:?}"
    );
    tui.key(ch('p'));
    assert_eq!(*tui.current_palette(), Palette::new(Color::Red));
    let red = title(&mut tui);
    assert!(
        !red.is_empty() && red.iter().all(|fg| *fg == Color::Red),
        "{red:?}"
    );
}

#[test]
fn words_from_a_screen_reword_the_quit_guard() {
    let mut tui = app(Script::default().on('w', |cx| {
        cx.words(Words::new().again("press it again"));
    }));
    tui.key(ch('w'));
    tui.key(ctrl_c());
    let armed = shown(&mut tui);
    assert!(armed.contains("press it again"), "{armed}");
    assert!(!armed.contains("again to leave"), "{armed}");
    assert_eq!(tui.key(ctrl_c()), Flow::Quit);
}

#[test]
fn hints_from_a_screen_replace_the_apps_hints() {
    let mut tui = base().hints([Hint::new("a", "app hint")]).screen(
        Section::new("Main"),
        Script::default().on('h', |cx| cx.hints(vec![Hint::new("x", "exit now")])),
    );
    tui.start();
    assert!(shown(&mut tui).contains("app hint"));
    tui.key(ch('h'));
    let hinted = shown(&mut tui);
    assert!(
        hinted.contains("exit now") && !hinted.contains("app hint"),
        "{hinted}"
    );
}

#[test]
fn a_toast_shows_for_its_time_and_leaves_without_a_key() {
    let mut tui = app(Script::default()
        .on('t', |cx| {
            cx.toast(
                Toast::new("Copied")
                    .tone(Tone::Good)
                    .lasting(Duration::from_secs(1)),
            );
        })
        .on('d', |cx| cx.toast(Toast::new("Default length"))));
    tui.key(ch('t'));
    assert!(shown(&mut tui).contains("Copied"));
    tui.advance(Duration::from_millis(999));
    assert!(shown(&mut tui).contains("Copied"));
    tui.advance(Duration::from_millis(1));
    assert!(!shown(&mut tui).contains("Copied"));
    tui.key(ch('d'));
    tui.advance(Duration::from_millis(2999));
    assert!(shown(&mut tui).contains("Default length"));
    tui.advance(Duration::from_millis(1));
    assert!(!shown(&mut tui).contains("Default length"));
}

#[test]
fn modals_open_close_and_change_by_id_and_the_top_one_hears_keys() {
    let mut tui = app(Script::default()
        .event_does(1, |cx| {
            cx.open(Modal::new(1, "First").text([Line::from("first body")]));
            cx.open(Modal::new(2, "Second").text([Line::from("second body")]));
        })
        .event_does(2, |cx| cx.close(1))
        .event_does(3, |cx| {
            assert!(cx.modal(9).is_none());
            if let Some(modal) = cx.modal(2) {
                modal.set_status(Some(("half way".into(), Tone::Warn)));
            }
        })
        .event_does(4, |cx| {
            cx.open(Modal::new(2, "Second again").text([Line::from("fresh body")]));
        }));
    tui.event(0, 1);
    assert!(shown(&mut tui).contains("second body"));
    tui.event(0, 3);
    assert!(shown(&mut tui).contains("half way"));
    tui.event(0, 4);
    let reopened = shown(&mut tui);
    assert!(reopened.contains("fresh body") && !reopened.contains("half way"));
    tui.event(0, 2);
    tui.key(ch('x'));
    assert!(script(&mut tui).keys.is_empty());
    tui.key(press(KeyCode::Esc));
    assert_eq!(script(&mut tui).layers, [(2, Layered::Closed)]);
    let closed = shown(&mut tui);
    assert!(!closed.contains("first body") && !closed.contains("fresh body"));
    tui.key(ch('x'));
    assert_eq!(script(&mut tui).keys, [KeyCode::Char('x')]);
}

#[test]
fn a_modal_list_reports_moves_and_the_choice_and_an_alert_closes_on_enter_or_q() {
    let mut tui = app(Script::default()
        .on('m', |cx| {
            cx.open(Modal::new(5, "Pick one").list([Choice::new("first"), Choice::new("second")]));
        })
        .on('a', |cx| cx.alert(6, "Saved", [Line::from("all good")])));
    tui.key(ch('m'));
    tui.key(press(KeyCode::Down));
    tui.key(press(KeyCode::Enter));
    assert_eq!(
        script(&mut tui).layers,
        [
            (5, Layered::Moved { part: 0, choice: 1 }),
            (5, Layered::Chosen { part: 0, choice: 1 }),
        ]
    );
    tui.key(press(KeyCode::Esc));
    for close in [press(KeyCode::Enter), ch('q')] {
        tui.key(ch('a'));
        assert!(shown(&mut tui).contains("all good"));
        tui.key(close);
        assert!(!shown(&mut tui).contains("all good"));
    }
    assert_eq!(
        script(&mut tui).layers[2..],
        [
            (5, Layered::Closed),
            (6, Layered::Closed),
            (6, Layered::Closed),
        ]
    );
}

#[test]
fn a_takeover_covers_the_whole_screen_until_it_ends() {
    let mut tui = app(Script::default()
        .on('t', |cx| {
            cx.takeover(
                Takeover::new(3, "Moving house")
                    .label("packing boxes")
                    .cancel(&[Key::Esc]),
            );
        })
        .event_does(1, |cx| cx.end_takeover()));
    assert!(shown(&mut tui).contains("script body"));
    tui.key(ch('t'));
    let taken = shown(&mut tui);
    assert!(taken.contains("Moving house"), "{taken}");
    assert!(taken.contains("packing boxes"), "{taken}");
    assert!(
        !taken.contains("script body") && !taken.contains("Other"),
        "{taken}"
    );
    tui.event(0, 1);
    assert!(shown(&mut tui).contains("script body"));
    assert!(script(&mut tui).layers.is_empty());
}

struct Lock;

impl Screen<u32> for Lock {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        frame.render_widget(Paragraph::new("locked away"), area);
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, u32>) {
        if key.code == KeyCode::Enter {
            cx.gate(false);
        }
    }
}

#[test]
fn a_gate_shut_from_a_screen_hides_the_tabs_until_it_opens() {
    let mut tui = base()
        .gate(Lock, false)
        .screen(
            Section::new("Main"),
            Script::default().on('g', |cx| cx.gate(true)),
        )
        .screen(Section::new("Other"), Other);
    tui.start();
    assert!(shown(&mut tui).contains("Other"));
    tui.key(ch('g'));
    let gated = shown(&mut tui);
    assert!(gated.contains("locked away"), "{gated}");
    assert!(
        !gated.contains("Other") && !gated.contains("script body"),
        "{gated}"
    );
    tui.key(press(KeyCode::Enter));
    let open = shown(&mut tui);
    assert!(
        open.contains("script body") && open.contains("Other"),
        "{open}"
    );
}

#[test]
fn a_gate_without_a_gate_screen_changes_nothing() {
    let mut tui = app(Script::default().on('g', |cx| cx.gate(true)));
    tui.key(ch('g'));
    assert_eq!(tui.current(), Some(0));
    assert!(shown(&mut tui).contains("script body"));
}

#[test]
fn detached_work_answers_its_screen_and_an_epoch_drops_the_queued_work() {
    let mut tui = app(Script::default()
        .on('d', |cx| cx.detach(|| 41))
        .on('e', |cx| cx.epoch()));
    tui.key(ch('d'));
    tui.settle();
    assert_eq!(script(&mut tui).events, [41]);
    tui.key(ch('d'));
    tui.key(ch('e'));
    tui.settle();
    assert_eq!(
        script(&mut tui).events,
        [41],
        "the epoch dropped the second"
    );
}

#[test]
fn a_wakers_message_sent_before_an_epoch_is_dropped_and_one_after_lands() {
    let mut tui = app(Script::default().on('e', |cx| cx.epoch()));
    let waker = tui.waker(0);
    assert!(waker.send(5));
    tui.key(ch('e'));
    tui.settle();
    assert!(script(&mut tui).events.is_empty());
    assert!(waker.send(6));
    tui.settle();
    assert_eq!(script(&mut tui).events, [6]);
}

#[test]
fn a_stamped_wake_from_before_an_epoch_is_dropped_and_one_from_after_lands() {
    let mut tui = app(Script::default().on('e', |cx| cx.epoch()));
    let sender = tui.sender();
    assert!(sender.send(Wake::Stamped(0, 0, 4)).is_ok());
    tui.settle();
    assert_eq!(script(&mut tui).events, [4]);
    tui.key(ch('e'));
    assert!(sender.send(Wake::Stamped(0, 0, 5)).is_ok());
    assert!(sender.send(Wake::Stamped(1, 0, 6)).is_ok());
    assert!(sender.send(Wake::Stamped(2, 0, 7)).is_ok());
    tui.settle();
    assert_eq!(script(&mut tui).events, [4, 6]);
}

#[test]
fn an_epoch_drops_a_running_read_and_every_started_screen_reads_again() {
    let mut tui = app(Script::default().reading().on('e', |cx| cx.epoch()));
    tui.shot(72, 20, &[], false);
    assert_eq!(script(&mut tui).loads, 1);
    tui.key(ch('e'));
    assert!(!tui.loaded(0, 1, 1), "the first read is stale now");
    assert!(script(&mut tui).loaded.is_empty());
    tui.settle();
    assert_eq!(script(&mut tui).loads, 2);
    assert_eq!(script(&mut tui).loaded, [2]);
}

#[test]
fn a_reload_after_a_read_landed_reads_again_under_a_new_generation() {
    let mut tui = app(Script::default().reading().on('r', |cx| cx.reload()));
    tui.settle();
    assert_eq!(script(&mut tui).loaded, [1]);
    tui.key(ch('r'));
    tui.shot(72, 20, &[], false);
    assert_eq!(script(&mut tui).loads, 2);
    assert!(!tui.loaded(0, 1, 1), "the first generation is spent");
    assert!(tui.loaded(0, 2, 2));
    assert_eq!(script(&mut tui).loaded, [1, 2]);
    assert!(!tui.loaded(7, 1, 1), "no such screen");
}

#[test]
fn a_reload_while_a_read_runs_keeps_its_answer_then_reads_again() {
    let mut tui = app(Script::default().reading().on('r', |cx| cx.reload()));
    tui.shot(72, 20, &[], false);
    assert_eq!(script(&mut tui).loads, 1);
    tui.key(ch('r'));
    tui.shot(72, 20, &[], false);
    assert_eq!(
        script(&mut tui).loads,
        1,
        "no second read while the first runs"
    );
    assert!(tui.loaded(0, 1, 1), "the running read still answers");
    assert_eq!(script(&mut tui).loaded, [1]);
    tui.shot(72, 20, &[], false);
    assert_eq!(
        script(&mut tui).loads,
        2,
        "the reload reads once the first landed"
    );
    assert!(tui.loaded(0, 2, 2));
    assert_eq!(script(&mut tui).loaded, [1, 2]);
}

#[test]
fn a_changed_source_from_a_screen_reads_every_screen_on_it_again() {
    let mut tui = app(Script::default().reading().on('f', |cx| cx.changed("feed")));
    tui.settle();
    tui.key(ch('f'));
    tui.settle();
    assert_eq!(script(&mut tui).loaded, [1, 2]);
    tui.changed("other");
    tui.settle();
    assert_eq!(script(&mut tui).loads, 2);
}

#[test]
fn quit_from_a_key_or_the_apps_own_keys_ends_the_loop() {
    let mut tui = app(Script::default().on('Q', |cx| cx.quit()));
    assert_eq!(tui.key(ch('z')), Flow::Stay);
    assert_eq!(tui.key(ch('Q')), Flow::Quit);

    let mut hooked = base()
        .on_key(|key, cx| {
            if key.code == KeyCode::F(10) {
                cx.quit();
                return true;
            }
            false
        })
        .screen(Section::new("Main"), Script::default());
    hooked.start();
    assert_eq!(hooked.key(press(KeyCode::F(10))), Flow::Quit);
    assert!(script(&mut hooked).keys.is_empty());
}

fn slow() -> Tui<u32> {
    let mut tui = base()
        .hourglass_timing(Duration::from_millis(300), Duration::from_millis(200))
        .screen(Section::new("Main"), Script::default().reading());
    tui.start();
    tui
}

#[test]
fn the_hourglass_waits_for_its_delay_and_stays_its_least_time() {
    let mut tui = slow();
    let first = dump::text(&tui.shot(72, 20, &[], false));
    assert!(!first.contains("Reading the feed...") && !first.contains("script body"));
    tui.advance(Duration::from_millis(299));
    assert!(!shown(&mut tui).contains("Reading the feed..."));
    tui.advance(Duration::from_millis(1));
    assert!(shown(&mut tui).contains("Reading the feed..."));

    assert!(tui.loaded(0, 1, 1));
    let kept = shown(&mut tui);
    assert!(kept.contains("Reading the feed...") && !kept.contains("script body"));
    tui.advance(Duration::from_millis(199));
    assert!(!shown(&mut tui).contains("script body"));
    tui.advance(Duration::from_millis(1));
    let done = shown(&mut tui);
    assert!(done.contains("script body") && !done.contains("Reading the feed..."));
}

#[test]
fn a_read_quicker_than_the_delay_never_shows_the_hourglass() {
    let mut tui = slow();
    tui.shot(72, 20, &[], false);
    tui.advance(Duration::from_millis(250));
    assert!(!shown(&mut tui).contains("Reading the feed..."));
    assert!(tui.loaded(0, 1, 1));
    assert!(shown(&mut tui).contains("script body"));
    tui.advance(Duration::from_millis(500));
    assert!(!shown(&mut tui).contains("Reading the feed..."));
}

#[test]
fn trouble_lines_draw_in_place_of_the_screen() {
    let mut tui = app(Script {
        trouble: vec!["The feed is down.".into(), "r reads it again.".into()],
        ..Script::default()
    });
    let text = shown(&mut tui);
    assert!(text.contains("The feed is down."), "{text}");
    assert!(text.contains("r reads it again."), "{text}");
    assert!(!text.contains("script body"), "{text}");
    let lines: Vec<&str> = text.lines().collect();
    let down = row_of(&text, "The feed is down.").unwrap();
    assert!(lines[down + 1].contains("r reads it again."), "{text}");
}

#[test]
fn an_eager_app_reads_screens_that_were_never_shown() {
    let build = |eager| {
        let mut tui = base()
            .eager(eager)
            .screen(Section::new("Other"), Other)
            .screen(Section::new("Main"), Script::default().reading());
        tui.start();
        tui.settle();
        tui
    };
    let mut lazy = build(false);
    assert_eq!(script(&mut lazy).loads, 0);
    let mut eager = build(true);
    assert_eq!(script(&mut eager).loaded, [1]);
}
