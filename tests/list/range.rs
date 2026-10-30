use std::borrow::Cow;

use crate::common::*;
use pito_tui::list::{Column, Key, List, ListView, Mark, Part, Row, Source, Step};
use ratatui::{buffer::Buffer, layout::Rect, style::Color, widgets::Widget};

const ONE: [Column<'static>; 1] = [Column::new("Name", 4, 0)];

fn numbered(count: usize) -> List {
    List::new().with_rows((0..count).map(|n| Row::new([format!("row {n}")])))
}

fn show(list: &mut List, height: u16) -> Drawn {
    draw_with(list, &ONE, 12, height, |view| {
        view.styles(STYLES).end(Some("· end ·"))
    })
}

fn lit(drawn: &Drawn) -> Vec<bool> {
    drawn
        .marks
        .iter()
        .map(|line| line == &"S".repeat(12))
        .collect()
}

#[test]
fn a_range_draws_its_rows_selected_with_the_marker_on_the_cursor_row_only() {
    let mut list = numbered(6);
    list.select(3);
    list.select_range(Some((1, 3)));
    let drawn = show(&mut list, 7);
    assert_eq!(
        drawn.text,
        [
            "  row 0",
            "  row 1",
            "  row 2",
            "▌ row 3",
            "  row 4",
            "  row 5",
            "  · end ·",
        ]
    );
    assert_eq!(lit(&drawn), [false, true, true, true, false, false, false]);
    assert_eq!(drawn.marks[0], "");
    assert_eq!(drawn.marks[4], "");
    assert_eq!(list.range(), Some((1, 3)));
    assert_eq!(list.selected(), Some(3));
}

#[test]
fn a_range_takes_its_ends_in_either_order_and_none_selects_one_row() {
    let mut list = numbered(6);
    list.select(1);
    list.select_range(Some((4, 1)));
    let drawn = show(&mut list, 7);
    assert_eq!(lit(&drawn), [false, true, true, true, true, false, false]);
    assert_eq!(drawn.text[1], "▌ row 1");
    assert_eq!(drawn.text[4], "  row 4");
    list.select_range(None);
    assert_eq!(list.range(), None);
    let drawn = show(&mut list, 7);
    assert_eq!(
        lit(&drawn),
        [false, true, false, false, false, false, false]
    );
}

#[test]
fn the_marker_cells_of_a_range_row_take_the_selected_style() {
    let mut list = List::new().with_rows([
        Row::new(["a"]).mark(Mark::new("⧗").style(GREEN)),
        Row::new(["b"]).mark(Mark::new("✓").style(GREEN)),
        Row::new(["c"]).mark(Mark::new("✗").style(GREEN)),
    ]);
    list.select_range(Some((0, 1)));
    let red = SELECTED.bg(Color::Red);
    let area = Rect::new(0, 0, 12, 3);
    let mut buf = Buffer::empty(area);
    ListView::new(&mut list, &ONE)
        .styles(STYLES.selected(red))
        .render(area, &mut buf);
    assert_eq!(read(&buf).text, ["▌ ⧗ a", "  ✓ b", "  ✗ c"]);
    for y in 0..2 {
        for x in 0..12 {
            assert_eq!(buf[(x, y)].bg, Color::Red, "{x},{y}");
            assert_eq!(buf[(x, y)].fg, Color::Magenta, "{x},{y}");
        }
    }
    assert_eq!(buf[(0, 1)].symbol(), " ");
    assert_eq!(buf[(2, 2)].fg, Color::Green);
    assert_eq!(buf[(0, 2)].bg, Color::Reset);
}

#[test]
fn sections_inside_a_range_stay_unselected() {
    let mut list = List::new().with_rows([
        Row::new(["one"]),
        Row::blank(),
        Row::heading([Part::new("Done").style(GREEN), Part::new(" · 2").faint()]),
        Row::new(["two"]),
        Row::new(["three"]),
    ]);
    list.select(4);
    list.select_range(Some((0, 4)));
    let drawn = show(&mut list, 6);
    assert_eq!(
        drawn.text,
        ["  one", "", "  Done · 2", "  two", "▌ three", "  · end ·"]
    );
    assert_eq!(lit(&drawn), [true, false, false, true, true, false]);
    assert_eq!(drawn.marks[1], "");
    assert_eq!(drawn.marks[2], "..ggggffff");
    assert_eq!(list.hit(Rect::new(0, 0, 12, 6), false, 3, 2), None);
}

#[test]
fn the_range_is_kept_as_is_when_the_rows_are_replaced() {
    let mut list = numbered(6);
    list.select(4);
    list.select_range(Some((2, 4)));
    list.set_rows((0..3).map(|n| Row::new([format!("new {n}")])));
    assert_eq!(list.range(), Some((2, 4)));
    assert_eq!(list.selected(), Some(2));
    let drawn = show(&mut list, 4);
    assert_eq!(lit(&drawn), [false, false, true, false]);
    list.set_rows((0..6).map(|n| Row::new([format!("again {n}")])));
    assert_eq!(list.range(), Some((2, 4)));
    let drawn = show(&mut list, 7);
    assert_eq!(lit(&drawn), [false, false, true, true, true, false, false]);
    assert_eq!(drawn.text[2], "▌ again 2");
    list.clear();
    assert_eq!(list.range(), Some((2, 4)));
}

#[test]
fn a_range_past_the_end_lights_only_the_rows_there_are() {
    let mut list = numbered(4);
    list.select_range(Some((2, usize::MAX)));
    let drawn = show(&mut list, 6);
    assert_eq!(lit(&drawn), [true, false, true, true, false, false]);
    assert_eq!(drawn.text[4], "  · end ·");
    assert_eq!(drawn.marks[4].trim_start_matches('.'), "fffffff");
    let mut empty = List::new();
    empty.select_range(Some((0, 9)));
    let drawn = draw_with(&mut empty, &ONE, 12, 2, |view| {
        view.styles(STYLES).empty(Some("Nothing."))
    });
    assert_eq!(drawn.text, ["  Nothing.", ""]);
}

#[test]
fn the_keys_move_the_cursor_and_leave_the_range_to_the_app() {
    let mut list = numbered(6);
    list.select_range(Some((0, 2)));
    assert_eq!(list.key(Key::Down), Step::Moved);
    assert_eq!(list.range(), Some((0, 2)));
    let anchor = 0;
    list.down();
    list.down();
    list.select_range(list.selected().map(|at| (anchor, at)));
    let drawn = show(&mut list, 7);
    assert_eq!(lit(&drawn), [true, true, true, true, false, false, false]);
    assert_eq!(drawn.text[3], "▌ row 3");
}

struct Lines(usize);

impl Source for Lines {
    fn len(&self) -> usize {
        self.0
    }

    fn row(&self, index: usize) -> Cow<'_, Row> {
        Cow::Owned(Row::new([format!("row {index}")]))
    }
}

#[test]
fn a_lazy_list_draws_its_range_like_the_same_rows_owned() {
    let mut owned = numbered(40);
    let mut lazy = List::from_source(Lines(40));
    for range in [Some((5, 12)), Some((30, 20)), None] {
        owned.select(12);
        lazy.select(12);
        owned.select_range(range);
        lazy.select_range(range);
        let area = Rect::new(0, 0, 12, 10);
        let mut want = Buffer::empty(area);
        let mut got = Buffer::empty(area);
        ListView::new(&mut owned, &ONE)
            .styles(STYLES)
            .render(area, &mut want);
        ListView::new(&mut lazy, &ONE)
            .styles(STYLES)
            .render(area, &mut got);
        assert_eq!(got, want, "{range:?}");
    }
    lazy.select_range(Some((1, 2)));
    lazy.update(|lines| lines.0 = 3);
    assert_eq!(lazy.range(), Some((1, 2)));
}
