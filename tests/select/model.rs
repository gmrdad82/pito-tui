use pito_tui::crossterm::event::{MouseButton, MouseEventKind};
use pito_tui::ratatui::{
    buffer::Buffer,
    layout::{Position, Rect},
    style::{Color, Modifier, Style},
};
use pito_tui::select::{Fed, Selection};

use crate::common::{down, drag, mouse, reversed, shifted, up};

fn swept(from: (u16, u16), to: (u16, u16)) -> Selection {
    let mut selection = Selection::new();
    assert_eq!(selection.feed(down(from)), Fed::Press);
    assert_eq!(selection.feed(drag(to)), Fed::Drag);
    assert_eq!(selection.feed(up(to)), Fed::Done);
    selection
}

fn lines() -> Buffer {
    Buffer::with_lines(["alpha beta", "gamma delta", "epsilon"])
}

#[test]
fn a_press_and_release_on_one_cell_is_a_click_and_selects_nothing() {
    let mut selection = Selection::new();
    assert_eq!(selection.feed(down((2, 0))), Fed::Press);
    assert!(!selection.shown());
    assert_eq!(selection.feed(up((2, 0))), Fed::Click);
    assert!(!selection.shown());
    assert_eq!(selection.range(), None);
    assert_eq!(selection.text(&lines()), "");
    assert_eq!(selection.take(&lines()), None);
}

#[test]
fn a_drag_that_stays_on_the_pressed_cell_is_still_a_click() {
    let mut selection = Selection::new();
    selection.feed(down((3, 1)));
    assert_eq!(selection.feed(drag((3, 1))), Fed::Pass);
    assert!(!selection.dragging());
    assert_eq!(selection.feed(up((3, 1))), Fed::Click);
    assert_eq!(selection.take(&lines()), None);
}

#[test]
fn a_drag_of_one_cell_starts_a_selection_shown_while_the_button_is_held() {
    let mut selection = Selection::new();
    selection.feed(down((0, 0)));
    assert_eq!(selection.feed(drag((1, 0))), Fed::Drag);
    assert!(selection.dragging() && selection.shown());
    assert_eq!(selection.text(&lines()), "al");
    assert_eq!(selection.feed(drag((4, 0))), Fed::Drag);
    assert_eq!(selection.text(&lines()), "alpha");
    assert_eq!(selection.take(&lines()), None);
    assert_eq!(selection.feed(up((4, 0))), Fed::Done);
    assert!(!selection.dragging() && selection.shown());
}

#[test]
fn a_forward_selection_reads_from_the_anchor_to_the_head() {
    let mut selection = swept((0, 0), (4, 0));
    assert_eq!(selection.text(&lines()), "alpha");
    assert_eq!(selection.take(&lines()).as_deref(), Some("alpha"));
    assert_eq!(selection.take(&lines()), None);
    assert!(selection.shown());
}

#[test]
fn a_backward_selection_reads_the_same_cells_in_reading_order() {
    let mut selection = swept((9, 0), (6, 0));
    assert_eq!(
        selection.range(),
        Some((Position::new(6, 0), Position::new(9, 0)))
    );
    assert_eq!(selection.take(&lines()).as_deref(), Some("beta"));
}

#[test]
fn a_selection_across_rows_streams_whole_middle_rows_either_way() {
    let forward = swept((6, 0), (4, 2));
    assert_eq!(forward.text(&lines()), "beta\ngamma delta\nepsil");
    let backward = swept((4, 2), (6, 0));
    assert_eq!(backward.text(&lines()), "beta\ngamma delta\nepsil");
    assert_eq!(forward.range(), backward.range());
    let rising = swept((2, 1), (8, 0));
    assert_eq!(rising.text(&lines()), "ta\ngam");
}

#[test]
fn a_selection_ending_left_of_its_start_on_the_next_row_keeps_both_ends() {
    let selection = swept((8, 0), (1, 1));
    assert_eq!(selection.text(&lines()), "ta\nga");
}

#[test]
fn wide_glyphs_are_read_once_and_a_half_selected_glyph_counts_whole() {
    let buffer = Buffer::with_lines(["日本語 ok"]);
    assert_eq!(swept((0, 0), (8, 0)).text(&buffer), "日本語 ok");
    assert_eq!(swept((1, 0), (4, 0)).text(&buffer), "日本語");
    assert_eq!(swept((3, 0), (2, 0)).text(&buffer), "本");
    assert_eq!(swept((5, 0), (7, 0)).text(&buffer), "語 o");
}

#[test]
fn zero_width_marks_stay_with_their_letter() {
    let mut buffer = Buffer::empty(Rect::new(0, 0, 6, 1));
    buffer.set_string(0, 0, "e\u{301}xa\u{308}", Style::new());
    assert_eq!(buffer[(0, 0)].symbol(), "e\u{301}");
    assert_eq!(swept((0, 0), (2, 0)).text(&buffer), "e\u{301}xa\u{308}");
    assert_eq!(swept((1, 0), (2, 0)).text(&buffer), "xa\u{308}");
}

#[test]
fn trailing_blanks_of_each_row_are_trimmed_and_inner_ones_kept() {
    let mut buffer = Buffer::empty(Rect::new(0, 0, 12, 3));
    buffer.set_string(0, 0, "ab  cd", Style::new());
    buffer.set_string(2, 2, "ef", Style::new());
    assert_eq!(swept((0, 0), (11, 2)).text(&buffer), "ab  cd\n\n  ef");
    assert_eq!(swept((6, 0), (11, 0)).text(&buffer), "");
    assert_eq!(swept((1, 0), (3, 0)).text(&buffer), "b");
}

#[test]
fn a_selection_past_the_buffer_reads_what_is_there_and_never_panics() {
    let buffer = lines();
    assert_eq!(swept((6, 1), (300, 90)).text(&buffer), "delta\nepsilon");
    assert_eq!(swept((40, 50), (60, 70)).text(&buffer), "");
    let mut painted = buffer.clone();
    swept((40, 50), (60, 70)).paint(&mut painted, Style::new().add_modifier(Modifier::REVERSED));
    assert_eq!(painted, buffer);
}

#[test]
fn a_buffer_away_from_the_origin_is_read_in_terminal_cells() {
    let mut buffer = Buffer::empty(Rect::new(10, 5, 8, 2));
    buffer.set_string(10, 5, "one two", Style::new());
    buffer.set_string(10, 6, "three", Style::new());
    assert_eq!(swept((14, 5), (11, 6)).text(&buffer), "two\nth");
}

#[test]
fn paint_marks_exactly_the_selected_cells_and_both_halves_of_a_wide_glyph() {
    let style = Style::new().add_modifier(Modifier::REVERSED);
    let mut buffer = Buffer::with_lines(["日本 ok", "next row"]);
    swept((1, 0), (2, 1)).paint(&mut buffer, style);
    for x in 0..buffer.area.width {
        assert!(reversed(&buffer, (x, 0)), "row 0 cell {x}");
    }
    for x in 0..buffer.area.width {
        assert_eq!(reversed(&buffer, (x, 1)), x <= 2, "row 1 cell {x}");
    }
    let mut start = Buffer::with_lines(["日本 ok"]);
    swept((3, 0), (5, 0)).paint(&mut start, style);
    let marked: Vec<bool> = (0..7).map(|x| reversed(&start, (x, 0))).collect();
    assert_eq!(marked, [false, false, true, true, true, true, false]);
}

#[test]
fn paint_flips_a_cell_that_was_already_reversed_so_it_still_shows() {
    let style = Style::new().add_modifier(Modifier::REVERSED);
    let mut buffer = Buffer::empty(Rect::new(0, 0, 4, 1));
    buffer.set_string(0, 0, "ab", Style::new().fg(Color::Red));
    buffer.set_string(
        2,
        0,
        "cd",
        Style::new().fg(Color::Red).add_modifier(Modifier::REVERSED),
    );
    swept((0, 0), (3, 0)).paint(&mut buffer, style);
    let marked: Vec<bool> = (0..4).map(|x| reversed(&buffer, (x, 0))).collect();
    assert_eq!(marked, [true, true, false, false]);
    assert!((0..4).all(|x| buffer[(x, 0)].fg == Color::Red));
}

#[test]
fn paint_lays_a_coloured_style_over_the_cells() {
    let mut buffer = lines();
    swept((0, 0), (1, 0)).paint(&mut buffer, Style::new().bg(Color::Indexed(8)));
    assert_eq!(buffer[(0, 0)].bg, Color::Indexed(8));
    assert_eq!(buffer[(1, 0)].bg, Color::Indexed(8));
    assert_eq!(buffer[(2, 0)].bg, Color::Reset);
}

#[test]
fn nothing_is_painted_without_a_selection() {
    let style = Style::new().add_modifier(Modifier::REVERSED);
    let mut buffer = lines();
    Selection::new().paint(&mut buffer, style);
    let mut clicked = Selection::new();
    clicked.feed(down((1, 1)));
    clicked.paint(&mut buffer, style);
    assert_eq!(buffer, lines());
}

#[test]
fn a_shift_press_is_left_to_the_terminal_and_its_drag_selects_nothing() {
    let mut selection = Selection::new();
    let left = MouseEventKind::Down(MouseButton::Left);
    assert_eq!(selection.feed(shifted(left, (0, 0))), Fed::Pass);
    let held = MouseEventKind::Drag(MouseButton::Left);
    assert_eq!(selection.feed(shifted(held, (5, 1))), Fed::Pass);
    let let_go = MouseEventKind::Up(MouseButton::Left);
    assert_eq!(selection.feed(shifted(let_go, (5, 1))), Fed::Pass);
    assert!(!selection.shown());
    assert_eq!(selection.take(&lines()), None);
}

#[test]
fn any_press_clears_a_shown_selection_and_a_right_press_starts_none() {
    let mut selection = swept((0, 0), (4, 0));
    let right = mouse(MouseEventKind::Down(MouseButton::Right), (0, 0));
    assert_eq!(selection.feed(right), Fed::Pass);
    assert!(!selection.shown());
    let mut again = swept((0, 0), (4, 0));
    assert_eq!(again.feed(down((7, 2))), Fed::Press);
    assert!(!again.shown());
    let mut cleared = swept((0, 0), (4, 0));
    cleared.clear();
    assert!(!cleared.shown());
    assert_eq!(cleared.range(), None);
}

#[test]
fn a_release_still_hands_over_its_text_after_a_press_or_clear_before_the_draw() {
    let mut selection = swept((0, 1), (4, 1));
    selection.feed(down((0, 0)));
    selection.clear();
    assert_eq!(selection.take(&lines()).as_deref(), Some("gamma"));
    assert_eq!(selection.take(&lines()), None);
}

#[test]
fn wheel_moves_and_other_buttons_pass_through_untouched() {
    let mut selection = swept((0, 0), (4, 0));
    for kind in [
        MouseEventKind::ScrollDown,
        MouseEventKind::ScrollUp,
        MouseEventKind::Moved,
        MouseEventKind::Drag(MouseButton::Right),
        MouseEventKind::Up(MouseButton::Middle),
        MouseEventKind::Drag(MouseButton::Left),
        MouseEventKind::Up(MouseButton::Left),
    ] {
        assert_eq!(selection.feed(mouse(kind, (2, 2))), Fed::Pass, "{kind:?}");
    }
    assert_eq!(selection.text(&lines()), "alpha");
}
