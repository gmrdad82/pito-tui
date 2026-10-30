use crate::common::*;
use pito_tui::list::{Cell, Column, Key, List, Mark, Part, Row, Step};
use ratatui::{
    layout::Rect,
    style::{Modifier, Style},
};

const BOLD: Style = Style::new().add_modifier(Modifier::BOLD);
const ONE: [Column<'static>; 1] = [Column::new("Name", 4, 0)];

fn heading(title: &str, count: usize) -> Row {
    Row::heading([
        Part::new(title).style(BOLD),
        Part::new(format!(" · {count}")).faint(),
    ])
}

fn groups() -> List {
    List::new().with_rows([
        heading("Running", 2),
        Row::new(["deploy"]),
        Row::new(["build"]),
        Row::blank(),
        heading("Done", 2),
        Row::new(["update"]),
        Row::new(["release"]),
    ])
}

fn three() -> List {
    let mut rows = Vec::new();
    for group in 0..3 {
        if group > 0 {
            rows.push(Row::blank());
        }
        rows.push(heading(&format!("Group {}", group + 1), 5));
        for _ in 0..5 {
            rows.push(Row::new([format!("item {}", rows.len())]));
        }
    }
    List::new().with_rows(rows)
}

fn show(list: &mut List, width: u16, height: u16) -> Drawn {
    draw_with(list, &ONE, width, height, |view| {
        view.styles(STYLES).end(Some("· end ·"))
    })
}

fn plain(list: &mut List, height: u16) -> Vec<String> {
    draw_with(list, &ONE, 24, height, |view| view.styles(STYLES)).text
}

#[test]
fn a_heading_and_a_blank_draw_in_their_own_styles() {
    let mut list = groups();
    let drawn = show(&mut list, 24, 8);
    assert_eq!(
        drawn.text,
        [
            "  Running · 2",
            "▌ deploy",
            "  build",
            "",
            "  Done · 2",
            "  update",
            "  release",
            "  · end ·",
        ]
    );
    assert_eq!(drawn.marks[0], "..SSSSSSSffff");
    assert_eq!(drawn.marks[1], "S".repeat(24));
    assert_eq!(drawn.marks[3], "");
    assert_eq!(drawn.marks[4], "..SSSSffff");
}

#[test]
fn a_heading_spans_the_row_from_the_marks_and_clips_as_one() {
    let mut list = List::new().with_rows([
        heading("Running", 2),
        Row::new(["deploy"]).mark(Mark::new("⧗")),
    ]);
    let drawn = show(&mut list, 24, 3);
    assert_eq!(drawn.text, ["  Running · 2", "▌ ⧗ deploy", "  · end ·"]);
    let drawn = show(&mut list, 10, 3);
    assert_eq!(drawn.text[0], "  Running…");
    assert_eq!(drawn.marks[0], "..SSSSSSSf");
    let columns = [
        Column::new("A", 3, 3),
        Column::new("B", 3, 3),
        Column::new("C", 1, 0),
    ];
    let mut list = List::new().with_rows([
        Row::heading(["a heading wider than any column"]),
        Row::new(["x", "y", "z"]),
    ]);
    let drawn = draw_with(&mut list, &columns, 30, 2, |view| view);
    assert_eq!(drawn.text, ["  a heading wider than any…", "▌ x    y    z"]);
}

#[test]
fn the_cursor_skips_sections() {
    let mut list = groups();
    assert_eq!(list.selected(), Some(1));
    assert_eq!(list.up(), Step::Held);
    assert_eq!(list.down(), Step::Moved);
    assert_eq!(list.selected(), Some(2));
    assert_eq!(list.down(), Step::Moved);
    assert_eq!(list.selected(), Some(5));
    assert_eq!(list.key(Key::Down), Step::Moved);
    assert_eq!(list.key(Key::Down), Step::Held);
    assert_eq!(list.selected(), Some(6));
    assert_eq!(list.key(Key::Enter), Step::Open(6));
    assert_eq!(list.up(), Step::Moved);
    assert_eq!(list.up(), Step::Moved);
    assert_eq!(list.selected(), Some(2));
    assert_eq!(list.key(Key::End), Step::Moved);
    assert_eq!(list.selected(), Some(6));
    assert_eq!(list.key(Key::Home), Step::Moved);
    assert_eq!(list.selected(), Some(1));
    assert_eq!(list.first(), Step::Held);
    list.select(0);
    assert_eq!(list.selected(), Some(1));
    list.select(3);
    assert_eq!(list.selected(), Some(5));
    list.select(4);
    assert_eq!(list.selected(), Some(5));
    assert_eq!(list.selected_row(), Some(&Row::new(["update"])));
}

#[test]
fn paging_skips_sections() {
    let mut list = three();
    plain(&mut list, 7);
    assert_eq!(list.page(), 6);
    let mut seen = Vec::new();
    while list.page_down() == Step::Moved {
        seen.push(list.selected().unwrap());
    }
    assert_eq!(seen, [8, 15, 19]);
    seen.clear();
    while list.key(Key::PageUp) == Step::Moved {
        seen.push(list.selected().unwrap());
    }
    assert_eq!(seen, [12, 5, 1]);
}

#[test]
fn following_up_keeps_the_groups_heading_in_view() {
    let mut list = three();
    list.last();
    assert_eq!(
        plain(&mut list, 4),
        ["  item 16", "  item 17", "  item 18", "▌ item 19"]
    );
    for _ in 0..4 {
        list.up();
    }
    assert_eq!(
        plain(&mut list, 4),
        ["  Group 3 · 5", "▌ item 15", "  item 16", "  item 17"]
    );
    list.up();
    assert_eq!(
        plain(&mut list, 4),
        ["▌ item 12", "", "  Group 3 · 5", "  item 15"]
    );
    for _ in 0..4 {
        list.up();
    }
    assert_eq!(
        plain(&mut list, 4),
        ["  Group 2 · 5", "▌ item 8", "  item 9", "  item 10"]
    );
    list.first();
    assert_eq!(
        plain(&mut list, 4),
        ["  Group 1 · 5", "▌ item 1", "  item 2", "  item 3"]
    );
    assert_eq!(list.top(), 0);
}

#[test]
fn sections_scroll_with_the_rows() {
    let mut list = three();
    plain(&mut list, 4);
    for _ in 0..3 {
        list.down();
    }
    assert_eq!(
        plain(&mut list, 4),
        ["  item 1", "  item 2", "  item 3", "▌ item 4"]
    );
    list.down();
    list.down();
    assert_eq!(
        plain(&mut list, 4),
        ["  item 5", "", "  Group 2 · 5", "▌ item 8"]
    );
}

#[test]
fn the_selected_row_wins_over_its_heading_when_one_line_is_left() {
    let mut list = three();
    list.select(8);
    assert_eq!(plain(&mut list, 1), ["▌ item 8"]);
    list.select(1);
    assert_eq!(plain(&mut list, 1), ["▌ item 1"]);
    assert_eq!(plain(&mut list, 2), ["  Group 1 · 5", "▌ item 1"]);
}

#[test]
fn hits_skip_sections() {
    let mut list = groups();
    show(&mut list, 24, 8);
    let area = Rect::new(0, 0, 24, 8);
    assert_eq!(list.hit(area, false, 3, 0), None);
    assert_eq!(list.hit(area, false, 3, 1), Some(1));
    assert_eq!(list.hit(area, false, 3, 2), Some(2));
    assert_eq!(list.hit(area, false, 3, 3), None);
    assert_eq!(list.hit(area, false, 3, 4), None);
    assert_eq!(list.hit(area, false, 3, 5), Some(5));
    assert_eq!(list.hit(area, false, 3, 7), None);
}

#[test]
fn a_list_of_sections_only_has_no_selection() {
    let mut list = List::new().with_rows([heading("Nothing", 0), Row::blank()]);
    assert!(!list.is_empty());
    assert_eq!(list.selected(), None);
    assert_eq!(list.selected_row(), None);
    for key in [Key::Down, Key::Up, Key::PageDown, Key::End, Key::Enter] {
        assert_eq!(list.key(key), Step::Held);
    }
    let drawn = show(&mut list, 24, 4);
    assert_eq!(drawn.text, ["  Nothing · 0", "", "  · end ·", ""]);
    assert_eq!(list.hit(Rect::new(0, 0, 24, 4), false, 3, 0), None);
}

#[test]
fn replacing_the_rows_moves_the_selection_off_a_section() {
    let mut list = List::new().with_rows((0..10).map(|n| Row::new([format!("row {n}")])));
    list.select(8);
    list.set_rows([Row::new(["a"]), Row::new(["b"]), Row::blank()]);
    assert_eq!(list.selected(), Some(1));
    list.set_rows([heading("Later", 1), Row::new(["c"])]);
    assert_eq!(list.selected(), Some(1));
}

#[test]
fn rows_say_whether_they_are_selectable() {
    assert!(Row::new(["a"]).selectable());
    assert!(!Row::heading(["a"]).selectable());
    assert!(!Row::blank().selectable());
    assert!(Row::blank().cells().is_empty());
    assert_eq!(heading("Done", 3).cells()[0].text(), "Done · 3");
    assert_eq!(
        Row::heading([Part::new("x")]).cells(),
        [Cell::parts([Part::new("x")])]
    );
}

#[test]
fn a_trailing_section_sends_select_and_paging_back_to_the_last_item() {
    let mut list = List::new().with_rows([
        heading("Running", 2),
        Row::new(["deploy"]),
        Row::new(["build"]),
        Row::blank(),
    ]);
    assert_eq!(list.position(), Some((2, 4)));
    list.select(99);
    assert_eq!(list.selected(), Some(2));
    assert_eq!(list.position(), Some((3, 4)));
    list.select(3);
    assert_eq!(list.selected(), Some(2));
    list.select(1);
    assert_eq!(list.page_down(), Step::Moved);
    assert_eq!(list.selected(), Some(2));
    assert_eq!(list.page_down(), Step::Held);
    assert_eq!(list.selected(), Some(2));
}

#[test]
fn a_list_with_no_selection_has_no_position() {
    assert_eq!(List::new().position(), None);
    let sections = List::new().with_rows([heading("Nothing", 0), Row::blank()]);
    assert_eq!(sections.position(), None);
}
