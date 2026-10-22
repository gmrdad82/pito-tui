use std::process::Command;
use std::time::{Duration, Instant};

use pito_tui::activity::{LINGER, State};
use pito_tui::command::{Exit, Heard};
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

fn rule(line: &str) -> bool {
    line.trim_start().starts_with('─')
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
    assert!(tone("halted").modifier.contains(Modifier::DIM));
    assert!(tone("queued up").modifier.contains(Modifier::DIM));
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
    post(&mut tui, move |now| {
        let then = now.checked_sub(LINGER * 2).unwrap();
        Activity::new(1, "old", then).state(State::Failed).ended(then)
    });
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
    assert!(rule(lines[at + 1]), "{text}");
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
fn a_run_stopped_before_it_starts_never_spawns_and_ends_stopped() {
    let mut tui = tui(jobs());
    act(&mut tui, run_stop);
    assert!(screen(&mut tui).contains("■ sim"));
    tui.pump();
    assert_eq!(tui.busy(), 0);
    tui.settle();
    let app = tui.find::<App>().unwrap();
    assert_eq!(app.heard, [(5, Heard::Exit(Exit::Stopped))]);
    act(&mut tui, run_stop);
    tui.settle();
    assert_eq!(tui.find::<App>().unwrap().heard.len(), 2);
}

#[test]
fn a_run_forgotten_before_it_starts_leaves_the_band() {
    let mut tui = tui(jobs());
    act(&mut tui, |cx| {
        cx.run(6, "sim", Command::new("true"));
        cx.forget(6);
    });
    assert!(!screen(&mut tui).contains("sim"));
}

#[test]
fn without_activities_posts_are_dropped_quietly() {
    let mut tui = Tui::new("probe", "1.2.3", Color::Blue).screen(Section::new("One"), App::default());
    tui.start();
    post(&mut tui, move |now| Activity::new(1, "lost", now));
    assert!(!screen(&mut tui).contains("lost"));
    assert_eq!(tui.names(), ["One"]);
}
