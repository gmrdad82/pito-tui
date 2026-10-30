use crate::common::*;
use pito_tui::list::{Column, Key, List, Paging, Row, Step};
use ratatui::layout::Rect;

const ONE: [Column<'static>; 1] = [Column::new("", 6, 6)];

fn numbered(count: usize) -> List {
    List::new().with_rows((0..count).map(|n| Row::new([format!("row {n}")])))
}

fn shown(list: &mut List, height: u16) -> Vec<String> {
    draw_with(list, &ONE, 20, height, |view| {
        view.styles(STYLES).end(Some("· end ·"))
    })
    .text
}

fn line_of_selection(list: &mut List, height: u16) -> usize {
    let text = shown(list, height);
    let marker = format!("▌ row {}", list.selected().unwrap());
    text.iter().position(|line| *line == marker).unwrap()
}

#[test]
fn the_selected_row_wins_over_the_end_line_when_one_line_is_left() {
    for count in [1usize, 2, 5, 40] {
        let mut list = numbered(count);
        list.last();
        let last = count - 1;
        assert_eq!(shown(&mut list, 1), [format!("▌ row {last}")], "{count}");
        assert_eq!(list.top(), last);
        assert_eq!(list.hit(Rect::new(0, 0, 20, 1), false, 3, 0), Some(last));
        let drawn = draw_with(&mut list, &ONE, 20, 2, |view| {
            view.styles(STYLES).header(true).end(Some("· end ·"))
        });
        assert_eq!(
            drawn.text,
            ["", format!("▌ row {last}").as_str()],
            "{count}"
        );
        assert_eq!(
            shown(&mut list, 2),
            [format!("▌ row {last}"), "  · end ·".to_string()],
            "{count}"
        );
    }
}

#[test]
fn view_paging_keeps_the_selection_on_its_line() {
    let mut list = numbered(100).paging(Paging::View);
    shown(&mut list, 10);
    assert_eq!(list.page(), 9);
    for _ in 0..4 {
        list.down();
    }
    assert_eq!(line_of_selection(&mut list, 10), 4);
    assert_eq!(list.page_down(), Step::Moved);
    assert_eq!(list.selected(), Some(13));
    assert_eq!(list.top(), 9);
    assert_eq!(line_of_selection(&mut list, 10), 4);
    assert_eq!(list.key(Key::PageDown), Step::Moved);
    assert_eq!(line_of_selection(&mut list, 10), 4);
    assert_eq!(list.top(), 18);
    assert_eq!(list.key(Key::PageUp), Step::Moved);
    assert_eq!(list.page_up(), Step::Moved);
    assert_eq!(list.selected(), Some(4));
    assert_eq!(line_of_selection(&mut list, 10), 4);
    assert_eq!(list.page_up(), Step::Moved);
    assert_eq!(list.selected(), Some(0));
    assert_eq!(line_of_selection(&mut list, 10), 0);
    assert_eq!(list.page_up(), Step::Held);
}

#[test]
fn view_paging_stops_at_the_end_of_the_list() {
    let mut list = numbered(100).paging(Paging::View);
    list.select(95);
    assert_eq!(line_of_selection(&mut list, 10), 9);
    assert_eq!(list.page_down(), Step::Moved);
    assert_eq!(list.selected(), Some(99));
    let text = shown(&mut list, 10);
    assert_eq!(text[8], "▌ row 99");
    assert_eq!(text[9], "  · end ·");
    assert_eq!(list.page_down(), Step::Held);
}

#[test]
fn cursor_paging_still_moves_the_view_only_to_follow() {
    let mut list = numbered(100);
    shown(&mut list, 10);
    for _ in 0..4 {
        list.down();
    }
    list.page_down();
    assert_eq!(list.selected(), Some(13));
    assert_eq!(line_of_selection(&mut list, 10), 9);
    assert_eq!(Paging::default(), Paging::Cursor);
}

#[test]
fn scroll_to_moves_the_view_and_the_selection_stays_in_it() {
    let mut list = numbered(100);
    shown(&mut list, 10);
    list.select(25);
    list.scroll_to(20);
    assert_eq!(list.top(), 20);
    assert_eq!(line_of_selection(&mut list, 10), 5);
    assert_eq!(list.top(), 20);
    list.scroll_to(50);
    assert_eq!(line_of_selection(&mut list, 10), 0);
    assert_eq!(list.top(), 25);
    list.scroll_to(0);
    assert_eq!(line_of_selection(&mut list, 10), 9);
    assert_eq!(list.top(), 16);
    list.scroll_to(usize::MAX);
    assert_eq!(list.top(), 99);
    list.last();
    shown(&mut list, 10);
    assert_eq!(list.top(), 91);
    assert_eq!(list.hit(Rect::new(0, 0, 20, 10), false, 0, 0), Some(91));
    List::new().scroll_to(5);
}
