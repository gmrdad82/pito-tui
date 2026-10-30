use crate::common::*;
use pito_tui::list::{Cell, Column, List, Part, Row};

const TWO: [Column<'static>; 2] = [Column::new("Name", 8, 8), Column::new("Note", 4, 0)];

const WIDE: [Column<'static>; 4] = [
    Column::new("Name", 6, 6),
    Column::new("Size", 6, 6).right(),
    Column::new("Owner", 6, 6),
    Column::new("Note", 4, 0),
];

fn two(cell: Cell) -> List {
    List::new().with_rows([Row::new(["zz", "first"]), Row::new([cell, Cell::new("x")])])
}

#[test]
fn a_cell_of_parts_pads_as_one() {
    let mut list = two(Cell::parts([
        Part::new("ab").style(GREEN),
        Part::new("cd").faint(),
    ]));
    let drawn = draw_with(&mut list, &TWO, 20, 2, |view| view.styles(STYLES));
    assert_eq!(drawn.text, ["▌ zz        first", "  abcd      x"]);
    assert_eq!(drawn.marks[1], "..ggff");
}

#[test]
fn a_cell_of_parts_clips_as_one() {
    let mut list = two(Cell::parts([
        Part::new("abcde").style(GREEN),
        Part::new("fghij").faint(),
    ]));
    let drawn = draw_with(&mut list, &TWO, 20, 2, |view| view.styles(STYLES));
    assert_eq!(drawn.text[1], "  …         x");
    assert_eq!(drawn.marks[1], "..g");
    let mut list = two(Cell::parts([
        Part::new("abcdefghij").style(GREEN),
        Part::new("zz").faint(),
    ]));
    let drawn = draw_with(&mut list, &TWO, 20, 2, |view| view.styles(STYLES));
    assert_eq!(drawn.text[1], "  …         x");
    assert_eq!(drawn.marks[1], "..g");
    let mut list = two(Cell::parts(["日本", "語日本語"]));
    let drawn = draw_with(&mut list, &TWO, 20, 2, |view| view.styles(STYLES));
    assert_eq!(drawn.text[1], "  日本語…   x");
}

#[test]
fn the_selected_row_draws_every_part_in_the_selected_style() {
    let mut list = two(Cell::parts([
        Part::new("ab").style(GREEN),
        Part::new("cd").faint(),
    ]));
    list.select(1);
    let drawn = draw_with(&mut list, &TWO, 20, 2, |view| view.styles(STYLES));
    assert_eq!(drawn.text[1], "▌ abcd      x");
    assert_eq!(drawn.marks[1], "S".repeat(20));
}

#[test]
fn parts_without_a_style_take_the_cells() {
    assert_eq!(Cell::parts(["a", "b"]).text(), "ab");
    let mut list = two(Cell::parts([Part::new("a"), Part::new("b").faint()]).style(GREEN));
    let drawn = draw_with(&mut list, &TWO, 20, 2, |view| view.styles(STYLES));
    assert_eq!(drawn.marks[1], "..gf");
    let mut list = two(Cell::parts(["a", "b"]).faint());
    let drawn = draw_with(&mut list, &TWO, 20, 2, |view| view.styles(STYLES));
    assert_eq!(drawn.marks[1], "..ff");
    assert_eq!(Part::new("p").text(), "p");
    assert_eq!(Part::from("q"), Part::new("q"));
    assert_eq!(Part::from(String::from("r")), Part::new("r"));
    assert_eq!(Cell::parts(Vec::<Part>::new()).text(), "");
}

#[test]
fn parts_in_a_right_column_end_at_its_edge() {
    let columns = [
        Column::new("Name", 6, 6),
        Column::new("Size", 6, 6).right(),
        Column::new("Note", 1, 0),
    ];
    let mut list = List::new().with_rows([
        Row::new(["a", "1", "m"]),
        Row::new([
            Cell::new("b"),
            Cell::parts([Part::new("12"), Part::new(" kB").faint()]),
            Cell::new("m"),
        ]),
    ]);
    let drawn = draw_with(&mut list, &columns, 20, 2, |view| view.styles(STYLES));
    assert_eq!(drawn.text, ["▌ a            1  m", "  b        12 kB  m"]);
    assert_eq!(&drawn.marks[1][11..16], "..fff");
}

#[test]
fn a_rest_cell_spans_to_the_end_of_the_row() {
    let mut list = List::new().with_rows([
        Row::new(["ab", "12", "me", "first"]),
        Row::new([
            Cell::new("cd"),
            Cell::new("a description that runs across the remaining columns").rest(),
        ]),
        Row::new([
            Cell::new("ef"),
            Cell::new("34"),
            Cell::new("short").rest(),
            Cell::new("hidden"),
        ]),
        Row::new(["gh", "", "", "last"]),
    ]);
    let drawn = draw_with(&mut list, &WIDE, 40, 5, |view| {
        view.styles(STYLES).header(true)
    });
    assert_eq!(
        drawn.text,
        [
            "  Name      Size  Owner   Note",
            "▌ ab          12  me      first",
            "  cd      a description that runs…",
            "  ef          34  short",
            "  gh                      last",
        ]
    );
    assert_eq!(cells(&drawn.text[2]), 34);
}

#[test]
fn a_rest_cell_takes_its_columns_alignment_and_the_selected_style() {
    let mut list = List::new().with_rows([
        Row::new([
            Cell::new("gh"),
            Cell::parts([Part::new("the "), Part::new("end").faint()]).rest(),
        ]),
        Row::new([Cell::new("ij"), Cell::new("more").rest()]),
    ]);
    let drawn = draw_with(&mut list, &WIDE, 40, 2, |view| view.styles(STYLES));
    assert_eq!(drawn.text[0], format!("▌ gh{}the end", " ".repeat(29)));
    assert_eq!(drawn.marks[0], "S".repeat(40));
    assert_eq!(drawn.text[1], format!("  ij{}more", " ".repeat(32)));
    list.select(1);
    let drawn = draw_with(&mut list, &WIDE, 40, 2, |view| view.styles(STYLES));
    assert_eq!(drawn.marks[0], format!("{}fff", ".".repeat(37)));
}

#[test]
fn a_rest_cell_drops_with_its_column() {
    let columns = [
        Column::new("Name", 6, 6),
        Column::new("About", 6, 6),
        Column::new("Owner", 6, 6),
        Column::new("Note", 4, 0),
    ];
    let mut list = List::new().with_rows([Row::new([
        Cell::new("cd"),
        Cell::new("a description").rest(),
    ])]);
    let drawn = draw_with(&mut list, &columns, 20, 1, |view| view);
    assert_eq!(drawn.text, ["▌ cd"]);
    let pinned = [
        columns[0].pinned(),
        columns[1].pinned(),
        columns[2],
        columns[3],
    ];
    let drawn = draw_with(&mut list, &pinned, 20, 1, |view| view);
    assert_eq!(drawn.text, ["▌ cd      a…"]);
}

#[test]
fn a_flexible_column_can_come_before_number_columns() {
    let columns = [
        Column::new("Name", 6, 0).flex(),
        Column::new("Size", 4, 6).right(),
        Column::new("Count", 5, 5).right(),
    ];
    let mut list = List::new().with_rows([
        Row::new(["deploy api service", "12", "3"]),
        Row::new(["build", "1024", "12345"]),
    ]);
    let drawn = draw_with(&mut list, &columns, 30, 3, |view| {
        view.styles(STYLES).header(true)
    });
    assert_eq!(
        drawn.text,
        [
            "  Name             Size  Count",
            "▌ deploy api…        12      3",
            "  build            1024  12345",
        ]
    );
    let drawn = draw_with(&mut list, &columns, 20, 3, |view| {
        view.styles(STYLES).header(true)
    });
    assert_eq!(
        drawn.text,
        [
            "  Name          Size",
            "▌ deploy…         12",
            "  build         1024",
        ]
    );
    let drawn = draw_with(&mut list, &columns, 9, 3, |view| view.header(true));
    assert_eq!(drawn.text, ["  Name", "▌ deploy…", "  build"]);
}

#[test]
fn the_first_flexible_column_takes_whats_left() {
    let columns = [
        Column::new("", 3, 3),
        Column::new("", 3, 0).flex(),
        Column::new("", 3, 0).flex(),
    ];
    let mut list = List::new().with_rows([Row::new(["a", "b", "c"])]);
    let drawn = draw_with(&mut list, &columns, 20, 1, |view| view);
    assert_eq!(drawn.text, ["▌ a    b         c"]);
}

#[test]
fn a_flexible_last_column_draws_as_before() {
    let flexed = [
        COLUMNS[0],
        COLUMNS[1],
        COLUMNS[2],
        COLUMNS[3],
        COLUMNS[4].flex(),
    ];
    for width in [0u16, 10, 30, 40, 80, 150, 200] {
        let mut before = list_of(operations());
        let mut after = list_of(operations());
        let expected = draw_with(&mut before, &COLUMNS, width, 6, |view| {
            view.styles(STYLES).header(true).end(Some("· end ·"))
        });
        let drawn = draw_with(&mut after, &flexed, width, 6, |view| {
            view.styles(STYLES).header(true).end(Some("· end ·"))
        });
        assert_eq!(drawn.text, expected.text, "{width}");
        assert_eq!(drawn.marks, expected.marks, "{width}");
    }
}

#[test]
fn flexible_and_rest_cells_never_leave_the_area() {
    let huge = "ă日".repeat(10_000);
    let columns = [
        Column::new("a", 4, 9).priority(1),
        Column::new("b", 6, 0).flex().right(),
        Column::new("c", 10, 10).pinned().right(),
        Column::new("d", 3, 8).priority(2),
    ];
    let rows = [
        Row::new(["7", "42", "日", "x"]),
        Row::new([
            Cell::new(huge.as_str()),
            Cell::parts([huge.as_str(), huge.as_str()]).rest(),
        ]),
        Row::new([Cell::new(huge.as_str()).rest()]),
        Row::heading([huge.as_str(), huge.as_str()]),
    ];
    for width in 0..60u16 {
        for gap in [0u16, 2, u16::MAX] {
            let mut list = List::new().with_rows(rows.clone());
            list.select(1);
            let drawn = draw_with(&mut list, &columns, width, 5, |view| {
                view.header(true).gap(gap)
            });
            for line in &drawn.text {
                assert!(cells(line) <= usize::from(width), "{width} {gap}: {line:?}");
            }
        }
    }
}
