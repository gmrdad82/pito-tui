use crate::common::*;
use pito_tui::list::{Cell, Column, List, ListView, Mark, Row};
use ratatui::{buffer::Buffer, layout::Rect, widgets::Widget};

#[test]
fn huge_texts_are_clipped_without_overflow() {
    let huge = "ă日".repeat(100_000);
    let rows = vec![
        Row::new([
            huge.clone(),
            huge.clone(),
            huge.clone(),
            huge.clone(),
            huge.clone(),
        ])
        .mark(Mark::new(huge.clone())),
        Row::new(["x", "y", "z", "w", "v"]),
    ];
    for width in [0u16, 1, 2, 7, 40, 80, 150, 200, u16::MAX] {
        let mut list = list_of(rows.clone());
        let drawn = draw(&mut list, width.min(2000), 4);
        for line in &drawn.text {
            assert!(cells(line) <= usize::from(width), "{width}");
        }
    }
}

#[test]
fn a_huge_mark_and_cursor_leave_the_area_whole() {
    let huge = "日".repeat(100_000);
    let columns = [Column::new("", 4, 4), Column::new("", 1, 0)];
    let mut list = List::new().with_rows([Row::new(["a", "b"]).mark(Mark::new(huge.clone()))]);
    let drawn = draw_with(&mut list, &columns, 30, 2, |view| view.cursor(&huge));
    for line in &drawn.text {
        assert!(cells(line) <= 30);
    }
}

#[test]
fn huge_widths_and_gaps_saturate() {
    let columns = [
        Column::new("a", u16::MAX, u16::MAX).priority(255),
        Column::new("b", u16::MAX, u16::MAX),
        Column::new("c", u16::MAX, u16::MAX).pinned(),
        Column::new("d", u16::MAX, 0),
    ];
    let mut list = List::new().with_rows([Row::new(["1", "2", "3", "4"])]);
    for gap in [0u16, 2, u16::MAX] {
        let drawn = draw_with(&mut list, &columns, 100, 3, |view| {
            view.gap(gap).header(true)
        });
        for line in &drawn.text {
            assert!(cells(line) <= 100);
        }
    }
}

#[test]
fn no_columns_draw_the_cursor_only() {
    let mut list = List::new().with_rows([Row::new(["a"]), Row::new(["b"])]);
    let drawn = draw_with(&mut list, &[], 10, 3, |view| view.header(true));
    assert_eq!(drawn.text, ["", "▌", ""]);
}

#[test]
fn a_single_column_is_the_message() {
    let columns = [Column::new("Only", 3, 3)];
    let mut list = List::new().with_rows([Row::new(["abcdefghijkl"])]);
    let drawn = draw_with(&mut list, &columns, 10, 2, |view| view.header(true));
    assert_eq!(drawn.text, ["  Only", "▌ abcdefg…"]);
}

#[test]
fn tiny_areas_never_panic() {
    let rows: Vec<Row> = (0..5)
        .map(|_| Row::new(["abc", "def", "g", "h", "i"]))
        .collect();
    for width in 0..8u16 {
        for height in 0..5u16 {
            let mut list = list_of(rows.clone());
            list.select(4);
            let area = Rect::new(0, 0, width, height);
            let mut buf = Buffer::empty(area);
            ListView::new(&mut list, &COLUMNS)
                .styles(STYLES)
                .header(true)
                .end(Some("· end ·"))
                .render(area, &mut buf);
            ListView::new(&mut list, &COLUMNS).render(area, &mut buf);
        }
    }
}

#[test]
fn an_area_beyond_the_buffer_is_cut_to_it() {
    let mut list = list_of(operations());
    let buf_area = Rect::new(0, 0, 40, 4);
    let mut buf = Buffer::empty(buf_area);
    ListView::new(&mut list, &COLUMNS)
        .styles(STYLES)
        .render(Rect::new(0, 0, 200, 50), &mut buf);
    assert_eq!(buf.area, buf_area);
}

#[test]
fn an_area_offset_inside_the_buffer_draws_only_there() {
    let mut list = list_of(operations());
    let buf_area = Rect::new(0, 0, 60, 8);
    let mut buf = Buffer::empty(buf_area);
    for cell in buf.content.iter_mut() {
        cell.set_symbol("#");
    }
    ListView::new(&mut list, &COLUMNS)
        .styles(STYLES)
        .render(Rect::new(10, 2, 30, 3), &mut buf);
    let drawn = read(&buf);
    assert_eq!(drawn.text[1], "#".repeat(60));
    assert_eq!(drawn.text[5], "#".repeat(60));
    assert!(drawn.text[2].starts_with(&"#".repeat(10)));
    assert!(drawn.text[2].ends_with(&"#".repeat(20)));
    assert!(drawn.text[2].contains("▌ ⧗ deploy api"));
}

#[test]
fn a_zero_width_cell_text_is_harmless() {
    let mut list = list_of(vec![Row::new(["", "\u{200b}", "\u{301}", "\n", "\u{7}"])]);
    let drawn = draw(&mut list, 60, 3);
    assert!(drawn.text[1].starts_with("▌"));
}

#[test]
fn zero_rows_in_a_one_line_area_with_a_header_draws_the_header_only() {
    let mut list = list_of(operations());
    let drawn = draw(&mut list, 60, 1);
    assert!(drawn.text[0].contains("Operation"));
}

#[test]
fn rows_and_cells_expose_their_content() {
    let row = Row::new(["a", "b"]).mark(Mark::new("x"));
    assert_eq!(row.cells().len(), 2);
    assert_eq!(row.cells()[1].text(), "b");
    assert_eq!(row.leading().map(Mark::text), Some("x"));
    assert_eq!(Cell::from("q").text(), "q");
    assert_eq!(Cell::from(String::from("r")).text(), "r");
}
