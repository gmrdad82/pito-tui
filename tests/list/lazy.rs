use std::{borrow::Cow, cell::Cell as Counter};

use crate::common::*;
use pito_tui::list::{Column, Key, List, ListView, Mark, Part, Row, Source, Step};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Modifier, Style},
    widgets::Widget,
};

const BOLD: Style = Style::new().add_modifier(Modifier::BOLD);
const TWO: [Column<'static>; 2] = [Column::new("Entry", 8, 12), Column::new("State", 4, 0)];

fn entry(index: usize) -> Row {
    if index.is_multiple_of(10) {
        Row::heading([
            Part::new(format!("Hour {}", index / 10)).style(BOLD),
            Part::new(" · 9").faint(),
        ])
    } else {
        Row::new([format!("entry {index}"), "ok".to_string()]).mark(Mark::new("•").style(GREEN))
    }
}

struct Log {
    len: usize,
    built: Counter<usize>,
}

impl Log {
    fn new(len: usize) -> Self {
        Log {
            len,
            built: Counter::new(0),
        }
    }
}

impl Source for Log {
    fn len(&self) -> usize {
        self.len
    }

    fn row(&self, index: usize) -> Cow<'_, Row> {
        self.built.set(self.built.get() + 1);
        Cow::Owned(entry(index))
    }

    fn selectable(&self, index: usize) -> bool {
        !index.is_multiple_of(10)
    }

    fn mark_width(&self) -> u16 {
        1
    }
}

struct Plain(usize);

impl Source for Plain {
    fn len(&self) -> usize {
        self.0
    }

    fn row(&self, index: usize) -> Cow<'_, Row> {
        Cow::Owned(entry(index))
    }
}

fn render<S: Source>(list: &mut List<S>, width: u16, height: u16) -> Drawn {
    let area = Rect::new(0, 0, width, height);
    let mut buf = Buffer::empty(area);
    ListView::new(list, &TWO)
        .styles(STYLES)
        .header(true)
        .end(Some("· end ·"))
        .empty(Some("Nothing yet."))
        .render(area, &mut buf);
    read(&buf)
}

#[test]
fn a_lazy_list_draws_like_the_same_rows_owned() {
    let mut owned = List::new().with_rows((0..200).map(entry));
    let mut lazy = List::from_source(Log::new(200));
    let keys = [
        Key::Down,
        Key::PageDown,
        Key::Down,
        Key::End,
        Key::Up,
        Key::PageUp,
        Key::PageUp,
        Key::Home,
    ];
    for key in keys {
        assert_eq!(owned.key(key), lazy.key(key));
        for (width, height) in [(40, 12), (24, 4), (12, 2), (60, 30)] {
            let expected = render(&mut owned, width, height);
            let drawn = render(&mut lazy, width, height);
            assert_eq!(drawn.text, expected.text, "{key:?} {width}x{height}");
            assert_eq!(drawn.marks, expected.marks, "{key:?} {width}x{height}");
            assert_eq!(lazy.top(), owned.top());
            assert_eq!(lazy.selected(), owned.selected());
        }
    }
}

#[test]
fn only_the_rows_on_screen_are_built() {
    let mut list = List::from_source(Log::new(50_000));
    for key in [Key::End, Key::PageUp, Key::Up, Key::Down, Key::Home] {
        list.key(key);
    }
    assert_eq!(list.source().built.get(), 0);
    list.select(49_000);
    for _ in 0..3 {
        render(&mut list, 60, 12);
        let built = list.source().built.replace(0);
        assert!((11..=22).contains(&built), "{built}");
        list.key(Key::PageDown);
    }
}

#[test]
fn sections_in_a_lazy_list_are_skipped() {
    let mut list = List::from_source(Log::new(30));
    assert_eq!(list.selected(), Some(1));
    assert_eq!(list.up(), Step::Held);
    for _ in 0..8 {
        list.down();
    }
    assert_eq!(list.selected(), Some(9));
    assert_eq!(list.down(), Step::Moved);
    assert_eq!(list.selected(), Some(11));
    list.select(20);
    assert_eq!(list.selected(), Some(21));
    let drawn = render(&mut list, 40, 6);
    assert_eq!(drawn.text[4], "  Hour 2 · 9");
    assert_eq!(drawn.text[5], "▌ • entry 21      ok");
    let area = Rect::new(0, 0, 40, 6);
    assert_eq!(list.hit(area, true, 3, 4), None);
    assert_eq!(list.hit(area, true, 3, 5), Some(21));
    assert_eq!(list.last(), Step::Moved);
    assert_eq!(list.selected(), Some(29));
}

#[test]
fn a_source_without_its_own_answers_reads_them_from_its_rows() {
    let mut list = List::from_source(Plain(25));
    assert_eq!(list.selected(), Some(1));
    list.select(10);
    assert_eq!(list.selected(), Some(11));
    assert_eq!(list.key(Key::Up), Step::Moved);
    assert_eq!(list.selected(), Some(9));
    assert!(!list.source().is_empty());
    let drawn = render(&mut list, 30, 4);
    assert!(drawn.text.iter().all(|line| !line.contains('•')));
    assert!(drawn.text.contains(&"▌ entry 9       ok".to_string()));
}

#[test]
fn updating_the_source_clamps_the_selection() {
    let mut list = List::from_source(Log::new(100));
    list.select(95);
    list.update(|log| log.len = 30);
    assert_eq!(list.selected(), Some(29));
    list.set_source(Log::new(21));
    assert_eq!(list.selected(), Some(19));
    assert_eq!(list.len(), 21);
    list.set_source(Log::new(0));
    assert_eq!(list.selected(), None);
    assert!(list.is_empty());
    let drawn = render(&mut list, 30, 3);
    assert_eq!(drawn.text[1], "  Nothing yet.");
}

#[test]
fn owned_rows_are_a_source_too() {
    let rows: Vec<Row> = (0..12).map(entry).collect();
    let mut built = List::new().with_rows(rows.clone());
    let mut sourced = List::from_source(rows);
    assert_eq!(
        render(&mut sourced, 40, 8).text,
        render(&mut built, 40, 8).text
    );
    sourced.update(|rows| rows.push(Row::new(["wide"]).mark(Mark::new("⧗⧗"))));
    sourced.last();
    let drawn = render(&mut sourced, 40, 8);
    assert_eq!(drawn.text[6], "▌ ⧗⧗ wide");
    assert_eq!(drawn.text[5], "  •  entry 11      ok");
    assert_eq!(sourced.rows().len(), 13);
    assert_eq!(sourced.source().len(), 13);
}
