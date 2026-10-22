use std::sync::Arc;

use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::list::{Bar, Count};
use pito_tui::log::{Lines, Turn};
use pito_tui::ratatui::{
    Terminal,
    backend::TestBackend,
    buffer::Buffer,
    style::{Color, Modifier},
    text::Line,
};
use pito_tui::{Log, Palette, dump};

const TOTAL: usize = 20_000;
const WIDTH: u16 = 40;
const HEIGHT: u16 = 10;

fn press(code: KeyCode) -> KeyEvent {
    KeyEvent::new(code, KeyModifiers::NONE)
}

fn typed(c: char) -> KeyEvent {
    press(KeyCode::Char(c))
}

fn palette() -> Palette {
    Palette::new(Color::Blue)
        .bar(Some(Bar::new("│", "┃")))
        .count(Some(Count::new("–", " of ").group(",")))
}

fn lines(texts: impl IntoIterator<Item = String>) -> Lines {
    Arc::new(texts.into_iter().map(Line::from).collect())
}

fn long() -> Lines {
    lines((0..TOTAL).map(|index| {
        if index % 5000 == 4999 {
            format!("needle {index}")
        } else {
            format!("line {index}")
        }
    }))
}

fn draw(log: &mut Log, palette: &Palette, width: u16, height: u16) -> Buffer {
    let mut terminal = Terminal::new(TestBackend::new(width, height)).unwrap();
    terminal
        .draw(|frame| log.draw(frame, frame.area(), palette))
        .unwrap();
    terminal.backend().buffer().clone()
}

fn rows(buffer: &Buffer) -> Vec<String> {
    let text = dump::text(buffer);
    let mut rows: Vec<String> = text.lines().map(str::to_string).collect();
    rows.resize(usize::from(buffer.area.height), String::new());
    rows
}

fn loaded(texts: Lines) -> Log {
    let mut log = Log::new("Find");
    log.set_lines(texts);
    log
}

fn search(log: &mut Log, query: &str) {
    assert_eq!(log.key(typed('/')), Turn::Taken);
    assert!(log.typing());
    for c in query.chars() {
        assert_eq!(log.key(typed(c)), Turn::Taken);
    }
}

#[test]
fn twenty_thousand_lines_scroll_with_the_bar_and_the_count() {
    let palette = palette();
    let mut log = loaded(long());
    assert_eq!(log.len(), TOTAL);
    let top = rows(&draw(&mut log, &palette, WIDTH, HEIGHT));
    assert!(top[0].starts_with("line 0"), "{top:?}");
    assert!(top[8].starts_with("line 8"), "{top:?}");
    assert!(top[9].ends_with("1–9 of 20,000"), "{top:?}");
    let buffer = draw(&mut log, &palette, WIDTH, HEIGHT);
    assert_eq!(buffer[(WIDTH - 1, 0)].symbol(), "┃");
    assert_eq!(buffer[(WIDTH - 1, 8)].symbol(), "│");

    assert_eq!(log.key(press(KeyCode::Down)), Turn::Taken);
    assert_eq!(log.top(), 1);
    assert_eq!(log.key(press(KeyCode::PageDown)), Turn::Taken);
    assert_eq!(log.top(), 10);
    assert_eq!(log.key(press(KeyCode::PageUp)), Turn::Taken);
    assert_eq!(log.top(), 1);
    assert_eq!(log.key(press(KeyCode::Up)), Turn::Taken);
    assert_eq!(log.key(press(KeyCode::Up)), Turn::Taken);
    assert_eq!(log.top(), 0);

    assert_eq!(log.key(press(KeyCode::End)), Turn::Taken);
    assert_eq!(log.top(), TOTAL - 9);
    let buffer = draw(&mut log, &palette, WIDTH, HEIGHT);
    let end = rows(&buffer);
    assert!(end[8].starts_with("needle 19999"), "{end:?}");
    assert!(end[9].ends_with("19,992–20,000 of 20,000"), "{end:?}");
    assert_eq!(buffer[(WIDTH - 1, 8)].symbol(), "┃");
    assert_eq!(buffer[(WIDTH - 1, 0)].symbol(), "│");
    assert_eq!(log.key(press(KeyCode::Down)), Turn::Taken);
    assert_eq!(log.top(), TOTAL - 9);
    assert_eq!(log.key(typed('g')), Turn::Taken);
    assert_eq!(log.top(), 0);
    assert_eq!(log.key(typed('G')), Turn::Taken);
    assert_eq!(log.top(), TOTAL - 9);
}

#[test]
fn a_log_that_fits_draws_no_bar_and_no_count() {
    let palette = palette();
    let mut log = loaded(lines((0..3).map(|index| format!("row {index}"))));
    let buffer = draw(&mut log, &palette, WIDTH, HEIGHT);
    let shown = rows(&buffer);
    assert_eq!(&shown[..4], ["row 0", "row 1", "row 2", ""]);
    assert!(shown.iter().all(|row| !row.contains(" of ")));
    assert!((0..HEIGHT).all(|y| buffer[(WIDTH - 1, y)].symbol() == " "));
    assert_eq!(log.key(press(KeyCode::Down)), Turn::Taken);
    assert_eq!(log.top(), 0);
}

#[test]
fn a_palette_without_bar_or_count_gives_every_row_and_column_to_the_lines() {
    let mut log = loaded(long());
    let buffer = draw(&mut log, &Palette::new(Color::Blue), WIDTH, HEIGHT);
    let shown = rows(&buffer);
    assert!(shown[9].starts_with("line 9"), "{shown:?}");
    assert_eq!(log.key(press(KeyCode::End)), Turn::Taken);
    assert_eq!(log.top(), TOTAL - 10);
}

#[test]
fn find_moves_to_the_next_and_previous_match_and_wraps() {
    let palette = palette();
    let mut log = loaded(long());
    draw(&mut log, &palette, WIDTH, HEIGHT);
    search(&mut log, "needle");
    assert_eq!(log.query(), "needle");
    assert_eq!(log.hits(), 4);
    assert_eq!(log.hit(), Some(0));
    assert_eq!(log.top(), 4996);
    assert!(log.bar().is_some());
    assert_eq!(log.key(press(KeyCode::Enter)), Turn::Taken);
    assert!(!log.typing());
    assert!(log.bar().is_none());

    let buffer = draw(&mut log, &palette, WIDTH, HEIGHT);
    let shown = rows(&buffer);
    let at = shown.iter().position(|row| row == "needle 4999").unwrap();
    let row = u16::try_from(at).unwrap();
    assert!(buffer[(0, row)].modifier.contains(Modifier::BOLD));
    assert!(!buffer[(0, row - 1)].modifier.contains(Modifier::BOLD));

    assert_eq!(log.key(typed('n')), Turn::Taken);
    assert_eq!(log.hit(), Some(1));
    assert_eq!(log.top(), 9996);
    assert_eq!(log.key(typed('n')), Turn::Taken);
    assert_eq!(log.key(typed('n')), Turn::Taken);
    assert_eq!(log.hit(), Some(3));
    assert_eq!(log.top(), TOTAL - 9);
    assert_eq!(log.key(typed('n')), Turn::Taken);
    assert_eq!(log.hit(), Some(0));
    assert_eq!(log.top(), 4996);
    assert_eq!(log.key(typed('N')), Turn::Taken);
    assert_eq!(log.hit(), Some(3));
    assert_eq!(log.key(typed('N')), Turn::Taken);
    assert_eq!(log.hit(), Some(2));
    assert_eq!(log.top(), 14996);
}

#[test]
fn find_starts_from_the_line_in_view() {
    let palette = palette();
    let mut log = loaded(long());
    draw(&mut log, &palette, WIDTH, HEIGHT);
    for _ in 0..1200 {
        log.key(press(KeyCode::PageDown));
    }
    assert_eq!(log.top(), 10800);
    search(&mut log, "NEEDLE");
    assert_eq!(log.hit(), Some(2));
    assert_eq!(log.top(), 14996);
}

fn above_the_view() -> Log {
    let mut log = loaded(long());
    draw(&mut log, &palette(), WIDTH, HEIGHT);
    log.key(press(KeyCode::End));
    search(&mut log, "needle 4999");
    log.key(press(KeyCode::Enter));
    assert_eq!(log.hits(), 2);
    assert_eq!(log.hit(), None);
    assert_eq!(log.top(), TOTAL - 9);
    log
}

#[test]
fn a_search_below_every_match_holds_the_view_then_previous_takes_the_last_match() {
    let mut log = above_the_view();
    assert_eq!(log.key(typed('N')), Turn::Taken);
    assert_eq!(log.hit(), Some(1));
    assert_eq!(log.top(), 14996);
}

#[test]
fn a_search_below_every_match_holds_the_view_then_next_takes_the_first_match() {
    let mut log = above_the_view();
    assert_eq!(log.key(typed('n')), Turn::Taken);
    assert_eq!(log.hit(), Some(0));
    assert_eq!(log.top(), 4996);
}

#[test]
fn a_match_already_in_view_keeps_the_view_where_it_is() {
    let palette = palette();
    let mut log = loaded(lines((0..30).map(|index| format!("row {index}"))));
    draw(&mut log, &palette, WIDTH, HEIGHT);
    log.key(press(KeyCode::Down));
    search(&mut log, "row 5");
    assert_eq!(log.hit(), Some(0));
    assert_eq!(log.top(), 1);
}

#[test]
fn escape_while_finding_clears_the_query_and_its_matches() {
    let mut log = loaded(long());
    search(&mut log, "needle");
    assert_eq!(log.hits(), 4);
    assert_eq!(log.key(press(KeyCode::Esc)), Turn::Taken);
    assert!(!log.typing());
    assert_eq!(log.query(), "");
    assert_eq!(log.hits(), 0);
    assert_eq!(log.hit(), None);
    assert_eq!(log.key(typed('n')), Turn::Taken);
    assert_eq!(log.hit(), None);
}

#[test]
fn a_query_with_no_match_finds_nothing_and_next_does_nothing() {
    let mut log = loaded(long());
    search(&mut log, "haystack");
    assert_eq!((log.hits(), log.hit(), log.top()), (0, None, 0));
    log.key(press(KeyCode::Enter));
    log.key(typed('n'));
    log.key(typed('N'));
    assert_eq!((log.hit(), log.top()), (None, 0));
}

#[test]
fn a_paste_lands_in_the_query_only_while_finding() {
    let mut log = loaded(long());
    assert_eq!(log.paste("needle"), Turn::Pass);
    assert_eq!(log.query(), "");
    log.key(typed('/'));
    assert_eq!(log.paste("needle 19"), Turn::Taken);
    assert_eq!(log.query(), "needle 19");
    assert_eq!(log.hits(), 1);
    assert_eq!(log.hit(), Some(0));
}

#[test]
fn new_lines_keep_the_matches_current() {
    let mut log = loaded(long());
    draw(&mut log, &palette(), WIDTH, HEIGHT);
    search(&mut log, "needle");
    log.key(press(KeyCode::Enter));
    log.set_lines(lines(["needle a".to_string(), "b".to_string()]));
    assert_eq!(log.hits(), 1);
    assert_eq!(log.hit(), None);
    assert_eq!(log.top(), 0);
    let mut back = loaded(long());
    draw(&mut back, &palette(), WIDTH, HEIGHT);
    search(&mut back, "needle");
    back.key(press(KeyCode::Enter));
    back.set_lines(long());
    assert_eq!((back.hits(), back.hit()), (4, None));
    assert_eq!(back.key(typed('N')), Turn::Taken);
    assert_eq!(back.hit(), Some(3));
    assert_eq!(back.top(), TOTAL - 9);
}

#[test]
fn copy_takes_the_lines_in_view_and_all_takes_every_line_without_trailing_space() {
    let palette = palette();
    let texts = lines((0..20).map(|index| format!("row {index}   ")));
    let mut log = loaded(texts);
    draw(&mut log, &palette, WIDTH, 5);
    log.key(press(KeyCode::Down));
    log.key(press(KeyCode::Down));
    let Turn::Copy(page) = log.key(typed('y')) else {
        panic!("y copies the page");
    };
    assert_eq!(page, "row 2\nrow 3\nrow 4\nrow 5");
    let Turn::Copy(all) = log.key(typed('Y')) else {
        panic!("Y copies everything");
    };
    assert_eq!(all.lines().count(), 20);
    assert!(all.starts_with("row 0\nrow 1\n") && all.ends_with("row 19"));
    assert!(all.lines().all(|line| !line.ends_with(' ')));
}

#[test]
fn copy_keeps_styled_spans_as_plain_text() {
    let line = Line::from(vec!["a ".into(), "b".into()]);
    let mut log = loaded(Arc::new(vec![line]));
    assert_eq!(log.key(typed('Y')), Turn::Copy("a b".to_string()));
}

#[test]
fn long_lines_are_clipped_at_the_edge_and_never_wrap() {
    let palette = palette();
    let wide = "x".repeat(200);
    let mut log = loaded(lines([wide.clone(), "next".to_string()]));
    let shown = rows(&draw(&mut log, &palette, WIDTH, HEIGHT));
    assert_eq!(shown[0], "x".repeat(usize::from(WIDTH)));
    assert_eq!(shown[1], "next");
    let Turn::Copy(all) = log.key(typed('Y')) else {
        panic!("Y copies everything");
    };
    assert_eq!(all, format!("{wide}\nnext"));
}

#[test]
fn an_empty_log_draws_blank_and_every_key_is_harmless() {
    let palette = palette();
    let mut log = Log::new("Find");
    assert!(log.is_empty());
    let buffer = draw(&mut log, &palette, WIDTH, HEIGHT);
    assert!(dump::text(&buffer).trim().is_empty());
    for code in [
        KeyCode::Down,
        KeyCode::Up,
        KeyCode::PageDown,
        KeyCode::End,
        KeyCode::Home,
    ] {
        assert_eq!(log.key(press(code)), Turn::Taken);
        assert_eq!(log.top(), 0);
    }
    assert_eq!(log.key(typed('n')), Turn::Taken);
    assert_eq!(log.key(typed('y')), Turn::Copy(String::new()));
    assert_eq!(log.key(typed('Y')), Turn::Copy(String::new()));
    search(&mut log, "x");
    assert_eq!(log.hits(), 0);
    assert_eq!(log.key(typed('z')), Turn::Taken);
}

#[test]
fn an_unbound_key_passes_to_the_screen() {
    let mut log = loaded(long());
    assert_eq!(log.key(typed('z')), Turn::Pass);
    assert_eq!(log.key(press(KeyCode::Tab)), Turn::Pass);
}

#[test]
fn the_app_rebinds_the_find_hit_and_copy_keys() {
    use pito_tui::list::Key;
    let mut log = Log::new("Find")
        .search_keys(&[Key::Char('f')])
        .hit_keys(&[Key::Char('>')], &[Key::Char('<')])
        .copy_keys(&[Key::Char('c')], &[Key::Char('C')]);
    log.set_lines(long());
    assert_eq!(log.key(typed('/')), Turn::Pass);
    assert_eq!(log.key(typed('f')), Turn::Taken);
    for c in "needle".chars() {
        log.key(typed(c));
    }
    log.key(press(KeyCode::Enter));
    assert_eq!(log.key(typed('n')), Turn::Pass);
    assert_eq!(log.key(typed('>')), Turn::Taken);
    assert_eq!(log.hit(), Some(1));
    assert_eq!(log.key(typed('<')), Turn::Taken);
    assert_eq!(log.hit(), Some(0));
    assert_eq!(log.key(typed('y')), Turn::Pass);
    assert!(matches!(log.key(typed('c')), Turn::Copy(_)));
    assert!(matches!(log.key(typed('C')), Turn::Copy(_)));
}

#[test]
fn a_following_log_at_the_bottom_stays_at_the_bottom_as_lines_arrive() {
    let palette = palette();
    let grow = |count: usize| lines((0..count).map(|index| format!("row {index}")));
    let mut log = loaded(grow(30));
    draw(&mut log, &palette, WIDTH, HEIGHT);
    log.key(press(KeyCode::End));
    assert_eq!(log.top(), 21);
    log.set_lines(grow(40));
    assert_eq!(log.top(), 31);

    let mut fixed = Log::new("Find").follow(false);
    fixed.set_lines(grow(30));
    draw(&mut fixed, &palette, WIDTH, HEIGHT);
    fixed.key(press(KeyCode::End));
    fixed.set_lines(grow(40));
    assert_eq!(fixed.top(), 21);

    let mut reading = loaded(grow(30));
    draw(&mut reading, &palette, WIDTH, HEIGHT);
    reading.key(press(KeyCode::Down));
    reading.set_lines(grow(40));
    assert_eq!(reading.top(), 1);
    reading.set_lines(grow(3));
    assert_eq!(reading.top(), 0);
}

#[test]
fn a_log_that_starts_short_stays_at_the_top_as_it_grows_past_a_page() {
    let mut log = Log::new("Find");
    draw(&mut log, &palette(), WIDTH, HEIGHT);
    log.set_lines(lines((0..40).map(|index| format!("row {index}"))));
    assert_eq!(log.top(), 0);
    let mut short = loaded(lines((0..3).map(|index| format!("row {index}"))));
    draw(&mut short, &palette(), WIDTH, HEIGHT);
    short.set_lines(lines((0..40).map(|index| format!("row {index}"))));
    assert_eq!(short.top(), 0);
}

#[test]
fn the_same_lines_set_again_change_nothing() {
    let mut log = loaded(long());
    search(&mut log, "needle");
    log.key(press(KeyCode::Enter));
    log.key(typed('n'));
    let same = Arc::clone(log.lines());
    log.set_lines(same);
    assert_eq!(log.hit(), Some(1));
}
