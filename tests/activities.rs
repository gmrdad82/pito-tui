use std::process::Command;
use std::time::{Duration, Instant};

use pito_tui::activity::{LINGER, State};
use pito_tui::command::{Exit, Heard, Stream};
use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::footer::Key;
use pito_tui::header::Section;
use pito_tui::progress::SPIN;
use pito_tui::ratatui::{
    Frame,
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier},
    text::Line,
};
use pito_tui::{Activities, Activity, Cx, Palette, Screen, Tui, dump};

const WIDTH: u16 = 60;
const HEIGHT: u16 = 24;

#[derive(Default)]
struct App {
    heard: Vec<(u64, Heard)>,
}

type Work = dyn FnOnce(&mut Cx<'_, Ask>) + Send;

struct Ask(Box<Work>);

impl Screen<Ask> for App {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn event(&mut self, ask: Ask, cx: &mut Cx<'_, Ask>) {
        (ask.0)(cx);
    }

    fn heard(&mut self, id: u64, heard: Heard, _cx: &mut Cx<'_, Ask>) {
        self.heard.push((id, heard));
    }
}

fn act(tui: &mut Tui<Ask>, work: impl FnOnce(&mut Cx<'_, Ask>) + Send + 'static) {
    tui.event(0, Ask(Box::new(work)));
}

fn post(tui: &mut Tui<Ask>, activity: impl FnOnce(Instant) -> Activity + Send + 'static) {
    act(tui, move |cx| {
        let now = cx.now();
        cx.activity(activity(now));
    });
}

fn press(code: KeyCode) -> KeyEvent {
    KeyEvent::new(code, KeyModifiers::NONE)
}

fn tui(words: Activities) -> Tui<Ask> {
    let mut tui = Tui::new("probe", "1.2.3", Color::Blue)
        .screen(Section::new("One"), App::default())
        .activities(words.keys(&[Key::Char('o')]));
    tui.start();
    tui
}

fn jobs() -> Activities {
    Activities::new(Section::new("Jobs"))
        .more(|hidden| format!("+{hidden} more"))
        .empty("Nothing has run yet")
}

fn screen(tui: &mut Tui<Ask>) -> String {
    dump::text(&tui.frame(WIDTH, HEIGHT))
}

fn find(buffer: &Buffer, text: &str) -> Option<(u16, u16)> {
    let rows = dump::text(buffer);
    rows.lines().enumerate().find_map(|(y, row)| {
        let at = row.find(text)?;
        let x = row[..at].chars().count();
        Some((u16::try_from(x).ok()?, u16::try_from(y).ok()?))
    })
}

fn run_stop(cx: &mut Cx<'_, Ask>) {
    cx.run(5, "sim", Command::new("true"));
    cx.stop(5);
}

fn current(tui: &Tui<Ask>) -> Option<&str> {
    tui.current().map(|index| tui.names()[index])
}

#[test]
fn a_posted_activity_shows_on_the_band_with_its_glyph_label_and_status() {
    let mut tui = tui(jobs());
    post(&mut tui, move |now| Activity::new(1, "build", now).status("compiling"));
    let text = screen(&mut tui);
    let line = text.lines().find(|line| line.contains("build")).unwrap();
    assert!(line.trim_start().starts_with("⠋ build"), "{text}");
    assert!(line.contains("compiling"), "{text}");
    tui.advance(SPIN);
    let text = screen(&mut tui);
    assert!(text.contains("⠙ build"), "{text}");
}

#[test]
fn each_finished_state_shows_its_glyph_and_its_status_in_its_tone() {
    let mut tui = tui(jobs());
    let palette = *tui.current_palette();
    for (id, state, status) in [
        (1, State::Done, "all good"),
        (2, State::Failed, "it broke"),
        (3, State::Stopped, "halted"),
        (4, State::Waiting, "queued up"),
    ] {
        post(
            &mut tui,
            move |now| Activity::new(id, format!("task{id}"), now).state(state).status(status),
        );
    }
    let buffer = tui.frame(WIDTH, HEIGHT);
    let text = dump::text(&buffer);
    for (glyph, label) in [("✓", "task1"), ("✗", "task2"), ("■", "task3"), ("⧗", "task4")] {
        assert!(text.contains(&format!("{glyph} {label}")), "{text}");
    }
    let tone = |status: &str| {
        let (x, y) = find(&buffer, status).unwrap();
        buffer[(x, y)].clone()
    };
    assert_eq!(tone("all good").fg, palette.good.fg.unwrap());
    assert!(tone("all good").modifier.contains(Modifier::DIM));
    assert_eq!(tone("it broke").fg, palette.bad.fg.unwrap());
    assert_ne!(tone("all good").fg, tone("it broke").fg);
}

#[test]
fn finished_activities_linger_then_leave_the_band_but_stay_on_the_board() {
    let mut tui = tui(jobs());
    post(&mut tui, move |now| Activity::new(1, "sync", now).state(State::Done));
    post(&mut tui, move |now| Activity::new(2, "watch", now));
    assert!(screen(&mut tui).contains("✓ sync"));
    tui.advance(LINGER - Duration::from_millis(1));
    assert!(screen(&mut tui).contains("✓ sync"));
    tui.advance(Duration::from_millis(1));
    let text = screen(&mut tui);
    assert!(!text.contains("sync"), "{text}");
    assert!(text.contains("watch"), "{text}");
    tui.advance(LINGER * 10);
    assert!(screen(&mut tui).contains("watch"));

    tui.key(press(KeyCode::Char('o')));
    assert_eq!(current(&tui), Some("Jobs"));
    let text = screen(&mut tui);
    assert!(text.contains("sync") && text.contains("watch"), "{text}");
}

#[test]
fn an_activity_that_ended_long_ago_never_reaches_the_band() {
    let mut tui = tui(jobs());
    let start = std::sync::Arc::new(std::sync::Mutex::new(None));
    let seen = std::sync::Arc::clone(&start);
    act(&mut tui, move |cx| *seen.lock().unwrap() = Some(cx.now()));
    let then = start.lock().unwrap().unwrap();
    tui.advance(LINGER * 2);
    post(&mut tui, move |_| Activity::new(1, "old", then).state(State::Failed).ended(then));
    assert!(!screen(&mut tui).contains("old"));
}

#[test]
fn more_than_fit_shows_the_more_words_with_the_hidden_count_on_the_last_band_row() {
    let mut tui = tui(jobs());
    let total = 30;
    for id in 0..total {
        post(&mut tui, move |now| {
            Activity::new(id, format!("job{id:02}"), now + Duration::from_millis(id))
        });
    }
    let text = screen(&mut tui);
    let lines: Vec<&str> = text.lines().collect();
    let at = lines.iter().position(|line| line.contains(" more")).unwrap();
    let hidden: u64 = lines[at]
        .trim()
        .strip_prefix('+')
        .and_then(|rest| rest.strip_suffix(" more"))
        .unwrap()
        .parse()
        .unwrap();
    let shown = lines.iter().filter(|line| line.contains("job")).count() as u64;
    assert!(shown > 0, "{text}");
    assert_eq!(shown + hidden, total, "{text}");
    assert!(lines[at - 1].contains("job"), "{text}");
    let first = lines.iter().position(|line| line.contains("job")).unwrap();
    assert!(lines[first].contains("job29"), "{text}");
}

#[test]
fn the_band_turns_off_and_on_at_the_app_s_word() {
    let mut tui = tui(jobs());
    post(&mut tui, move |now| Activity::new(1, "build", now));
    act(&mut tui, |cx| cx.band(false));
    assert!(!screen(&mut tui).contains("build"));
    act(&mut tui, |cx| cx.band(true));
    assert!(screen(&mut tui).contains("build"));
}

#[test]
fn activities_replace_every_app_post_at_once() {
    let mut tui = tui(jobs());
    post(&mut tui, move |now| Activity::new(1, "alpha", now));
    post(&mut tui, move |now| Activity::new(2, "beta", now));
    act(&mut tui, |cx| {
        let now = cx.now();
        cx.activities([Activity::new(3, "gamma", now)]);
    });
    let text = screen(&mut tui);
    assert!(text.contains("gamma"), "{text}");
    assert!(!text.contains("alpha") && !text.contains("beta"), "{text}");
    act(&mut tui, |cx| cx.activities(Vec::new()));
    assert!(!screen(&mut tui).contains("gamma"));
}

#[test]
fn a_post_with_the_same_id_updates_the_row_in_place() {
    let mut tui = tui(jobs());
    post(&mut tui, move |now| Activity::new(1, "build", now).status("step one"));
    post(&mut tui, move |now| Activity::new(1, "build", now).status("step two"));
    let text = screen(&mut tui);
    assert_eq!(text.matches("build").count(), 1, "{text}");
    assert!(text.contains("step two") && !text.contains("step one"), "{text}");
}

#[test]
fn the_board_lists_every_activity_newest_first_and_opens_one_to_its_detail() {
    let mut tui = tui(jobs());
    post(
        &mut tui,
        move |now| Activity::new(1, "older", now).detail([Line::from("first detail")]),
    );
    post(
        &mut tui,
        move |now| Activity::new(2, "newer", now + Duration::from_millis(1))
            .detail([Line::from("second detail")]),
    );
    tui.key(press(KeyCode::Char('o')));
    let text = screen(&mut tui);
    let newer = text.find("newer").unwrap();
    let older = text.find("older").unwrap();
    assert!(newer < older, "{text}");

    tui.key(press(KeyCode::Down));
    tui.key(press(KeyCode::Enter));
    let text = screen(&mut tui);
    assert!(text.contains("Jobs / older"), "{text}");
    assert!(text.contains("first detail") && !text.contains("second detail"), "{text}");
    tui.key(press(KeyCode::Esc));
    let text = screen(&mut tui);
    assert!(!text.contains("first detail"), "{text}");
    assert_eq!(current(&tui), Some("Jobs"));
}

#[test]
fn an_empty_board_shows_the_app_s_empty_words() {
    let mut tui = tui(jobs());
    tui.key(press(KeyCode::Char('o')));
    assert!(screen(&mut tui).contains("Nothing has run yet"));
}

#[test]
fn forget_removes_an_activity_from_the_band_and_the_board_and_closes_it_if_open() {
    let mut tui = tui(jobs());
    post(&mut tui, move |now| Activity::new(1, "keep", now));
    post(
        &mut tui,
        move |now| Activity::new(2, "drop", now + Duration::from_millis(1)).detail([Line::from("gone soon")]),
    );
    tui.key(press(KeyCode::Char('o')));
    tui.key(press(KeyCode::Enter));
    assert!(screen(&mut tui).contains("gone soon"));
    act(&mut tui, |cx| cx.forget(2));
    let text = screen(&mut tui);
    assert!(!text.contains("gone soon") && !text.contains("drop"), "{text}");
    assert!(text.contains("keep"), "{text}");
    act(&mut tui, |cx| cx.forget(1));
    assert!(screen(&mut tui).contains("Nothing has run yet"));
    act(&mut tui, |cx| cx.forget(99));
    assert!(screen(&mut tui).contains("Nothing has run yet"));
}

#[test]
fn a_run_stopped_while_queued_never_spawns_and_stops_again_when_asked_again() {
    let mut tui = tui(jobs());
    act(&mut tui, run_stop);
    tui.pump();
    assert_eq!(tui.busy(), 0);
    tui.settle();
    act(&mut tui, run_stop);
    tui.pump();
    tui.settle();
    let app = tui.find::<App>().unwrap();
    assert_eq!(
        app.heard,
        [(5, Heard::Exit(Exit::Stopped)), (5, Heard::Exit(Exit::Stopped))]
    );
}

#[cfg(unix)]
fn exited(tui: &mut Tui<Ask>, id: u64) -> Vec<Heard> {
    let limit = Instant::now() + Duration::from_secs(10);
    tui.pump();
    loop {
        tui.settle();
        let heard: Vec<Heard> = tui
            .find::<App>()
            .unwrap()
            .heard
            .iter()
            .filter(|(from, _)| *from == id)
            .map(|(_, heard)| heard.clone())
            .collect();
        if heard.iter().any(|heard| matches!(heard, Heard::Exit(_))) {
            return heard;
        }
        assert!(Instant::now() < limit, "the run never ended: {heard:?}");
        std::thread::sleep(Duration::from_millis(5));
    }
}

#[cfg(unix)]
fn shell(script: &str) -> Command {
    let mut command = Command::new("sh");
    command.args(["-c", script]);
    command
}

#[cfg(unix)]
const STEP: &str = r#"{"v":1,"state":"progress","stage":"build","fraction":0.5}"#;

#[cfg(unix)]
fn both(cx: &mut Cx<'_, Ask>) {
    let script = format!("printf 'out one\\n'; printf '%s\\nerr one\\n' '{STEP}' >&2; exit 3");
    cx.run_from(7, "mixed", shell(&script), Stream::Both);
}

#[cfg(unix)]
fn lines(heard: &[Heard]) -> Vec<(Stream, String, bool)> {
    heard
        .iter()
        .filter_map(|heard| match heard {
            Heard::Line {
                stream,
                text,
                progress,
                ..
            } => Some((*stream, text.clone(), progress.is_some())),
            _ => None,
        })
        .collect()
}

#[cfg(unix)]
#[test]
fn a_run_from_both_streams_reaches_the_screen_and_ends_failed_on_the_band_with_its_detail() {
    let mut tui = tui(jobs());
    act(&mut tui, both);
    assert!(screen(&mut tui).contains("mixed"));
    let heard = exited(&mut tui, 7);
    let mut said = lines(&heard);
    said.sort_by_key(|(stream, _, _)| *stream == Stream::Stderr);
    assert_eq!(
        said,
        [
            (Stream::Stdout, "out one".to_string(), false),
            (Stream::Stderr, STEP.to_string(), true),
            (Stream::Stderr, "err one".to_string(), false),
        ]
    );
    assert_eq!(heard.last(), Some(&Heard::Exit(Exit::Code(3))));
    let text = screen(&mut tui);
    assert!(text.contains("✗ mixed"), "{text}");

    tui.key(press(KeyCode::Char('o')));
    tui.key(press(KeyCode::Enter));
    let text = screen(&mut tui);
    assert!(text.contains("out one") && text.contains("err one"), "{text}");
    assert!(text.contains("build  progress  50%") && !text.contains(STEP), "{text}");
}

#[cfg(unix)]
#[test]
fn a_run_from_without_activities_still_reaches_the_screen_line_by_line() {
    let mut tui =
        Tui::new("probe", "1.2.3", Color::Blue).screen(Section::new("One"), App::default());
    tui.start();
    act(&mut tui, both);
    let heard = exited(&mut tui, 7);
    assert_eq!(lines(&heard).len(), 3);
    assert!(lines(&heard).contains(&(Stream::Stderr, STEP.to_string(), true)));
    assert_eq!(heard.last(), Some(&Heard::Exit(Exit::Code(3))));
    assert_eq!(tui.names(), ["One"]);
}

#[cfg(unix)]
#[test]
fn a_run_forgotten_while_queued_still_runs_for_its_screen_and_never_returns_to_the_band() {
    let mut tui = tui(jobs());
    act(&mut tui, |cx| {
        cx.run(6, "sim", Command::new("true"));
        cx.forget(6);
    });
    assert!(!screen(&mut tui).contains("sim"));
    let heard = exited(&mut tui, 6);
    assert_eq!(heard, [Heard::Exit(Exit::Code(0))]);
    tui.settle();
    assert!(!screen(&mut tui).contains("sim"));
    tui.key(press(KeyCode::Char('o')));
    assert!(screen(&mut tui).contains("Nothing has run yet"));
}

#[test]
fn without_activities_posts_are_dropped_quietly() {
    let mut tui = Tui::new("probe", "1.2.3", Color::Blue).screen(Section::new("One"), App::default());
    tui.start();
    post(&mut tui, move |now| Activity::new(1, "lost", now));
    assert!(!screen(&mut tui).contains("lost"));
    assert_eq!(tui.names(), ["One"]);
}
