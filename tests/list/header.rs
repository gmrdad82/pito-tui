use crate::common::*;
use pito_tui::list::{Column, List, ListView, Mark, Row, Styles};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

const PAGE: Style = Style::new().fg(Color::White).bg(Color::Blue);
const HEAD: Style = Style::new().fg(Color::Yellow).bg(Color::Black);
const KEY: Style = Style::new().fg(Color::Cyan).add_modifier(Modifier::BOLD);

const SIZES: [Column<'static>; 3] = [
    Column::new("Name", 6, 6).pinned(),
    Column::new("Size", 5, 5).right().priority(1),
    Column::new("Note", 4, 0),
];

fn list() -> List {
    List::new().with_rows([
        Row::new(["ab", "12", "first"]).mark(Mark::new("⧗")),
        Row::new(["cd", "345", "second"]).mark(Mark::new("✓")),
    ])
}

fn render(styles: Styles, width: u16, key: Option<usize>, header: bool) -> Buffer {
    let mut list = list();
    let area = Rect::new(0, 0, width, 4);
    let mut buf = Buffer::empty(area);
    ListView::new(&mut list, &SIZES)
        .styles(styles)
        .header(header)
        .key_column(key)
        .end(Some("· end ·"))
        .render(area, &mut buf);
    buf
}

#[test]
fn a_header_style_spans_the_whole_line_and_the_key_title_draws_over_it() {
    let buf = render(
        STYLES.base(PAGE).header(HEAD).header_key(KEY),
        30,
        Some(1),
        true,
    );
    assert_eq!(read(&buf).text[0], "    Name     Size  Note");
    for x in 0..30 {
        assert_eq!(buf[(x, 0)].bg, Color::Black, "{x}");
        assert!(!buf[(x, 0)].modifier.contains(Modifier::DIM), "{x}");
    }
    for x in (0..13).chain(17..30) {
        assert_eq!(buf[(x, 0)].fg, Color::Yellow, "{x}");
        assert!(!buf[(x, 0)].modifier.contains(Modifier::BOLD), "{x}");
    }
    for x in 13..17 {
        assert_eq!(buf[(x, 0)].fg, Color::Cyan, "{x}");
        assert!(buf[(x, 0)].modifier.contains(Modifier::BOLD), "{x}");
    }
    for y in 1..4 {
        for x in 0..30 {
            assert_eq!(buf[(x, y)].bg, Color::Blue, "{x},{y}");
        }
    }
    assert_eq!(buf[(0, 1)].fg, Color::Magenta);
}

#[test]
fn titles_take_the_header_style_instead_of_the_faint_one() {
    let buf = render(STYLES.header(HEAD), 30, None, true);
    let marks = read(&buf).marks;
    assert_eq!(marks[0], "");
    for x in 0..30 {
        assert_eq!(buf[(x, 0)].fg, Color::Yellow, "{x}");
        assert_eq!(buf[(x, 0)].bg, Color::Black, "{x}");
    }
    assert_eq!(buf[(0, 2)].bg, Color::Reset);
}

#[test]
fn a_key_style_alone_marks_one_title_over_the_faint_ones() {
    let buf = render(STYLES.base(PAGE).header_key(KEY), 30, Some(0), true);
    for x in 4..8 {
        assert_eq!(buf[(x, 0)].fg, Color::Cyan, "{x}");
        assert_eq!(buf[(x, 0)].bg, Color::Blue, "{x}");
        assert!(buf[(x, 0)].modifier.contains(Modifier::BOLD), "{x}");
        assert!(!buf[(x, 0)].modifier.contains(Modifier::DIM), "{x}");
    }
    for x in (13..17).chain(19..23) {
        assert!(buf[(x, 0)].modifier.contains(Modifier::DIM), "{x}");
        assert_eq!(buf[(x, 0)].fg, Color::White, "{x}");
    }
    for x in (0..4).chain(8..13) {
        assert!(!buf[(x, 0)].modifier.contains(Modifier::DIM), "{x}");
        assert_eq!(buf[(x, 0)].bg, Color::Blue, "{x}");
    }
}

#[test]
fn the_defaults_draw_the_header_as_before() {
    let plain = render(STYLES, 30, None, true);
    assert_eq!(read(&plain).marks[0], "....ffff.....ffff..ffff");
    for key in [Some(0), Some(1), Some(2), Some(9)] {
        assert_eq!(render(STYLES, 30, key, true), plain, "{key:?}");
    }
    let keyed = STYLES.header_key(KEY);
    assert_eq!(render(keyed, 30, None, true), plain);
    assert_eq!(render(keyed, 30, Some(9), true), plain);
    let headed = STYLES.header(HEAD).header_key(KEY);
    assert_eq!(
        render(headed, 30, Some(1), false),
        render(STYLES, 30, None, false)
    );
}

#[test]
fn a_dropped_key_column_marks_nothing() {
    let buf = render(STYLES.header(HEAD).header_key(KEY), 18, Some(1), true);
    assert_eq!(read(&buf).text[0], "    Name    Note");
    for x in 0..18 {
        assert_eq!(buf[(x, 0)].fg, Color::Yellow, "{x}");
        assert_eq!(buf[(x, 0)].bg, Color::Black, "{x}");
    }
}
