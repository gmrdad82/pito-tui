use crate::common::*;
use pito_tui::list::{Cell, Column, List, ListView, Mark, Part, Row, Styles};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

const TWO: [Column<'static>; 2] = [Column::new("", 3, 3), Column::new("", 1, 0)];
const INK: Style = Style::new().fg(Color::Gray);
const RED: Style = Style::new().fg(Color::Red);
const YELLOW: Style = Style::new().fg(Color::Yellow);
const LIT: Style = Style::new().bg(Color::Blue).add_modifier(Modifier::BOLD);

fn render(list: &mut List, styles: Styles) -> Buffer {
    let area = Rect::new(0, 0, 16, 3);
    let mut buf = Buffer::empty(area);
    ListView::new(list, &TWO)
        .styles(styles)
        .render(area, &mut buf);
    buf
}

fn counts() -> List {
    List::new().with_rows([
        Row::new(["a", "1"]).mark(Mark::new("⧗")),
        Row::new([Cell::new("b"), Cell::new("2").faint()]).mark(Mark::new("⧗").faint()),
        Row::new(["c", "300"]).mark(Mark::new("⧗")),
    ])
}

fn warmth(value: u32) -> Style {
    match value {
        0..=9 => GREEN,
        10..=99 => YELLOW,
        _ => RED,
    }
}

#[test]
fn an_explicit_style_replaces_the_ink_and_faint_paints() {
    let styles = STYLES.ink(INK);
    let mut list = counts();
    let before = render(&mut list, styles);
    assert!(before[(2, 1)].modifier.contains(Modifier::DIM));
    assert_eq!(before[(4, 1)].fg, Color::Gray);
    assert!(before[(9, 1)].modifier.contains(Modifier::DIM));
    let row = list.row_mut(1).unwrap();
    row.leading_mut().unwrap().set_style(RED);
    row.cells_mut()[0].set_style(GREEN);
    row.cells_mut()[1].set_style(RED);
    let after = render(&mut list, styles);
    assert_eq!(read(&after).text, read(&before).text);
    assert_eq!(after[(2, 1)].fg, Color::Red);
    assert!(!after[(2, 1)].modifier.contains(Modifier::DIM));
    assert_eq!(after[(4, 1)].fg, Color::Green);
    assert_eq!(after[(9, 1)].fg, Color::Red);
    assert!(!after[(9, 1)].modifier.contains(Modifier::DIM));
    assert_eq!(after[(4, 2)].fg, Color::Gray);
    assert_eq!(
        list.rows()[1],
        Row::new([Cell::new("b").style(GREEN), Cell::new("2").style(RED)])
            .mark(Mark::new("⧗").style(RED))
    );
}

#[test]
fn the_style_setters_keep_the_texts_buffers() {
    let mut cell = Cell::new(String::with_capacity(16)).faint();
    cell.set_text("12 kB");
    let at = cell.text().as_ptr();
    cell.set_style(RED);
    assert!(cell.set_part_style(0, GREEN));
    assert_eq!(cell.text().as_ptr(), at);
    assert_eq!(cell, Cell::new("12 kB").style(GREEN));
    let mut mark = Mark::new(String::with_capacity(8)).faint();
    mark.set_text("✓");
    let at = mark.text().as_ptr();
    mark.set_style(GREEN);
    assert_eq!(mark.text().as_ptr(), at);
    assert_eq!(mark, Mark::new("✓").style(GREEN));
    let mut parts = Cell::parts([Part::new("ab"), Part::new("cd").faint()]);
    let at = parts.text().as_ptr();
    assert!(parts.set_part_style(1, RED));
    parts.set_style(GREEN);
    assert_eq!(parts.text().as_ptr(), at);
}

#[test]
fn set_part_style_restyles_one_part_and_answers_false_for_a_part_that_isnt_there() {
    let mut cell = Cell::parts([
        Part::new("Done").style(GREEN),
        Part::new(" · 3").faint(),
        Part::new("!"),
    ]);
    assert!(cell.set_part_style(1, RED));
    assert!(cell.set_part_style(2, YELLOW));
    assert!(!cell.set_part_style(3, RED));
    assert_eq!(
        cell,
        Cell::parts([
            Part::new("Done").style(GREEN),
            Part::new(" · 3").style(RED),
            Part::new("!").style(YELLOW),
        ])
    );
    cell.set_text("x");
    assert!(!cell.set_part_style(1, RED));
    assert!(cell.set_part_style(0, RED));
    assert_eq!(cell, Cell::new("x").style(RED));
    let mut plain = Cell::new("one").faint();
    assert!(!plain.set_part_style(1, RED));
    assert_eq!(plain, Cell::new("one").faint());
    assert!(plain.set_part_style(0, RED));
    assert_eq!(plain, Cell::new("one").style(RED));
}

#[test]
fn set_style_on_a_cell_of_parts_styles_the_parts_without_their_own() {
    let parts = || {
        Cell::parts([
            Part::new("ab").style(GREEN),
            Part::new("cd"),
            Part::new("ef").faint(),
        ])
    };
    let mut cell = parts().faint();
    cell.set_style(RED);
    assert_eq!(cell, parts().style(RED));
    let mut list =
        List::new().with_rows([Row::new(["a", "1"]), Row::new([Cell::new("b"), parts()])]);
    list.row_mut(1).unwrap().cells_mut()[1].set_style(FAINT);
    let drawn = draw_with(&mut list, &TWO, 16, 2, |view| view.styles(STYLES));
    assert_eq!(drawn.text[1], "  b    abcdef");
    assert_eq!(drawn.marks[1], ".......ggffff");
}

#[test]
fn a_headings_part_restyled_in_place_draws_in_its_new_style() {
    let mut list = List::new().with_rows([
        Row::heading([Part::new("Running").faint(), Part::new(" · 2").faint()]),
        Row::new(["a", "1"]),
    ]);
    let heading = &mut list.row_mut(0).unwrap().cells_mut()[0];
    assert!(heading.set_part_style(0, GREEN));
    assert!(!heading.set_part_style(2, GREEN));
    let drawn = draw_with(&mut list, &TWO, 16, 2, |view| view.styles(STYLES));
    assert_eq!(drawn.text[0], "  Running · 2");
    assert_eq!(drawn.marks[0], "..gggggggffff");
    assert_eq!(list.selected(), Some(1));
}

#[test]
fn the_selected_row_replaces_a_set_style_or_patches_over_it_with_kept_colours() {
    let mut list = counts();
    list.row_mut(0).unwrap().cells_mut()[1].set_style(RED);
    list.row_mut(0)
        .unwrap()
        .leading_mut()
        .unwrap()
        .set_style(GREEN);
    let replaced = render(&mut list, STYLES.selected(LIT));
    assert_eq!(replaced[(9, 0)].fg, Color::Reset);
    assert_eq!(replaced[(9, 0)].bg, Color::Blue);
    assert_eq!(replaced[(2, 0)].fg, Color::Reset);
    let kept = render(&mut list, STYLES.selected(LIT).keep_colours(true));
    assert_eq!(kept[(9, 0)].fg, Color::Red);
    assert_eq!(kept[(9, 0)].bg, Color::Blue);
    assert!(kept[(9, 0)].modifier.contains(Modifier::BOLD));
    assert_eq!(kept[(2, 0)].fg, Color::Green);
    assert_eq!(kept[(2, 0)].bg, Color::Blue);
}

#[test]
fn numbers_recoloured_through_update_draw_warmer_as_they_rise() {
    let mut list = counts();
    list.select(2);
    list.update(|rows| {
        for row in rows.iter_mut() {
            let cell = &mut row.cells_mut()[1];
            let value = cell.text().parse().unwrap_or(0);
            cell.set_style(warmth(value));
        }
    });
    let buf = render(&mut list, STYLES);
    assert_eq!(buf[(9, 0)].fg, Color::Green);
    assert_eq!(buf[(9, 1)].fg, Color::Green);
    assert!(!buf[(9, 1)].modifier.contains(Modifier::DIM));
    list.update(|rows| {
        rows[0].cells_mut()[1].set_text("42");
        rows[0].cells_mut()[1].set_style(warmth(42));
    });
    let buf = render(&mut list, STYLES);
    assert_eq!(read(&buf).text[0], "  ⧗ a    42");
    assert_eq!(buf[(9, 0)].fg, Color::Yellow);
    assert_eq!(buf[(10, 0)].fg, Color::Yellow);
    assert_eq!(list.selected(), Some(2));
    assert_eq!(buf[(9, 2)].fg, Color::Magenta);
}
