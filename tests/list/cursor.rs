use crate::common::*;
use pito_tui::list::{Cell, Column, List, ListView, Mark, Row, Styles};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

const BOLD: Style = Style::new().add_modifier(Modifier::BOLD);
const RED: Style = Style::new().fg(Color::Red);
const CARET: Style = Style::new().fg(Color::Yellow);
const LIT: Style = Style::new().bg(Color::Blue).add_modifier(Modifier::BOLD);

const TWO: [Column<'static>; 2] = [Column::new("A", 5, 5), Column::new("B", 3, 0)];

fn list() -> List {
    List::new().with_rows([
        Row::new([Cell::new("hot").style(RED), Cell::new("x")]).mark(Mark::new("✓").style(GREEN)),
        Row::new([Cell::new("cold").style(RED), Cell::new("y")]).mark(Mark::new("✓").style(GREEN)),
        Row::new(["warm", "z"]),
    ])
}

fn render(list: &mut List, styles: Styles, cursor: Option<&'static str>) -> Buffer {
    let area = Rect::new(0, 0, 16, 4);
    let mut buf = Buffer::empty(area);
    let mut view = ListView::new(list, &TWO).styles(styles);
    if let Some(text) = cursor {
        view = view.cursor(text);
    }
    view.render(area, &mut buf);
    buf
}

#[test]
fn a_cursor_style_colours_the_marker_under_a_bold_selection_that_keeps_colours() {
    let mut list = list();
    let buf = render(
        &mut list,
        STYLES.selected(BOLD).keep_colours(true).cursor(CARET),
        None,
    );
    assert_eq!(read(&buf).text[0], "▌ ✓ hot    x");
    for x in 0..2 {
        assert_eq!(buf[(x, 0)].fg, Color::Yellow, "{x}");
        assert!(buf[(x, 0)].modifier.contains(Modifier::BOLD), "{x}");
    }
    assert_eq!(buf[(2, 0)].fg, Color::Green);
    assert_eq!(buf[(4, 0)].fg, Color::Red);
    assert!(buf[(4, 0)].modifier.contains(Modifier::BOLD));
    assert_eq!(buf[(11, 0)].fg, Color::Reset);
    assert_eq!(buf[(0, 1)].symbol(), " ");
    assert_eq!(buf[(0, 1)].fg, Color::Reset);
}

#[test]
fn the_cursor_style_draws_over_the_selected_line() {
    let mut list = list();
    let buf = render(&mut list, STYLES.selected(LIT).cursor(CARET), None);
    for x in 0..2 {
        assert_eq!(buf[(x, 0)].fg, Color::Yellow, "{x}");
        assert_eq!(buf[(x, 0)].bg, Color::Blue, "{x}");
        assert!(buf[(x, 0)].modifier.contains(Modifier::BOLD), "{x}");
    }
    for x in 2..16 {
        assert_eq!(buf[(x, 0)].fg, Color::Reset, "{x}");
        assert_eq!(buf[(x, 0)].bg, Color::Blue, "{x}");
    }
    let own = CARET.bg(Color::Black).remove_modifier(Modifier::BOLD);
    let page = Style::new().bg(Color::DarkGray);
    let buf = render(&mut list, STYLES.selected(LIT).base(page).cursor(own), None);
    for x in 0..2 {
        assert_eq!(buf[(x, 0)].fg, Color::Yellow, "{x}");
        assert_eq!(buf[(x, 0)].bg, Color::Black, "{x}");
        assert!(!buf[(x, 0)].modifier.contains(Modifier::BOLD), "{x}");
    }
    assert_eq!(buf[(2, 0)].bg, Color::Blue);
    assert_eq!(buf[(0, 1)].bg, Color::DarkGray);
}

#[test]
fn unset_the_marker_draws_in_the_selected_style_as_before() {
    let mut list = list();
    let plain = render(&mut list, STYLES, None);
    assert_eq!(read(&plain).marks[0], "S".repeat(16));
    assert_eq!(plain[(0, 0)].fg, Color::Magenta);
    assert_eq!(render(&mut list, STYLES.cursor(SELECTED), None), plain);
    let kept = STYLES.selected(BOLD).keep_colours(true);
    assert_eq!(
        render(&mut list, kept.cursor(BOLD), None),
        render(&mut list, kept, None)
    );
    assert_ne!(Styles::new().cursor(CARET), Styles::new());
    assert_eq!(Styles::default(), Styles::new());
}

#[test]
fn the_cursor_style_takes_a_replaced_marker_and_stays_off_the_range() {
    let mut list = list();
    list.select(2);
    list.select_range(Some((0, 2)));
    let buf = render(&mut list, STYLES.selected(LIT).cursor(CARET), Some("> "));
    let drawn = read(&buf);
    assert_eq!(drawn.text[0], "  ✓ hot    x");
    assert_eq!(drawn.text[2], ">   warm   z");
    assert_eq!(buf[(0, 2)].fg, Color::Yellow);
    assert_eq!(buf[(0, 2)].bg, Color::Blue);
    for y in 0..2 {
        assert_eq!(buf[(0, y)].fg, Color::Reset, "{y}");
        assert_eq!(buf[(0, y)].bg, Color::Blue, "{y}");
    }
    let bare = render(&mut list, STYLES.selected(LIT).cursor(CARET), Some(""));
    assert_eq!(read(&bare).text[2], "  warm   z");
    assert_eq!(bare[(0, 2)].fg, Color::Reset);
}
