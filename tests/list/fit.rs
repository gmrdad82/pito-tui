use std::{borrow::Cow, cell::RefCell};

use crate::common::*;
use pito_tui::list::{Cell, Column, List, ListView, Part, Row, Source};
use ratatui::{buffer::Buffer, layout::Rect, widgets::Widget};

const SIZED: [Column<'static>; 3] = [
    Column::new("Name", 4, 0).fit(12),
    Column::new("Size", 4, 4).right(),
    Column::new("Note", 4, 0),
];

fn render<S: Source>(
    list: &mut List<S>,
    columns: &[Column],
    width: u16,
    height: u16,
    header: bool,
) -> Buffer {
    let area = Rect::new(0, 0, width, height);
    let mut buf = Buffer::empty(area);
    ListView::new(list, columns)
        .styles(STYLES)
        .header(header)
        .render(area, &mut buf);
    buf
}

fn three() -> List {
    List::new().with_rows([
        Row::new(["ab", "1", "x"]),
        Row::new(["abcdef", "22", "y"]),
        Row::new(["abc", "333", "z"]),
    ])
}

#[test]
fn a_fit_column_takes_its_widest_cell_on_screen() {
    let mut list = three();
    let drawn = read(&render(&mut list, &SIZED, 30, 4, false));
    assert_eq!(list.columns(), [(0, 2, 6), (1, 10, 4), (2, 16, 14)]);
    assert_eq!(
        drawn.text[..3],
        [
            "▌ ab         1  x",
            "  abcdef    22  y",
            "  abc      333  z"
        ]
    );
    let plain = [
        Column::new("Name", 4, 0),
        Column::new("Size", 4, 4).right(),
        Column::new("Note", 4, 0),
    ];
    let drawn = read(&render(&mut list, &plain, 30, 4, false));
    assert_eq!(drawn.text[1], "  abc…    22  y");
}

#[test]
fn the_title_counts_when_the_header_is_drawn() {
    let titled = [
        Column::new("Filename", 4, 0).fit(12),
        Column::new("Size", 4, 4).right(),
        Column::new("Note", 4, 0),
    ];
    let mut list = three();
    let drawn = read(&render(&mut list, &titled, 30, 5, true));
    assert_eq!(list.columns(), [(0, 2, 8), (1, 12, 4), (2, 18, 12)]);
    assert_eq!(drawn.text[0], "  Filename  Size  Note");
    render(&mut list, &titled, 30, 5, false);
    assert_eq!(list.columns(), [(0, 2, 6), (1, 10, 4), (2, 16, 14)]);
}

#[test]
fn the_cap_and_the_minimum_bound_it() {
    let mut list = List::new().with_rows([Row::new(["a very long file name.txt", "1", "x"])]);
    let drawn = read(&render(&mut list, &SIZED, 40, 2, false));
    assert_eq!(list.columns()[0], (0, 2, 12));
    assert_eq!(drawn.text[0], "▌ a very long…     1  x");
    let mut list = List::new().with_rows([Row::new(["a", "1", "x"])]);
    render(&mut list, &SIZED, 40, 2, false);
    assert_eq!(list.columns()[0], (0, 2, 4));
    let low = [Column::new("Name", 4, 0).fit(2), Column::new("Note", 4, 0)];
    let mut list = List::new().with_rows([Row::new(["abcdef", "x"])]);
    render(&mut list, &low, 40, 2, false);
    assert_eq!(list.columns(), [(0, 2, 4), (1, 8, 32)]);
}

#[test]
fn only_the_rows_on_screen_count() {
    let mut list = List::new().with_rows((0..30).map(|n| {
        let name = if n == 25 {
            "a much wider name".to_string()
        } else {
            format!("row {n}")
        };
        Row::new([name, n.to_string(), "x".to_string()])
    }));
    render(&mut list, &SIZED, 40, 5, false);
    assert_eq!(list.columns()[0], (0, 2, 5));
    list.select(25);
    render(&mut list, &SIZED, 40, 5, false);
    assert_eq!(list.top(), 21);
    assert_eq!(list.columns()[0], (0, 2, 12));
    list.select(0);
    render(&mut list, &SIZED, 40, 5, false);
    assert_eq!(list.columns()[0], (0, 2, 5));
}

struct Names {
    built: RefCell<Vec<usize>>,
}

impl Source for Names {
    fn len(&self) -> usize {
        1000
    }

    fn row(&self, index: usize) -> Cow<'_, Row> {
        self.built.borrow_mut().push(index);
        let name = if index == 500 {
            "the widest name".to_string()
        } else {
            format!("n{index}")
        };
        Cow::Owned(Row::new([name, "1".to_string(), "x".to_string()]))
    }

    fn selectable(&self, _: usize) -> bool {
        true
    }
}

fn built(list: &List<Names>) -> Vec<usize> {
    let mut built = list.source().built.take();
    built.sort_unstable();
    built
}

#[test]
fn a_lazy_source_measures_only_the_rows_on_screen_and_builds_them_twice() {
    let mut list = List::from_source(Names {
        built: RefCell::new(Vec::new()),
    });
    render(&mut list, &SIZED, 40, 10, false);
    assert_eq!(list.columns()[0], (0, 2, 4));
    let twice: Vec<usize> = (0..10).flat_map(|n| [n, n]).collect();
    assert_eq!(built(&list), twice);
    let plain = [
        Column::new("Name", 4, 0),
        Column::new("Size", 4, 4).right(),
        Column::new("Note", 4, 0),
    ];
    render(&mut list, &plain, 40, 10, false);
    assert_eq!(built(&list), (0..10).collect::<Vec<_>>());
    list.select(500);
    render(&mut list, &SIZED, 40, 10, false);
    assert_eq!(list.top(), 491);
    assert_eq!(list.columns()[0], (0, 2, 12));
    let seen = built(&list);
    assert!(seen.contains(&500));
    assert!(
        seen.iter().all(|index| (491..501).contains(index)),
        "{seen:?}"
    );
}

#[test]
fn rest_cells_headings_and_blank_rows_do_not_count() {
    let columns = [
        Column::new("N", 1, 0).fit(20),
        Column::new("S", 1, 0).fit(10),
        Column::new("Note", 4, 0),
    ];
    let mut list = List::new().with_rows([
        Row::heading([Part::new("a heading wider than any cell")]),
        Row::blank(),
        Row::new([Cell::new("a note across the row").rest()]),
        Row::new(["abc", "1", "n"]),
        Row::new([Cell::new("ab"), Cell::new("a long size").rest()]),
    ]);
    let drawn = read(&render(&mut list, &columns, 30, 6, false));
    assert_eq!(list.columns(), [(0, 2, 3), (1, 7, 1), (2, 10, 20)]);
    assert_eq!(drawn.text[2], "▌ a note across the row");
    assert_eq!(drawn.text[4], "  ab   a long size");
}

#[test]
fn a_fit_column_drops_by_priority_and_shares_the_spare_width() {
    let columns = [
        Column::new("Name", 4, 0).fit(12).pinned(),
        Column::new("Size", 4, 0).fit(8).priority(1).right(),
        Column::new("Note", 4, 0),
    ];
    let mut list = List::new().with_rows([Row::new(["abcdefghij", "123456", "x"])]);
    render(&mut list, &columns, 40, 2, false);
    assert_eq!(list.columns(), [(0, 2, 10), (1, 14, 6), (2, 22, 18)]);
    let drawn = read(&render(&mut list, &columns, 20, 2, false));
    assert_eq!(list.columns(), [(0, 2, 6), (1, 10, 4), (2, 16, 4)]);
    assert_eq!(drawn.text[0], "▌ abcde…  123…  x");
    render(&mut list, &columns, 17, 2, false);
    assert_eq!(list.columns(), [(0, 2, 9), (2, 13, 4)]);
}

#[test]
fn a_fit_on_the_flexible_column_changes_nothing() {
    let fitted = [
        Column::new("Name", 4, 0).fit(12),
        Column::new("Size", 4, 4).right(),
        Column::new("Note", 4, 0).fit(1),
    ];
    let mut list = three();
    assert_eq!(
        render(&mut list, &fitted, 30, 4, true),
        render(&mut list, &SIZED, 30, 4, true)
    );
    let middle = [
        Column::new("Name", 4, 0).fit(30).flex(),
        Column::new("Size", 4, 4).right(),
    ];
    render(&mut list, &middle, 30, 4, true);
    assert_eq!(list.columns(), [(0, 2, 22), (1, 26, 4)]);
}

#[test]
fn parts_and_wide_glyphs_measure_by_cell() {
    let mut list = List::new().with_rows([
        Row::new([
            Cell::parts([Part::new("ab"), Part::new("cd").faint()]),
            Cell::new("1"),
            Cell::new("x"),
        ]),
        Row::new(["日本語", "2", "y"]),
        Row::new(["ținut", "3", "z"]),
    ]);
    let drawn = read(&render(&mut list, &SIZED, 30, 4, false));
    assert_eq!(list.columns()[0], (0, 2, 6));
    assert_eq!(drawn.text[1], "  日本語     2  y");
}
