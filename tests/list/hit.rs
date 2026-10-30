use crate::common::*;
use pito_tui::list::{List, Row};
use ratatui::layout::Rect;

fn numbered(count: usize) -> List {
    List::new().with_rows((0..count).map(|n| Row::new([format!("row {n}")])))
}

#[test]
fn a_click_names_the_row_under_it() {
    let mut list = numbered(20);
    draw(&mut list, 40, 6);
    let area = Rect::new(0, 0, 40, 6);
    assert_eq!(list.hit(area, true, 5, 0), None);
    assert_eq!(list.hit(area, true, 5, 1), Some(0));
    assert_eq!(list.hit(area, true, 39, 5), Some(4));
    assert_eq!(list.hit(area, true, 40, 3), None);
    assert_eq!(list.hit(area, true, 5, 6), None);
}

#[test]
fn a_click_without_a_header_starts_on_the_first_line() {
    let list = numbered(20);
    let area = Rect::new(0, 0, 40, 6);
    assert_eq!(list.hit(area, false, 0, 0), Some(0));
    assert_eq!(list.hit(area, false, 0, 5), Some(5));
}

#[test]
fn hits_follow_the_scroll_and_the_area_origin() {
    let mut list = numbered(40);
    list.last();
    draw(&mut list, 40, 8);
    let top = list.top();
    let area = Rect::new(10, 4, 30, 8);
    assert_eq!(list.hit(area, true, 10, 5), Some(top));
    assert_eq!(list.hit(area, true, 9, 5), None);
    assert_eq!(list.hit(area, true, 39, 10), Some(top + 5));
    assert_eq!(list.hit(area, true, 39, 11), None);
    assert_eq!(list.hit(area, true, 40, 10), None);
    assert_eq!(list.hit(area, true, 10, 3), None);
}

#[test]
fn the_end_tail_and_blank_lines_are_not_rows() {
    let mut list = numbered(2);
    draw(&mut list, 40, 8);
    let area = Rect::new(0, 0, 40, 8);
    assert_eq!(list.hit(area, true, 3, 2), Some(1));
    assert_eq!(list.hit(area, true, 3, 3), None);
    assert_eq!(list.hit(area, true, 3, 6), None);
}

#[test]
fn an_empty_list_has_no_hits() {
    let list = List::new();
    assert_eq!(list.hit(Rect::new(0, 0, 40, 8), true, 3, 3), None);
    assert_eq!(list.hit(Rect::new(0, 0, 0, 0), false, 0, 0), None);
}

#[test]
fn hit_never_overflows_at_the_edge_of_the_grid() {
    let mut list = numbered(3);
    list.select(2);
    let area = Rect::new(u16::MAX - 5, u16::MAX - 5, 5, 5);
    assert_eq!(list.hit(area, true, u16::MAX - 1, u16::MAX - 1), None);
    assert_eq!(list.hit(area, false, u16::MAX - 1, u16::MAX - 5), Some(0));
}
