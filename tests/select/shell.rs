use pito_tui::crossterm::event::{MouseButton, MouseEventKind};
use pito_tui::{COPY_MAX, Words, dump};

use crate::common::{
    Probe, SIZE, Sent, Sink, app, ch, down, drag, ends, heard, keyed, mouse, plain, reversed,
    shifted, spot, sweep, up,
};

const DOWN: MouseEventKind = MouseEventKind::Down(MouseButton::Left);
const DRAG: MouseEventKind = MouseEventKind::Drag(MouseButton::Left);
const UP: MouseEventKind = MouseEventKind::Up(MouseButton::Left);

fn copied(text: &str) -> Vec<Sent> {
    vec![
        Sent::Osc(format!(
            "\x1b]52;c;{}\x07",
            pito_tui::base64(text.as_bytes())
        )),
        Sent::Command(text.to_string()),
    ]
}

#[test]
fn a_drag_over_the_screen_is_highlighted_while_held_and_copied_on_release() {
    let sink = Sink::default();
    let mut tui = plain(&sink);
    let before = tui.frame(SIZE.0, SIZE.1);
    let (from, to) = ends(&before, "alpha beta");
    assert!(!reversed(&before, from));
    tui.mouse(down(from));
    tui.mouse(drag(to));
    let held = tui.frame(SIZE.0, SIZE.1);
    assert!(sink.sent().is_empty());
    for x in from.0..=to.0 {
        assert!(reversed(&held, (x, from.1)), "cell {x}");
    }
    assert!(!reversed(&held, (from.0 - 1, from.1)));
    assert!(!reversed(&held, (to.0 + 1, to.1)));
    tui.mouse(up(to));
    let after = tui.frame(SIZE.0, SIZE.1);
    assert_eq!(sink.sent(), copied("alpha beta"));
    assert_eq!(after, held);
    assert_eq!(heard(&tui), [DOWN]);
    tui.frame(SIZE.0, SIZE.1);
    assert_eq!(sink.sent(), copied("alpha beta"));
}

#[test]
fn a_drag_across_rows_copies_them_joined_by_new_lines() {
    let sink = Sink::default();
    let mut tui = plain(&sink);
    let before = tui.frame(SIZE.0, SIZE.1);
    let (beta, _) = ends(&before, "beta");
    let (start, _) = ends(&before, "epsilon");
    sweep(&mut tui, beta, (start.0 + 2, start.1));
    tui.frame(SIZE.0, SIZE.1);
    assert_eq!(sink.sent(), copied("beta\ngamma delta\neps"));
}

#[test]
fn the_highlight_shows_in_a_dump_as_reversed_cells() {
    let sink = Sink::default();
    let mut tui = plain(&sink);
    let before = tui.frame(SIZE.0, SIZE.1);
    let (from, to) = ends(&before, "alpha beta");
    sweep(&mut tui, from, to);
    let after = tui.frame(SIZE.0, SIZE.1);
    let ansi = dump::ansi(&after);
    assert!(ansi.contains("\x1b[0;7malpha beta\x1b[0m"), "{ansi:?}");
    assert!(!dump::ansi(&before).contains("\x1b[0;7malpha"));
    assert_eq!(dump::text(&after), dump::text(&before));
}

#[test]
fn a_click_reaches_the_screen_and_selects_nothing() {
    let sink = Sink::default();
    let mut tui = plain(&sink);
    let before = tui.frame(SIZE.0, SIZE.1);
    let (from, _) = ends(&before, "gamma");
    tui.mouse(down(from));
    tui.mouse(up(from));
    assert_eq!(tui.frame(SIZE.0, SIZE.1), before);
    assert!(sink.sent().is_empty());
    assert_eq!(heard(&tui), [DOWN, UP]);
    assert!(!tui.selection().shown());
}

#[test]
fn the_header_is_selectable_like_any_drawn_text() {
    let sink = Sink::default();
    let mut tui = plain(&sink);
    let before = tui.frame(SIZE.0, SIZE.1);
    let (from, to) = ends(&before, "probe");
    sweep(&mut tui, from, to);
    let after = tui.frame(SIZE.0, SIZE.1);
    assert!(reversed(&after, from) && reversed(&after, to));
    assert_eq!(sink.sent(), copied("probe"));
}

#[test]
fn a_modal_is_selectable_and_its_screen_hears_no_mouse() {
    let sink = Sink::default();
    let mut tui = plain(&sink);
    tui.key(ch('m'));
    let before = tui.frame(SIZE.0, SIZE.1);
    let (from, to) = ends(&before, "modal body text");
    sweep(&mut tui, from, to);
    let after = tui.frame(SIZE.0, SIZE.1);
    assert!(reversed(&after, from));
    assert_eq!(sink.sent(), copied("modal body text"));
    assert!(heard(&tui).is_empty());
}

#[test]
fn the_help_sheet_is_selectable() {
    let sink = Sink::default();
    let mut tui = keyed(&sink);
    tui.key(ch('?'));
    let before = tui.frame(SIZE.0, SIZE.1);
    let (from, to) = ends(&before, "sweeps the floor");
    sweep(&mut tui, from, to);
    let after = tui.frame(SIZE.0, SIZE.1);
    assert!(reversed(&after, to));
    assert_eq!(sink.sent(), copied("sweeps the floor"));
    assert!(heard(&tui).is_empty());
}

#[test]
fn the_apps_words_for_a_copy_show_as_a_toast_on_the_next_frame() {
    let sink = Sink::default();
    let mut tui = app(Probe::default(), &sink, Words::new().copied("Copied it"));
    let before = tui.frame(SIZE.0, SIZE.1);
    let (from, to) = ends(&before, "alpha");
    sweep(&mut tui, from, to);
    let released = dump::text(&tui.frame(SIZE.0, SIZE.1));
    assert!(!released.contains("Copied it"), "{released}");
    let next = dump::text(&tui.frame(SIZE.0, SIZE.1));
    assert!(next.contains("Copied it"), "{next}");
    assert_eq!(sink.sent(), copied("alpha"));
}

#[test]
fn without_words_a_copy_shows_no_toast() {
    let sink = Sink::default();
    let mut tui = plain(&sink);
    let before = tui.frame(SIZE.0, SIZE.1);
    let (from, to) = ends(&before, "alpha");
    sweep(&mut tui, from, to);
    let released = tui.frame(SIZE.0, SIZE.1);
    assert_eq!(tui.frame(SIZE.0, SIZE.1), released);
    assert_eq!(sink.sent(), copied("alpha"));
}

#[test]
fn a_selection_of_blank_cells_copies_nothing_and_says_nothing() {
    let sink = Sink::default();
    let mut tui = app(Probe::default(), &sink, Words::new().copied("Copied it"));
    let before = tui.frame(SIZE.0, SIZE.1);
    let (_, row) = spot(&before, "alpha beta");
    sweep(&mut tui, (40, row), (50, row));
    let released = tui.frame(SIZE.0, SIZE.1);
    assert!(reversed(&released, (45, row)));
    let next = dump::text(&tui.frame(SIZE.0, SIZE.1));
    assert!(!next.contains("Copied it"), "{next}");
    assert!(sink.sent().is_empty());
}

#[test]
fn a_selection_over_copy_max_reaches_only_the_clipboard_command() {
    let sink = Sink::default();
    let probe = Probe {
        fill: true,
        ..Probe::default()
    };
    let mut tui = app(probe, &sink, Words::new());
    let (width, height) = (500, 120);
    tui.frame(width, height);
    sweep(&mut tui, (0, 0), (width - 1, height - 1));
    tui.frame(width, height);
    let sent = sink.sent();
    assert_eq!(sent.len(), 1, "{:?}", sent.len());
    let Sent::Command(text) = &sent[0] else {
        panic!("{sent:?}");
    };
    assert!(text.len() > COPY_MAX, "{}", text.len());
    assert!(text.contains(&"x".repeat(400)));
    assert_eq!(text.split('\n').count(), usize::from(height));
}

#[test]
fn select_false_leaves_every_drag_to_the_screen() {
    let sink = Sink::default();
    let mut tui = plain(&sink).select(false);
    let before = tui.frame(SIZE.0, SIZE.1);
    let (from, to) = ends(&before, "alpha beta");
    sweep(&mut tui, from, to);
    assert_eq!(tui.frame(SIZE.0, SIZE.1), before);
    assert!(sink.sent().is_empty());
    assert_eq!(heard(&tui), [DOWN, DRAG, UP]);
    assert!(!tui.selection().shown());
}

#[test]
fn a_screen_that_drags_keeps_its_drags_and_the_chrome_stays_selectable() {
    let sink = Sink::default();
    let probe = Probe {
        drags: true,
        ..Probe::default()
    };
    let mut tui = app(probe, &sink, Words::new());
    let before = tui.frame(SIZE.0, SIZE.1);
    let (from, to) = ends(&before, "alpha beta");
    sweep(&mut tui, from, to);
    assert_eq!(tui.frame(SIZE.0, SIZE.1), before);
    assert!(sink.sent().is_empty());
    assert_eq!(heard(&tui), [DOWN, DRAG, UP]);
    let (from, to) = ends(&before, "probe");
    sweep(&mut tui, from, to);
    tui.frame(SIZE.0, SIZE.1);
    assert_eq!(sink.sent(), copied("probe"));
    assert_eq!(heard(&tui), [DOWN, DRAG, UP]);
}

#[test]
fn a_shift_drag_is_left_alone_and_reaches_the_screen() {
    let sink = Sink::default();
    let mut tui = plain(&sink);
    let before = tui.frame(SIZE.0, SIZE.1);
    let (from, to) = ends(&before, "alpha beta");
    tui.mouse(shifted(DOWN, from));
    tui.mouse(shifted(DRAG, to));
    tui.mouse(shifted(UP, to));
    assert_eq!(tui.frame(SIZE.0, SIZE.1), before);
    assert!(sink.sent().is_empty());
    assert_eq!(heard(&tui), [DOWN, DRAG, UP]);
}

#[test]
fn the_next_press_clears_the_selection() {
    let sink = Sink::default();
    let mut tui = plain(&sink);
    let before = tui.frame(SIZE.0, SIZE.1);
    let (from, to) = ends(&before, "alpha beta");
    sweep(&mut tui, from, to);
    assert!(reversed(&tui.frame(SIZE.0, SIZE.1), from));
    let (gamma, _) = ends(&before, "gamma");
    tui.mouse(down(gamma));
    let pressed = tui.frame(SIZE.0, SIZE.1);
    assert!(!reversed(&pressed, from) && !reversed(&pressed, gamma));
    assert!(!tui.selection().shown());
    sweep(&mut tui, from, to);
    tui.mouse(mouse(MouseEventKind::Down(MouseButton::Right), gamma));
    assert!(!reversed(&tui.frame(SIZE.0, SIZE.1), from));
}

#[test]
fn the_next_key_clears_the_selection() {
    let sink = Sink::default();
    let mut tui = plain(&sink);
    let before = tui.frame(SIZE.0, SIZE.1);
    let (from, to) = ends(&before, "alpha beta");
    sweep(&mut tui, from, to);
    assert!(reversed(&tui.frame(SIZE.0, SIZE.1), from));
    tui.key(ch('z'));
    assert_eq!(tui.frame(SIZE.0, SIZE.1), before);
    assert!(!tui.selection().shown());
    assert_eq!(sink.sent(), copied("alpha beta"));
}

#[test]
fn a_wheel_turn_still_reaches_the_screen_while_a_selection_shows() {
    let sink = Sink::default();
    let mut tui = plain(&sink);
    let before = tui.frame(SIZE.0, SIZE.1);
    let (from, to) = ends(&before, "alpha beta");
    sweep(&mut tui, from, to);
    tui.mouse(mouse(MouseEventKind::ScrollDown, from));
    assert_eq!(heard(&tui), [DOWN, MouseEventKind::ScrollDown]);
    assert!(tui.selection().shown());
}
