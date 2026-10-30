use std::time::Duration;

use pito_tui::Flow;
use pito_tui::crossterm::event::KeyCode;
use pito_tui::footer::Hint;
use pito_tui::keys::Words;

use crate::common::{BIG, KEYS, WIDE, bare, ch, code, ctrl, heard, keyed, started, text, typed};

const SMALL: (u16, u16) = (80, 24);

fn boxed(shown: &str) -> usize {
    let lines: Vec<&str> = shown.lines().collect();
    let top = lines.iter().position(|line| line.contains('╭'));
    let bottom = lines.iter().position(|line| line.contains('╰'));
    match (top, bottom) {
        (Some(top), Some(bottom)) => bottom + 1 - top,
        _ => 0,
    }
}

fn inside(shown: &str) -> String {
    let lines = shown.lines().filter(|line| line.contains('│'));
    lines.collect::<Vec<_>>().join("\n")
}

#[test]
fn question_mark_opens_the_sheet_only_when_a_keymap_is_set() {
    let title = Words::default().title;
    let mut without = started(bare().hints([Hint::new("x", "marks")]));
    assert!(text(&mut without, WIDE).contains("x marks"));
    without.key(ch('?'));
    let shown = text(&mut without, WIDE);
    assert!(!without.sheet().is_open());
    assert!(!shown.contains("x marks"), "{shown}");
    assert!(!shown.contains(title.as_ref()), "{shown}");

    let mut with = started(keyed(&KEYS));
    with.key(ch('?'));
    assert!(with.sheet().is_open());
    let shown = text(&mut with, WIDE);
    for part in [title.as_ref(), "clips", "Rail", "keeps", "Elsewhere", "Cut"] {
        assert!(shown.contains(part), "{part}:\n{shown}");
    }
    assert!(shown.contains("ctrl+z") && shown.contains("as often as you like"));
    assert!(heard(&with).is_empty());
}

#[test]
fn the_sheet_puts_the_current_mode_first() {
    let mut tui = started(keyed(&KEYS));
    tui.key(ch('?'));
    let shown = text(&mut tui, WIDE);
    let at = |part: &str| shown.find(part).unwrap_or(usize::MAX);
    assert!(at("keeps") < at("Elsewhere"), "{shown}");
    assert!(at("Elsewhere") < at("left and right"), "{shown}");
    assert!(shown.contains("choose · cut"), "{shown}");
}

#[test]
fn the_sheet_searches_and_its_keys_close_it() {
    let mut tui = started(keyed(&KEYS));
    tui.key(ch('?'));
    tui.key(ch('/'));
    typed(&mut tui, "undo");
    assert!(tui.sheet().typing());
    assert_eq!(tui.sheet().query(), "undo");
    let shown = inside(&text(&mut tui, WIDE));
    assert!(shown.contains("/ undo"), "{shown}");
    assert!(
        shown.contains("undoes") && !shown.contains("keeps"),
        "{shown}"
    );

    tui.key(code(KeyCode::Enter));
    assert!(!tui.sheet().typing() && tui.sheet().query() == "undo");
    tui.key(code(KeyCode::Esc));
    assert!(tui.sheet().is_open() && tui.sheet().query().is_empty());
    tui.key(code(KeyCode::Esc));
    assert!(!tui.sheet().is_open());
    assert!(!text(&mut tui, WIDE).contains("Elsewhere"));

    tui.key(ch('?'));
    typed(&mut tui, "/nothing answers");
    assert!(text(&mut tui, WIDE).contains(Words::default().nothing.as_ref()));
    tui.key(code(KeyCode::Esc));
    tui.key(ch('?'));
    assert!(!tui.sheet().is_open());
    assert!(heard(&tui).is_empty());
}

#[test]
fn keys_the_sheet_passes_on_reach_the_screen_and_it_stays_open() {
    let mut tui = started(keyed(&KEYS));
    tui.key(ch('?'));
    tui.key(code(KeyCode::Right));
    tui.key(code(KeyCode::Tab));
    assert_eq!(heard(&tui), [KeyCode::Right, KeyCode::Tab]);
    assert!(tui.sheet().is_open());
    let shown = text(&mut tui, WIDE);
    let at = |part: &str| shown.find(part).unwrap_or(usize::MAX);
    assert!(at("left and right") < at("Elsewhere"), "{shown}");
}

#[test]
fn ctrl_c_quits_while_the_sheet_is_open_and_typing() {
    let mut tui = started(keyed(&KEYS));
    tui.key(ch('?'));
    tui.key(ch('/'));
    assert_eq!(tui.key(ctrl('c')), Flow::Stay);
    assert_eq!(tui.key(ctrl('c')), Flow::Quit);
}

#[test]
fn the_sheet_scrolls_a_page_at_a_time_when_it_outgrows_the_area() {
    let mut tui = started(keyed(&BIG));
    tui.key(ch('?'));
    let first = text(&mut tui, SMALL);
    assert!(first.contains("does 00") && first.contains('↓'), "{first}");
    assert!(
        !first.contains("does 59") && !first.contains('↑'),
        "{first}"
    );

    for _ in 0..20 {
        tui.key(code(KeyCode::PageDown));
    }
    let last = text(&mut tui, SMALL);
    assert!(last.contains("does 59") && last.contains('↑'), "{last}");
    assert!(!last.contains("does 00") && !last.contains('↓'), "{last}");

    tui.key(code(KeyCode::PageUp));
    let back = text(&mut tui, SMALL);
    assert!(back.contains('↑') && back.contains('↓'), "{back}");
    assert!(heard(&tui).is_empty());
}

#[test]
fn the_sheet_eases_open_and_closed_and_is_instant_when_still() {
    let mut tui = started(keyed(&KEYS).still(false));
    tui.key(ch('?'));
    assert_eq!(boxed(&text(&mut tui, WIDE)), 0);
    tui.advance(Duration::from_millis(90));
    let half = boxed(&text(&mut tui, WIDE));
    tui.advance(Duration::from_millis(200));
    let full = boxed(&text(&mut tui, WIDE));
    assert!(0 < half && half < full, "{half} {full}");

    tui.key(ch('?'));
    assert_eq!(boxed(&text(&mut tui, WIDE)), full);
    tui.advance(Duration::from_millis(90));
    let closing = boxed(&text(&mut tui, WIDE));
    assert!(0 < closing && closing < full, "{closing} {full}");
    tui.advance(Duration::from_millis(200));
    assert_eq!(boxed(&text(&mut tui, WIDE)), 0);

    let mut still = started(keyed(&KEYS));
    still.key(ch('?'));
    assert_eq!(boxed(&text(&mut still, WIDE)), full);
    still.key(ch('?'));
    assert_eq!(boxed(&text(&mut still, WIDE)), 0);
}
