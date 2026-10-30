use crate::common::*;
use pito_tui::list::{Column, List, ListView, Mark, Row, Styles};
use ratatui::{
    buffer::{Buffer, Cell},
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

const PINNED: [Column<'static>; 3] = [
    Column::new("Name", 10, 10).pinned(),
    Column::new("Owner", 10, 10).pinned(),
    Column::new("Message", 4, 0),
];

const SIZES: [Column<'static>; 3] = [
    Column::new("Name", 10, 10).pinned(),
    Column::new("Size", 10, 10).pinned().right(),
    Column::new("Message", 4, 0),
];

fn sentinel() -> Cell {
    let mut cell = Cell::default();
    cell.set_symbol("#");
    cell.set_style(Style::new().fg(Color::Red).bg(Color::Blue));
    cell
}

fn filled(area: Rect) -> Buffer {
    Buffer::filled(area, sentinel())
}

fn untouched_outside(buf: &Buffer, area: Rect) {
    let sentinel = sentinel();
    for y in buf.area.top()..buf.area.bottom() {
        for x in buf.area.left()..buf.area.right() {
            if area.contains((x, y).into()) {
                continue;
            }
            assert_eq!(buf[(x, y)], sentinel, "({x}, {y})");
        }
    }
}

fn same_inside(buf: &Buffer, alone: &Buffer) {
    for y in alone.area.top()..alone.area.bottom() {
        for x in alone.area.left()..alone.area.right() {
            assert_eq!(buf[(x, y)], alone[(x, y)], "({x}, {y})");
        }
    }
}

fn line(buf: &Buffer, y: u16, from: u16, to: u16) -> String {
    (from..to).map(|x| buf[(x, y)].symbol()).collect()
}

#[test]
fn pinned_columns_past_the_area_stop_at_its_right_edge() {
    let mut list = List::new().with_rows([Row::new(["deploy api", "billing-team", "uploading"])]);
    let area = Rect::new(0, 0, 20, 2);
    let mut buf = filled(Rect::new(0, 0, 40, 2));
    ListView::new(&mut list, &PINNED)
        .header(true)
        .render(area, &mut buf);
    for y in 0..2 {
        for x in 20..40 {
            assert_eq!(buf[(x, y)], sentinel(), "({x}, {y})");
        }
    }
    assert_eq!(line(&buf, 0, 0, 20), "  Name        Owner ");
    assert_eq!(line(&buf, 1, 0, 20), "▌ deploy api  …     ");
    assert_eq!(list.columns(), [(0, 2, 10), (1, 14, 6)]);
}

#[test]
fn a_list_cut_by_its_area_draws_as_at_the_buffer_edge() {
    let rows = [Row::new(["deploy api", "billing-team", "uploading"])];
    let area = Rect::new(0, 0, 20, 2);
    let mut buf = filled(Rect::new(0, 0, 40, 2));
    let mut alone = Buffer::empty(area);
    for target in [&mut buf, &mut alone] {
        let mut list = List::new().with_rows(rows.clone());
        ListView::new(&mut list, &PINNED)
            .styles(STYLES)
            .header(true)
            .render(area, target);
    }
    untouched_outside(&buf, area);
    same_inside(&buf, &alone);
}

#[test]
fn a_right_column_at_the_edge_aligns_to_the_area() {
    let mut list = List::new().with_rows([
        Row::new(["alpha", "12345", "x"]),
        Row::new(["bravo", "1234567", "y"]),
    ]);
    let area = Rect::new(0, 0, 20, 3);
    let mut buf = filled(Rect::new(0, 0, 40, 3));
    ListView::new(&mut list, &SIZES)
        .styles(STYLES)
        .header(true)
        .render(area, &mut buf);
    untouched_outside(&buf, area);
    assert_eq!(line(&buf, 0, 0, 20), "  Name          Size");
    assert_eq!(line(&buf, 1, 0, 20), "▌ alpha        12345");
    assert_eq!(
        line(&buf, 2, 0, 20),
        format!("  bravo{}…{}", " ".repeat(7), " ".repeat(5))
    );
    assert_eq!(list.columns(), [(0, 2, 10), (1, 14, 6)]);
}

#[test]
fn a_list_at_a_non_zero_x_leaves_both_sides_alone() {
    let rows = [
        Row::new(["deploy api", "billing-team", "uploading"]).mark(Mark::new("⧗")),
        Row::new(["update tool", "tools", "finished"]).mark(Mark::new("✓")),
        Row::new(["release", "billing", "stopped"]),
    ];
    let area = Rect::new(10, 1, 20, 4);
    let mut buf = filled(Rect::new(0, 0, 40, 6));
    let mut alone = Buffer::empty(area);
    for target in [&mut buf, &mut alone] {
        let mut list = List::new().with_rows(rows.clone());
        ListView::new(&mut list, &PINNED)
            .styles(STYLES)
            .header(true)
            .end(Some("· the end of the list ·"))
            .render(area, target);
        assert_eq!(list.columns(), [(0, 14, 10), (1, 26, 4)]);
    }
    untouched_outside(&buf, area);
    same_inside(&buf, &alone);
    assert_eq!(line(&buf, 2, 10, 30), "▌ ⧗ deploy api  …   ");
}

#[test]
fn every_write_stays_inside_an_offset_area() {
    let styles = Styles::new()
        .selected(SELECTED)
        .faint(FAINT)
        .base(Style::new().bg(Color::Black))
        .header(Style::new().add_modifier(Modifier::UNDERLINED))
        .header_key(Style::new().fg(Color::Yellow))
        .cursor(Style::new().fg(Color::Cyan))
        .keep_colours(true);
    let columns = [
        Column::new("Operation", 10, 16).pinned(),
        Column::new("Size", 9, 9).pinned().right(),
        Column::new("Owner", 8, 8).pinned().fit(12),
        Column::new("Message", 8, 0),
    ];
    let rows = [
        Row::new(["deploy api", "123456789", "billing", "uploading the bundle"])
            .mark(Mark::new("⧗").style(GREEN)),
        Row::heading(["Done"]),
        Row::new(["update tool", "42", "tools-and-more", "finished"]).mark(Mark::new("✓")),
        Row::blank(),
        Row::new(["release", "7", "billing", "stopped"]).mark(Mark::new("■")),
    ];
    for width in [1u16, 3, 6, 12, 20, 33, 45] {
        for x in [0u16, 7] {
            let area = Rect::new(x, 2, width, 8);
            let mut buf = filled(Rect::new(0, 0, 60, 12));
            let mut alone = Buffer::empty(area);
            for target in [&mut buf, &mut alone] {
                let mut list = List::new().with_rows(rows.clone());
                list.select(2);
                list.select_range(Some((0, 4)));
                ListView::new(&mut list, &columns)
                    .styles(styles)
                    .cursor("▌▌ ")
                    .header(true)
                    .key_column(Some(1))
                    .end(Some("· the end of the list ·"))
                    .render(area, target);
            }
            untouched_outside(&buf, area);
            same_inside(&buf, &alone);
        }
    }
}

#[test]
fn a_cursor_and_mark_wider_than_the_area_stay_inside() {
    let cursor = "▌".repeat(30);
    let mut list = List::new().with_rows([Row::new(["a", "b"]).mark(Mark::new("日".repeat(20)))]);
    let columns = [Column::new("A", 4, 4).pinned(), Column::new("B", 1, 0)];
    let area = Rect::new(5, 0, 12, 2);
    let mut buf = filled(Rect::new(0, 0, 80, 2));
    ListView::new(&mut list, &columns)
        .cursor(&cursor)
        .header(true)
        .render(area, &mut buf);
    untouched_outside(&buf, area);
    assert_eq!(line(&buf, 1, 5, 17), format!("…{}", " ".repeat(11)));
}

#[test]
fn an_empty_list_note_stays_inside_the_area() {
    let mut list = List::new();
    let area = Rect::new(4, 1, 10, 2);
    let mut buf = filled(Rect::new(0, 0, 30, 4));
    ListView::new(&mut list, &PINNED)
        .header(true)
        .empty(Some("Nothing here at all, not one row."))
        .render(area, &mut buf);
    untouched_outside(&buf, area);
    assert_eq!(line(&buf, 2, 4, 14), "  Nothing…");
}
