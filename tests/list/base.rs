use crate::common::*;
use pito_tui::list::{Cell, Column, List, ListView, Row, Styles};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

const PAGE: Style = Style::new().fg(Color::White).bg(Color::Blue);

const NARROW: [Column<'static>; 3] = [
    Column::new("Name", 6, 6),
    Column::new("Size", 5, 5),
    Column::new("Note", 4, 0),
];

fn rows() -> Vec<Row> {
    vec![
        Row::new(["ab", "12", "first"]),
        Row::new(["cd", "345", "second"]),
        Row::new(["ef", "6", "third"]),
    ]
}

fn render(list: &mut List, styles: Styles, buf: &mut Buffer) {
    let area = buf.area;
    ListView::new(list, &NARROW)
        .styles(styles)
        .header(true)
        .end(Some("· end ·"))
        .empty(Some("Nothing here."))
        .render(area, buf);
}

fn fresh(list: &mut List, styles: Styles) -> Buffer {
    let mut buf = Buffer::empty(Rect::new(0, 0, 30, 7));
    render(list, styles, &mut buf);
    buf
}

fn every(buf: &Buffer) -> impl Iterator<Item = (u16, u16)> + '_ {
    let area = buf.area;
    (area.top()..area.bottom()).flat_map(move |y| (area.left()..area.right()).map(move |x| (x, y)))
}

#[test]
fn a_base_style_shows_in_the_gaps_padding_markers_and_below_the_end() {
    let mut list = List::new().with_rows(rows());
    let buf = fresh(&mut list, STYLES.base(PAGE));
    assert_eq!(
        read(&buf).text,
        [
            "  Name    Size   Note",
            "▌ ab      12     first",
            "  cd      345    second",
            "  ef      6      third",
            "  · end ·",
            "",
            "",
        ]
    );
    let base = |x: u16, y: u16| {
        let cell = &buf[(x, y)];
        assert_eq!(cell.bg, Color::Blue, "{x},{y}");
        assert_eq!(cell.fg, Color::White, "{x},{y}");
    };
    for y in [2, 3] {
        base(0, y);
        base(1, y);
    }
    for y in [0, 2, 3] {
        for x in [8, 9, 15, 16] {
            base(x, y);
        }
    }
    for x in 4..8 {
        base(x, 2);
    }
    for x in 13..15 {
        base(x, 2);
    }
    for x in 23..30 {
        base(x, 2);
    }
    for x in 9..30 {
        base(x, 4);
    }
    for y in 5..7 {
        for x in 0..30 {
            base(x, y);
        }
    }
    for (x, y) in every(&buf) {
        assert_eq!(buf[(x, y)].bg, Color::Blue, "{x},{y}");
    }
}

#[test]
fn the_selected_row_draws_its_style_over_the_base() {
    let mut list = List::new().with_rows(rows());
    let buf = fresh(&mut list, STYLES.base(PAGE));
    for x in 0..30 {
        let cell = &buf[(x, 1)];
        assert_eq!(cell.bg, Color::Blue, "{x}");
        assert_eq!(cell.fg, Color::Magenta, "{x}");
        assert!(cell.modifier.contains(Modifier::BOLD), "{x}");
    }
    let lit = STYLES.selected(SELECTED.bg(Color::Red)).base(PAGE);
    let buf = fresh(&mut list, lit);
    for (x, y) in every(&buf) {
        let want = if y == 1 { Color::Red } else { Color::Blue };
        assert_eq!(buf[(x, y)].bg, want, "{x},{y}");
    }
    assert_eq!(read(&buf).marks[1], "S".repeat(30));
}

#[test]
fn cell_styles_draw_over_the_base() {
    let mut list = List::new().with_rows([
        Row::new(["ab", "12", "first"]),
        Row::new([
            Cell::new("cd").style(Style::new().fg(Color::Black).bg(Color::Green)),
            Cell::new("345").faint(),
            Cell::new("second"),
        ]),
    ]);
    let buf = fresh(&mut list, STYLES.base(PAGE));
    for x in 2..4 {
        assert_eq!(buf[(x, 2)].bg, Color::Green, "{x}");
        assert_eq!(buf[(x, 2)].fg, Color::Black, "{x}");
    }
    for x in 4..10 {
        assert_eq!(buf[(x, 2)].bg, Color::Blue, "{x}");
    }
    for x in 10..13 {
        assert_eq!(buf[(x, 2)].bg, Color::Blue, "{x}");
        assert!(buf[(x, 2)].modifier.contains(Modifier::DIM), "{x}");
    }
    for x in 17..23 {
        assert_eq!(buf[(x, 2)].bg, Color::Blue, "{x}");
        assert_eq!(buf[(x, 2)].fg, Color::White, "{x}");
    }
    for x in 0..30 {
        assert_eq!(buf[(x, 0)].bg, Color::Blue, "{x}");
    }
}

#[test]
fn a_base_style_replaces_what_was_under_it() {
    for rows in [rows(), Vec::new()] {
        let mut list = List::new().with_rows(rows);
        let mut buf = Buffer::empty(Rect::new(0, 0, 30, 7));
        for cell in buf.content.iter_mut() {
            cell.set_symbol("#")
                .set_style(Style::new().bg(Color::Red).add_modifier(Modifier::ITALIC));
        }
        render(&mut list, STYLES.base(PAGE), &mut buf);
        assert!(read(&buf).text.iter().all(|line| !line.contains('#')));
        for (x, y) in every(&buf) {
            assert_eq!(buf[(x, y)].bg, Color::Blue, "{x},{y}");
            assert!(!buf[(x, y)].modifier.contains(Modifier::ITALIC), "{x},{y}");
        }
    }
}

#[test]
fn the_default_base_draws_what_was_drawn_before() {
    for rows in [rows(), Vec::new()] {
        let mut list = List::new().with_rows(rows);
        let before = fresh(&mut list, STYLES);
        let after = fresh(&mut list, STYLES.base(Style::new()));
        assert_eq!(before, after);
        for (x, y) in every(&before) {
            assert_eq!(before[(x, y)].bg, Color::Reset, "{x},{y}");
        }
    }
    assert_eq!(Styles::new().base(Style::new()), Styles::new());
}
