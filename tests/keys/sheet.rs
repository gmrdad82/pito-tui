use std::time::Duration;

use pito_tui::crossterm::event::{KeyCode, KeyModifiers, MouseButton, MouseEvent, MouseEventKind};
use pito_tui::footer::Hint;
use pito_tui::keys::Words;
use pito_tui::{Flow, Tui};

use crate::common::{
    BIG, KEYS, Probe, WIDE, bare, ch, code, ctrl, heard, keyed, started, text, typed,
};

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
fn the_sheets_own_keys_are_on_screen_on_its_first_page_at_80x24() {
    let words = Words::default();
    let mut tui = started(keyed(&BIG));
    tui.key(ch('?'));
    let first = text(&mut tui, SMALL);
    assert!(first.contains('↓') && !first.contains("does 59"), "{first}");
    for (keys, does) in [
        ("esc", &words.close),
        ("/", &words.search),
        ("pgup / pgdn", &words.scroll),
    ] {
        let hint = format!("{keys} {does}");
        assert!(first.contains(&hint), "{hint}:\n{first}");
    }

    tui.key(ch('/'));
    let typing = text(&mut tui, SMALL);
    for (keys, does) in [
        ("esc", &words.clear),
        ("enter", &words.keep),
        ("pgup / pgdn", &words.scroll),
    ] {
        let hint = format!("{keys} {does}");
        assert!(typing.contains(&hint), "{hint}:\n{typing}");
    }
}

#[test]
fn a_layer_opened_under_an_eased_sheet_shows_at_once() {
    let mut tui = started(keyed(&KEYS).still_sheet(false));
    tui.key(ch('?'));
    tui.advance(Duration::from_millis(400));
    assert!(text(&mut tui, WIDE).contains("Elsewhere"));
    tui.key(ch('m'));
    assert!(!tui.sheet().is_open());
    let shown = text(&mut tui, WIDE);
    assert!(!shown.contains("Elsewhere"), "{shown}");
}

#[test]
fn the_sheet_eases_open_and_closed_and_is_instant_when_still() {
    let mut tui = started(keyed(&KEYS).still_sheet(false));
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

#[test]
fn a_key_the_sheet_passes_on_that_opens_a_layer_closes_the_sheet() {
    for key in ['m', 'p', 'i'] {
        let mut tui = started(keyed(&KEYS));
        tui.key(ch('?'));
        assert!(tui.sheet().is_open(), "{key}");
        tui.key(ch(key));
        assert!(!tui.sheet().is_open(), "{key}");
        assert!(!text(&mut tui, WIDE).contains("Elsewhere"), "{key}");
        tui.key(ch('?'));
        assert!(!tui.sheet().is_open(), "{key}");
        assert!(!text(&mut tui, WIDE).contains("Elsewhere"), "{key}");
    }
}

#[test]
fn question_mark_typed_into_the_screens_input_stays_text() {
    let mut tui = started(keyed(&KEYS));
    tui.key(ch('i'));
    tui.key(ch('?'));
    tui.key(ch('/'));
    assert!(!tui.sheet().is_open());
    assert_eq!(
        heard(&tui),
        [KeyCode::Char('i'), KeyCode::Char('?'), KeyCode::Char('/')]
    );
}

#[test]
fn question_mark_does_nothing_while_the_shell_is_gated() {
    let mut tui = started(keyed(&KEYS).gate(Probe::default(), true));
    tui.key(ch('?'));
    assert!(!tui.sheet().is_open());
    assert!(!text(&mut tui, WIDE).contains("Elsewhere"));
}

#[test]
fn a_paste_goes_to_the_query_while_searching_and_nowhere_else_while_open() {
    let mut tui = started(keyed(&KEYS));
    tui.key(ch('?'));
    tui.paste("ignored");
    assert!(tui.sheet().query().is_empty());
    tui.key(ch('/'));
    tui.paste("un\ndo");
    assert_eq!(tui.sheet().query(), "un do");
    assert!(tui.sheet().typing());
    tui.key(code(KeyCode::Esc));
    tui.key(code(KeyCode::Esc));
    assert!(!tui.sheet().is_open());
    tui.paste("body");
    let pastes = tui.find::<Probe>().map(|probe| probe.pastes.clone());
    assert_eq!(pastes.unwrap_or_default(), ["body"]);
}

#[test]
fn a_screen_toggles_the_sheet_through_its_context() {
    let mut tui = started(keyed(&KEYS));
    tui.key(ch('h'));
    assert!(tui.sheet().is_open());
    assert!(text(&mut tui, WIDE).contains("Elsewhere"));

    let mut without = started(bare());
    without.key(ch('h'));
    assert!(!without.sheet().is_open());
}

fn mice(tui: &Tui<()>) -> usize {
    tui.find::<Probe>().map_or(0, |probe| probe.mice)
}

#[test]
fn clicks_and_wheels_under_the_open_sheet_reach_nothing() {
    let mut tui = started(keyed(&KEYS));
    let at = |kind| MouseEvent {
        kind,
        column: WIDE.0 / 2,
        row: WIDE.1 / 2,
        modifiers: KeyModifiers::NONE,
    };
    let click = at(MouseEventKind::Down(MouseButton::Left));
    let wheel = at(MouseEventKind::ScrollDown);
    text(&mut tui, WIDE);
    tui.key(ch('?'));
    assert!(text(&mut tui, WIDE).contains("Elsewhere"));
    assert_eq!(tui.mouse(click), Flow::Stay);
    assert_eq!(tui.mouse(wheel), Flow::Stay);
    assert_eq!(mice(&tui), 0);
    assert!(tui.sheet().is_open());

    tui.key(ch('?'));
    tui.mouse(click);
    tui.mouse(wheel);
    assert_eq!(mice(&tui), 2);
}
