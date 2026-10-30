use crate::common::*;
use pito_tui::list::{Cell, Column, List, ListView, Mark, Part, Row, Styles};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

const RED: Style = Style::new().fg(Color::Red);
const LIT: Style = Style::new().bg(Color::Blue).add_modifier(Modifier::BOLD);

const THREE: [Column<'static>; 3] = [
    Column::new("A", 5, 5),
    Column::new("B", 5, 5),
    Column::new("C", 3, 0),
];

fn hot(name: &str) -> Row {
    Row::new([
        Cell::new(name).style(RED),
        Cell::parts([Part::new("ab").style(GREEN), Part::new("cd")]),
        Cell::new("dim").faint(),
    ])
    .mark(Mark::new("✓").style(GREEN))
}

fn render(list: &mut List, styles: Styles) -> Buffer {
    let area = Rect::new(0, 0, 24, 4);
    let mut buf = Buffer::empty(area);
    ListView::new(list, &THREE)
        .styles(styles)
        .render(area, &mut buf);
    buf
}

fn three() -> List {
    List::new().with_rows([hot("hot"), hot("warm"), hot("cool")])
}

#[test]
fn kept_colours_patch_the_selected_style_over_each_cell_part_and_mark() {
    let mut list = three();
    let buf = render(&mut list, STYLES.selected(LIT).keep_colours(true));
    assert_eq!(read(&buf).text[0], "▌ ✓ hot    abcd   dim");
    let at = |x: u16| &buf[(x, 0)];
    for x in 0..24 {
        assert_eq!(at(x).bg, Color::Blue, "{x}");
        assert!(at(x).modifier.contains(Modifier::BOLD), "{x}");
    }
    assert_eq!(at(0).fg, Color::Reset);
    assert_eq!(at(2).fg, Color::Green);
    assert_eq!(at(4).fg, Color::Red);
    assert_eq!(at(6).fg, Color::Red);
    assert_eq!(at(11).fg, Color::Green);
    assert_eq!(at(12).fg, Color::Green);
    assert_eq!(at(13).fg, Color::Reset);
    assert_eq!(at(18).fg, Color::Reset);
    assert!(at(18).modifier.contains(Modifier::DIM));
    assert!(!at(4).modifier.contains(Modifier::DIM));
    assert_eq!(buf[(4, 1)].bg, Color::Reset);
    assert!(!buf[(4, 1)].modifier.contains(Modifier::BOLD));
}

#[test]
fn replacing_stays_the_default() {
    let mut list = three();
    let buf = render(&mut list, STYLES.selected(LIT));
    for x in 0..24 {
        assert_eq!(buf[(x, 0)].fg, Color::Reset, "{x}");
        assert_eq!(buf[(x, 0)].bg, Color::Blue, "{x}");
        assert!(!buf[(x, 0)].modifier.contains(Modifier::DIM), "{x}");
    }
    assert_eq!(
        render(&mut list, STYLES.selected(LIT).keep_colours(false)),
        buf
    );
    assert_eq!(Styles::new().keep_colours(false), Styles::new());
}

#[test]
fn a_selected_foreground_still_wins_over_the_cells() {
    let mut list = three();
    let buf = render(
        &mut list,
        STYLES.selected(LIT.fg(Color::Yellow)).keep_colours(true),
    );
    for x in 0..24 {
        assert_eq!(buf[(x, 0)].fg, Color::Yellow, "{x}");
    }
    assert!(buf[(18, 0)].modifier.contains(Modifier::DIM));
}

#[test]
fn a_cells_own_background_shows_when_the_selected_style_has_none() {
    let mut list = List::new().with_rows([Row::new([
        Cell::new("tag").style(Style::new().fg(Color::Black).bg(Color::Green)),
        Cell::new("x"),
        Cell::new("y"),
    ])]);
    let bold = Style::new().add_modifier(Modifier::BOLD);
    let buf = render(&mut list, STYLES.selected(bold).keep_colours(true));
    for x in 2..5 {
        assert_eq!(buf[(x, 0)].bg, Color::Green, "{x}");
        assert_eq!(buf[(x, 0)].fg, Color::Black, "{x}");
        assert!(buf[(x, 0)].modifier.contains(Modifier::BOLD), "{x}");
    }
    assert_eq!(buf[(5, 0)].bg, Color::Reset);
    let buf = render(&mut list, STYLES.selected(LIT).keep_colours(true));
    assert_eq!(buf[(2, 0)].bg, Color::Blue);
    assert_eq!(buf[(2, 0)].fg, Color::Black);
}

#[test]
fn kept_colours_apply_to_every_row_of_a_range() {
    let mut list = three();
    list.select(2);
    list.select_range(Some((1, 2)));
    let buf = render(&mut list, STYLES.selected(LIT).keep_colours(true));
    let drawn = read(&buf);
    assert_eq!(drawn.text[1], "  ✓ warm   abcd   dim");
    assert_eq!(drawn.text[2], "▌ ✓ cool   abcd   dim");
    for y in 1..3 {
        assert_eq!(buf[(0, y)].bg, Color::Blue, "{y}");
        assert_eq!(buf[(2, y)].fg, Color::Green, "{y}");
        assert_eq!(buf[(4, y)].fg, Color::Red, "{y}");
        assert_eq!(buf[(11, y)].fg, Color::Green, "{y}");
        assert!(buf[(4, y)].modifier.contains(Modifier::BOLD), "{y}");
    }
    assert_eq!(buf[(4, 0)].fg, Color::Red);
    assert_eq!(buf[(4, 0)].bg, Color::Reset);
}

#[test]
fn kept_colours_draw_over_the_base() {
    let mut list = three();
    let page = Style::new().fg(Color::White).bg(Color::Black);
    let buf = render(
        &mut list,
        STYLES
            .selected(Style::new().add_modifier(Modifier::BOLD))
            .base(page)
            .keep_colours(true),
    );
    for x in 0..24 {
        assert_eq!(buf[(x, 0)].bg, Color::Black, "{x}");
        assert!(buf[(x, 0)].modifier.contains(Modifier::BOLD), "{x}");
    }
    assert_eq!(buf[(4, 0)].fg, Color::Red);
    assert_eq!(buf[(9, 0)].fg, Color::White);
    assert_eq!(buf[(13, 0)].fg, Color::White);
}
