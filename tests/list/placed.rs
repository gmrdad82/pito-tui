use crate::common::*;
use pito_tui::list::{Column, List, ListView, Row};
use ratatui::{buffer::Buffer, layout::Rect, style::Style, widgets::Widget};

const TITLES: [&str; 5] = ["Operation", "Versions", "Owner", "State", "Message"];

const FILES: [Column<'static>; 3] = [
    Column::new("Name", 8, 0).flex(),
    Column::new("Size", 4, 8).right(),
    Column::new("Lines", 5, 6).right(),
];

fn render(list: &mut List, columns: &[Column], area: Rect, header: bool) -> Buffer {
    let mut buf = Buffer::empty(Rect::new(0, 0, 60, 10));
    ListView::new(list, columns)
        .styles(STYLES)
        .header(header)
        .render(area, &mut buf);
    buf
}

fn slice(line: &str, x: u16, len: usize) -> String {
    line.chars().skip(usize::from(x)).take(len).collect()
}

#[test]
fn no_columns_are_known_before_a_draw() {
    assert!(List::new().columns().is_empty());
    assert!(list_of(operations()).columns().is_empty());
}

#[test]
fn a_draw_places_each_kept_column_where_its_cells_start() {
    let mut list = list_of(operations());
    let drawn = draw(&mut list, 80, 8);
    assert_eq!(
        list.columns(),
        [(0, 4, 16), (1, 22, 12), (2, 36, 8), (3, 46, 8), (4, 56, 24)]
    );
    for &(at, x, _) in list.columns() {
        assert_eq!(slice(&drawn.text[0], x, TITLES[at].len()), TITLES[at]);
    }
    draw(&mut list, 40, 8);
    assert_eq!(
        list.columns(),
        [(0, 4, 12), (2, 18, 5), (3, 25, 5), (4, 32, 8)]
    );
}

#[test]
fn positions_follow_the_area_and_answer_for_the_last_draw() {
    let mut list = list_of(operations());
    render(&mut list, &COLUMNS, Rect::new(10, 2, 40, 6), true);
    assert_eq!(
        list.columns(),
        [(0, 14, 12), (2, 28, 5), (3, 35, 5), (4, 42, 8)]
    );
    let shown = list.columns().to_vec();
    render(&mut list, &COLUMNS, Rect::new(10, 2, 40, 6), false);
    assert_eq!(list.columns(), shown);
    list.select(3);
    list.set_rows(operations());
    assert_eq!(list.columns(), shown);
    render(&mut list, &COLUMNS, Rect::new(10, 2, 0, 6), true);
    assert!(list.columns().is_empty());
    render(&mut list, &COLUMNS, Rect::new(70, 2, 10, 6), true);
    assert!(list.columns().is_empty());
}

#[test]
fn the_edge_clips_a_column_and_one_with_no_room_is_absent() {
    let columns = [
        Column::new("A", 10, 10).pinned(),
        Column::new("B", 10, 10).pinned(),
        Column::new("C", 4, 0),
    ];
    let mut list = List::new().with_rows([Row::new(["a", "b", "c"])]);
    render(&mut list, &columns, Rect::new(0, 0, 20, 3), true);
    assert_eq!(list.columns(), [(0, 2, 10), (1, 14, 6)]);
    render(&mut list, &columns, Rect::new(0, 0, 13, 3), true);
    assert_eq!(list.columns(), [(0, 2, 10)]);
    render(&mut list, &columns, Rect::new(0, 0, 2, 3), true);
    assert!(list.columns().is_empty());
}

#[test]
fn right_aligned_and_flexible_columns_report_their_whole_cells() {
    let mut list = List::new().with_rows([Row::new(["a.txt", "12 kB", "300"])]);
    let buf = render(&mut list, &FILES, Rect::new(0, 0, 40, 3), true);
    assert_eq!(list.columns(), [(0, 2, 20), (1, 24, 8), (2, 34, 6)]);
    let line = &read(&buf).text[1];
    assert_eq!(slice(line, 2, 5), "a.txt");
    assert_eq!(slice(line, 27, 5), "12 kB");
    assert_eq!(slice(line, 37, 3), "300");
}

#[test]
fn an_empty_list_still_places_its_columns() {
    let mut list = List::new();
    render(&mut list, &FILES, Rect::new(0, 0, 40, 3), true);
    assert_eq!(list.columns(), [(0, 2, 20), (1, 24, 8), (2, 34, 6)]);
}

#[test]
fn a_widget_laid_over_one_cell_covers_exactly_that_cell() {
    let mut list = list_of(operations());
    list.select(2);
    let area = Rect::new(0, 0, 80, 4);
    let mut buf = Buffer::empty(area);
    ListView::new(&mut list, &COLUMNS)
        .styles(STYLES)
        .header(true)
        .render(area, &mut buf);
    let &(_, x, width) = list.columns().iter().find(|&&(at, _, _)| at == 3).unwrap();
    let y = area.y + 1 + u16::try_from(2 - list.top()).unwrap();
    let cell = Rect::new(x, y, width, 1);
    buf.set_string(
        cell.x,
        cell.y,
        "#".repeat(usize::from(cell.width)),
        Style::new(),
    );
    let drawn = read(&buf);
    assert_eq!(
        drawn.text[usize::from(y)],
        "▌ ✗ install app       v3 → v4       design    ########  exit 1: the build could…"
    );
}
