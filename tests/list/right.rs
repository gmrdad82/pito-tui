use crate::common::*;
use pito_tui::list::{Column, List, ListView, Row};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Style},
    widgets::Widget,
};

const SIZES: [Column<'static>; 3] = [
    Column::new("Name", 6, 6),
    Column::new("Size", 6, 6).right(),
    Column::new("Note", 4, 0),
];

fn sized(rows: &[[&str; 3]]) -> List {
    List::new().with_rows(rows.iter().map(|row| Row::new(*row)))
}

#[test]
fn a_right_column_pads_on_the_left_and_aligns_its_header() {
    let mut list = sized(&[
        ["ab", "12", "x"],
        ["cd", "12345", "y"],
        ["ef", "123456", "z"],
        ["gh", "1234567", "w"],
    ]);
    let drawn = draw_with(&mut list, &SIZES, 30, 6, |view| {
        view.styles(STYLES).header(true)
    });
    assert_eq!(
        drawn.text,
        [
            "  Name      Size  Note",
            "▌ ab          12  x",
            "  cd       12345  y",
            "  ef      123456  z",
            "  gh      12345…  w",
            "",
        ]
    );
    assert_eq!(drawn.marks[0], "..ffff......ffff..ffff");
    assert_eq!(drawn.marks[1], "S".repeat(30));
}

#[test]
fn a_right_column_clips_as_before() {
    let left = [
        Column::new("Name", 6, 6),
        Column::new("Kilobytes", 6, 6),
        Column::new("Note", 4, 0),
    ];
    let right = [left[0], left[1].right(), left[2]];
    let rows = [
        ["a", "1234567", "x"],
        ["b", "日本語日本語", "y"],
        ["c", "123456789012345", "z"],
        ["d", "12345日", "w"],
        ["e", "日本語", "v"],
    ];
    let mut plain = sized(&rows);
    let mut aligned = sized(&rows);
    let expected = draw_with(&mut plain, &left, 30, 6, |view| {
        view.styles(STYLES).header(true)
    });
    let drawn = draw_with(&mut aligned, &right, 30, 6, |view| {
        view.styles(STYLES).header(true)
    });
    assert_eq!(drawn.text, expected.text);
    assert_eq!(drawn.marks, expected.marks);
    assert_eq!(
        drawn.text,
        [
            "  Name    Kilob…  Note",
            "▌ a       12345…  x",
            "  b       日本…   y",
            "  c       12345…  z",
            "  d       12345…  w",
            "  e       日本語  v",
        ]
    );
}

#[test]
fn a_right_column_pads_by_cells() {
    let columns = [
        Column::new("", 3, 3),
        Column::new("", 6, 6).right(),
        Column::new("", 1, 0),
    ];
    let mut list = sized(&[
        ["a", "日本", "m"],
        ["b", "ă", "m"],
        ["c", "e\u{301}", "m"],
        ["d", "a\tb", "m"],
        ["e", "", "m"],
    ]);
    let drawn = draw_with(&mut list, &columns, 20, 5, |view| view);
    assert_eq!(
        drawn.text,
        [
            "▌ a      日本  m",
            "  b         ă  m",
            "  c         e\u{301}  m",
            "  d       a b  m",
            "  e            m",
        ]
    );
}

#[test]
fn a_right_last_column_ends_at_the_right_edge() {
    let columns = [
        Column::new("Name", 6, 6),
        Column::new("Total", 4, 0).right(),
    ];
    let mut list = sized(&[["a", "42", ""], ["b", "1234567890123", ""]]);
    let drawn = draw_with(&mut list, &columns, 20, 3, |view| view.header(true));
    assert_eq!(
        drawn.text,
        [
            "  Name         Total",
            "▌ a               42",
            "  b       123456789…",
        ]
    );
    assert_eq!(cells(&drawn.text[1]), 20);
}

#[test]
fn a_right_columns_padding_takes_the_base() {
    let page = Style::new().bg(Color::Blue);
    let mut list = sized(&[["ab", "12", "x"], ["cd", "3", "y"]]);
    let area = Rect::new(0, 0, 30, 4);
    let mut buf = Buffer::empty(area);
    ListView::new(&mut list, &SIZES)
        .styles(STYLES.base(page))
        .render(area, &mut buf);
    for x in 0..30 {
        assert_eq!(buf[(x, 1)].bg, Color::Blue, "{x}");
    }
    assert_eq!(read(&buf).text[1], "  cd           3  y");
}

#[test]
fn right_columns_never_leave_the_area() {
    let huge = "ă日".repeat(10_000);
    let columns = [
        Column::new("a", 10, 10).pinned().right(),
        Column::new("b", 10, 10).pinned().right(),
        Column::new("c", 3, 8).priority(1).right(),
        Column::new("d", 1, 0).right(),
    ];
    let rows = [
        Row::new(["7", "42", "日", "x"]),
        Row::new([huge.as_str(), huge.as_str(), huge.as_str(), huge.as_str()]),
        Row::new(["", "", "", ""]),
    ];
    for width in 0..60u16 {
        for gap in [0u16, 2, u16::MAX] {
            let mut list = List::new().with_rows(rows.clone());
            let drawn = draw_with(&mut list, &columns, width, 4, |view| {
                view.header(true).gap(gap)
            });
            for line in &drawn.text {
                assert!(cells(line) <= usize::from(width), "{width} {gap}: {line:?}");
            }
        }
    }
}
