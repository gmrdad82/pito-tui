use crate::common::*;
use pito_tui::list::{Column, Key, Keys, List, Paging, Row, Step};

const ONE: [Column<'static>; 1] = [Column::new("", 6, 6)];
const SETS: [Keys; 2] = [Keys::new(), Keys::VIM];
const HALF_DOWN: Key = Key::Ctrl('d');
const HALF_UP: Key = Key::Ctrl('u');

fn numbered(count: usize) -> List {
    List::new().with_rows((0..count).map(|n| Row::new([format!("row {n}")])))
}

fn shown(list: &mut List, height: u16) -> Vec<String> {
    draw_with(list, &ONE, 20, height, |view| view.styles(STYLES)).text
}

fn ended(list: &mut List, height: u16) -> Vec<String> {
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

fn landing(list: &mut List, height: u16, key: Key, step: Step, row: usize) {
    assert_eq!(list.key(key), step, "{key:?}");
    assert_eq!(list.selected(), Some(row), "{key:?}");
    assert!(
        line_of_selection(list, height) < usize::from(height),
        "{key:?}"
    );
}

fn at(keys: Keys, count: usize, height: u16, row: usize) -> List {
    let mut list = numbered(count).keys(keys);
    shown(&mut list, height);
    list.select(row);
    shown(&mut list, height);
    list
}

#[test]
fn the_built_in_sets_split_half_pages_from_whole_ones() {
    let keys = Keys::new();
    assert_eq!(keys.page_up, &[Key::PageUp]);
    assert_eq!(keys.page_down, &[Key::PageDown]);
    assert_eq!(keys.half_up, &[Key::Ctrl('u')]);
    assert_eq!(keys.half_down, &[Key::Ctrl('d')]);
    assert_eq!(Keys::default(), Keys::new());
    assert_eq!(Keys::VIM.page_up, &[Key::PageUp]);
    assert_eq!(Keys::VIM.page_down, &[Key::PageDown]);
    assert_eq!(Keys::VIM.half_up, &[Key::Ctrl('u')]);
    assert_eq!(Keys::VIM.half_down, &[Key::Ctrl('d')]);
}

#[test]
fn at_the_top_the_down_keys_move_and_the_up_keys_hold() {
    for keys in SETS {
        let mut list = at(keys, 100, 10, 0);
        assert_eq!(list.page(), 9);
        landing(&mut list, 10, Key::PageUp, Step::Held, 0);
        landing(&mut list, 10, HALF_UP, Step::Held, 0);
        landing(&mut list, 10, HALF_DOWN, Step::Moved, 5);
        landing(&mut list, 10, Key::PageUp, Step::Moved, 0);
        landing(&mut list, 10, Key::PageDown, Step::Moved, 9);
        landing(&mut list, 10, HALF_UP, Step::Moved, 4);
        landing(&mut list, 10, HALF_UP, Step::Moved, 0);
    }
}

#[test]
fn in_the_middle_each_key_lands_on_its_row() {
    for keys in SETS {
        landing(
            &mut at(keys, 100, 10, 50),
            10,
            Key::PageDown,
            Step::Moved,
            59,
        );
        landing(&mut at(keys, 100, 10, 50), 10, Key::PageUp, Step::Moved, 41);
        landing(&mut at(keys, 100, 10, 50), 10, HALF_DOWN, Step::Moved, 55);
        landing(&mut at(keys, 100, 10, 50), 10, HALF_UP, Step::Moved, 45);
    }
}

#[test]
fn at_the_bottom_the_down_keys_clamp_without_wrapping() {
    for keys in SETS {
        let mut list = at(keys, 100, 10, 99);
        landing(&mut list, 10, Key::PageDown, Step::Held, 99);
        landing(&mut list, 10, HALF_DOWN, Step::Held, 99);
        landing(&mut list, 10, HALF_UP, Step::Moved, 94);
        landing(&mut list, 10, HALF_DOWN, Step::Moved, 99);
        landing(&mut list, 10, HALF_DOWN, Step::Held, 99);
        let mut near = at(keys, 100, 10, 96);
        landing(&mut near, 10, HALF_DOWN, Step::Moved, 99);
        landing(&mut near, 10, Key::PageDown, Step::Held, 99);
        let mut near = at(keys, 100, 10, 92);
        landing(&mut near, 10, Key::PageDown, Step::Moved, 99);
        landing(&mut near, 10, HALF_DOWN, Step::Held, 99);
    }
}

#[test]
fn near_the_top_the_up_keys_clamp_without_wrapping() {
    for keys in SETS {
        let mut list = at(keys, 100, 10, 3);
        landing(&mut list, 10, HALF_UP, Step::Moved, 0);
        landing(&mut list, 10, HALF_UP, Step::Held, 0);
        landing(&mut list, 10, Key::PageUp, Step::Held, 0);
        let mut list = at(keys, 100, 10, 8);
        landing(&mut list, 10, Key::PageUp, Step::Moved, 0);
        landing(&mut list, 10, Key::PageUp, Step::Held, 0);
    }
}

#[test]
fn a_walk_down_and_back_keeps_the_selection_in_view() {
    for keys in SETS {
        let mut list = at(keys, 100, 10, 0);
        let mut seen = vec![0];
        for _ in 0..20 {
            if list.key(HALF_DOWN) == Step::Held {
                break;
            }
            assert!(line_of_selection(&mut list, 10) < 10);
            seen.push(list.selected().unwrap());
        }
        assert_eq!(seen.len(), 21);
        assert_eq!(list.selected(), Some(99));
        assert!(
            seen.windows(2)
                .all(|pair| pair[1] - pair[0] == 5 || pair[1] == 99)
        );
        for _ in 0..30 {
            if list.key(Key::PageUp) == Step::Held {
                break;
            }
            assert!(line_of_selection(&mut list, 10) < 10);
        }
        assert_eq!(list.selected(), Some(0));
    }
}

#[test]
fn a_list_shorter_than_a_page_clamps_to_its_ends_then_holds() {
    for keys in SETS {
        for down in [Key::PageDown, HALF_DOWN] {
            let mut list = at(keys, 4, 10, 1);
            landing(&mut list, 10, down, Step::Moved, 3);
            landing(&mut list, 10, down, Step::Held, 3);
        }
        for up in [Key::PageUp, HALF_UP] {
            let mut list = at(keys, 4, 10, 2);
            landing(&mut list, 10, up, Step::Moved, 0);
            landing(&mut list, 10, up, Step::Held, 0);
        }
        let mut one = at(keys, 1, 10, 0);
        for key in [Key::PageDown, Key::PageUp, HALF_DOWN, HALF_UP] {
            landing(&mut one, 10, key, Step::Held, 0);
        }
    }
}

#[test]
fn an_empty_list_holds_and_selects_nothing() {
    for keys in SETS {
        let mut list = numbered(0).keys(keys);
        shown(&mut list, 10);
        for key in [Key::PageDown, Key::PageUp, HALF_DOWN, HALF_UP] {
            assert_eq!(list.key(key), Step::Held, "{key:?}");
            assert_eq!(list.selected(), None, "{key:?}");
        }
        assert_eq!(list.half_down(), Step::Held);
        assert_eq!(list.half_up(), Step::Held);
        assert_eq!(list.top(), 0);
    }
}

#[test]
fn a_list_with_nothing_selectable_holds() {
    let mut list = List::new().with_rows([Row::heading(["a"]), Row::blank(), Row::heading(["b"])]);
    shown(&mut list, 10);
    for key in [Key::PageDown, Key::PageUp, HALF_DOWN, HALF_UP] {
        assert_eq!(list.key(key), Step::Held, "{key:?}");
        assert_eq!(list.selected(), None, "{key:?}");
    }
}

#[test]
fn a_one_line_view_moves_half_a_page_by_one_row() {
    for keys in SETS {
        let mut list = at(keys, 30, 1, 10);
        assert_eq!(list.page(), 1);
        landing(&mut list, 1, HALF_DOWN, Step::Moved, 11);
        landing(&mut list, 1, HALF_UP, Step::Moved, 10);
        landing(&mut list, 1, Key::PageDown, Step::Moved, 11);
        landing(&mut list, 1, Key::PageUp, Step::Moved, 10);
    }
}

#[test]
fn half_a_page_rounds_up_and_is_never_zero() {
    let cases = [
        (1u16, 1usize, 1usize),
        (2, 1, 1),
        (3, 2, 1),
        (4, 3, 2),
        (8, 7, 4),
        (11, 10, 5),
        (12, 11, 6),
    ];
    for (height, page, half) in cases {
        let mut list = at(Keys::new(), 100, height, 50);
        assert_eq!(list.page(), page, "{height}");
        landing(&mut list, height, HALF_DOWN, Step::Moved, 50 + half);
        landing(&mut list, height, HALF_UP, Step::Moved, 50);
    }
}

#[test]
fn before_the_first_draw_half_a_page_is_half_the_default() {
    let mut list = numbered(100);
    assert_eq!(list.page(), 10);
    assert_eq!(list.half_down(), Step::Moved);
    assert_eq!(list.selected(), Some(5));
    assert_eq!(list.page_down(), Step::Moved);
    assert_eq!(list.selected(), Some(15));
    assert_eq!(list.half_up(), Step::Moved);
    assert_eq!(list.selected(), Some(10));
    assert_eq!(list.top(), 0);
}

fn grouped() -> List {
    let mut rows = Vec::new();
    for group in 1..=3 {
        if group > 1 {
            rows.push(Row::blank());
        }
        rows.push(Row::heading([format!("Group {group}")]));
        for _ in 0..4 {
            rows.push(Row::new([format!("row {}", rows.len())]));
        }
    }
    List::new().with_rows(rows)
}

#[test]
fn sections_in_the_way_are_skipped_going_down() {
    for keys in SETS {
        let mut list = grouped().keys(keys);
        shown(&mut list, 10);
        assert_eq!(list.selected(), Some(1));
        assert_eq!(list.key(HALF_DOWN), Step::Moved);
        assert_eq!(list.selected(), Some(7));
        assert_eq!(list.key(HALF_DOWN), Step::Moved);
        assert_eq!(list.selected(), Some(13));
        assert_eq!(list.key(HALF_DOWN), Step::Moved);
        assert_eq!(list.selected(), Some(16));
        assert_eq!(list.key(HALF_DOWN), Step::Held);
    }
}

#[test]
fn sections_in_the_way_are_skipped_going_up() {
    for keys in SETS {
        let mut list = grouped().keys(keys);
        shown(&mut list, 10);
        list.last();
        assert_eq!(list.selected(), Some(16));
        assert_eq!(list.key(HALF_UP), Step::Moved);
        assert_eq!(list.selected(), Some(10));
        assert_eq!(list.key(HALF_UP), Step::Moved);
        assert_eq!(list.selected(), Some(4));
        assert_eq!(list.key(HALF_UP), Step::Moved);
        assert_eq!(list.selected(), Some(1));
        assert_eq!(list.key(HALF_UP), Step::Held);
    }
}

#[test]
fn the_page_keys_skip_sections_the_same_way() {
    for keys in SETS {
        let mut list = grouped().keys(keys);
        shown(&mut list, 10);
        assert_eq!(list.key(Key::PageDown), Step::Moved);
        assert_eq!(list.selected(), Some(10));
        assert_eq!(list.key(Key::PageDown), Step::Moved);
        assert_eq!(list.selected(), Some(16));
        assert_eq!(list.key(Key::PageUp), Step::Moved);
        assert_eq!(list.selected(), Some(7));
        assert_eq!(list.key(Key::PageUp), Step::Moved);
        assert_eq!(list.selected(), Some(1));
        assert_eq!(list.key(Key::PageUp), Step::Held);
    }
}

#[test]
fn view_paging_keeps_the_selection_on_its_line_with_the_half_keys() {
    for keys in SETS {
        let mut list = numbered(100).paging(Paging::View).keys(keys);
        shown(&mut list, 10);
        for _ in 0..4 {
            list.down();
        }
        assert_eq!(line_of_selection(&mut list, 10), 4);
        assert_eq!(list.key(HALF_DOWN), Step::Moved);
        assert_eq!(list.selected(), Some(9));
        assert_eq!(list.top(), 5);
        assert_eq!(line_of_selection(&mut list, 10), 4);
        assert_eq!(list.half_down(), Step::Moved);
        assert_eq!(list.selected(), Some(14));
        assert_eq!(list.top(), 10);
        assert_eq!(line_of_selection(&mut list, 10), 4);
        assert_eq!(list.key(HALF_UP), Step::Moved);
        assert_eq!(list.selected(), Some(9));
        assert_eq!(list.top(), 5);
        assert_eq!(line_of_selection(&mut list, 10), 4);
        assert_eq!(list.key(HALF_UP), Step::Moved);
        assert_eq!(list.key(HALF_UP), Step::Moved);
        assert_eq!(list.selected(), Some(0));
        assert_eq!(list.top(), 0);
        assert_eq!(list.key(HALF_UP), Step::Held);
    }
}

#[test]
fn view_paging_with_the_half_keys_stops_at_the_end_of_the_list() {
    for keys in SETS {
        let mut list = numbered(100).paging(Paging::View).keys(keys);
        list.select(95);
        assert_eq!(line_of_selection(&mut list, 10), 9);
        assert_eq!(list.key(HALF_DOWN), Step::Moved);
        assert_eq!(list.selected(), Some(99));
        let text = ended(&mut list, 10);
        assert_eq!(text[8], "▌ row 99");
        assert_eq!(text[9], "  · end ·");
        assert_eq!(list.key(HALF_DOWN), Step::Held);
    }
}

#[test]
fn view_paging_with_the_half_keys_stops_at_the_start_of_the_list() {
    for keys in SETS {
        let mut list = numbered(100).paging(Paging::View).keys(keys);
        shown(&mut list, 10);
        list.select(2);
        assert_eq!(line_of_selection(&mut list, 10), 2);
        assert_eq!(list.key(HALF_UP), Step::Moved);
        assert_eq!(list.selected(), Some(0));
        assert_eq!(list.top(), 0);
        assert_eq!(line_of_selection(&mut list, 10), 0);
        assert_eq!(list.key(HALF_UP), Step::Held);
    }
}

#[test]
fn cursor_paging_with_the_half_keys_leaves_the_view_to_the_next_draw() {
    let mut list = numbered(100);
    shown(&mut list, 10);
    assert_eq!(list.key(HALF_DOWN), Step::Moved);
    assert_eq!(list.top(), 0);
    assert_eq!(list.key(HALF_DOWN), Step::Moved);
    assert_eq!(list.selected(), Some(10));
    assert_eq!(list.top(), 0);
    assert_eq!(line_of_selection(&mut list, 10), 9);
    assert_eq!(list.top(), 1);
}

#[test]
fn the_old_vim_page_behaviour_is_gone() {
    let mut list = at(Keys::VIM, 100, 10, 0);
    assert_eq!(list.key(Key::Ctrl('d')), Step::Moved);
    assert_eq!(list.selected(), Some(5));
    assert_eq!(list.key(Key::Ctrl('u')), Step::Moved);
    assert_eq!(list.selected(), Some(0));
    assert_eq!(list.key(Key::PageDown), Step::Moved);
    assert_eq!(list.selected(), Some(9));
}

#[test]
fn a_custom_map_through_the_builders_replaces_the_defaults() {
    const KEYS: Keys = Keys::new()
        .half_down(&[Key::Char('d')])
        .half_up(&[Key::Char('u')])
        .page_down(&[Key::Char('f')])
        .page_up(&[Key::Char('b')]);
    let mut list = at(KEYS, 100, 10, 0);
    assert_eq!(list.key(Key::Char('d')), Step::Moved);
    assert_eq!(list.selected(), Some(5));
    assert_eq!(list.key(Key::Char('f')), Step::Moved);
    assert_eq!(list.selected(), Some(14));
    assert_eq!(list.key(Key::Char('u')), Step::Moved);
    assert_eq!(list.selected(), Some(9));
    assert_eq!(list.key(Key::Char('b')), Step::Moved);
    assert_eq!(list.selected(), Some(0));
    for key in [HALF_DOWN, HALF_UP, Key::PageDown, Key::PageUp] {
        assert_eq!(list.key(key), Step::Pass, "{key:?}");
        assert_eq!(list.selected(), Some(0), "{key:?}");
    }
}

#[test]
fn emptying_the_half_maps_hands_the_keys_back_to_the_app() {
    let mut list = at(Keys::new().half_down(&[]).half_up(&[]), 100, 10, 20);
    assert_eq!(list.key(HALF_DOWN), Step::Pass);
    assert_eq!(list.key(HALF_UP), Step::Pass);
    assert_eq!(list.selected(), Some(20));
    assert_eq!(list.key(Key::PageDown), Step::Moved);
    assert_eq!(list.selected(), Some(29));
}

#[test]
fn a_key_in_both_a_page_and_a_half_map_pages() {
    let mut list = at(
        Keys::new()
            .half_down(&[Key::Char('x')])
            .page_down(&[Key::Char('x')]),
        100,
        10,
        0,
    );
    assert_eq!(list.key(Key::Char('x')), Step::Moved);
    assert_eq!(list.selected(), Some(9));
}

#[test]
fn half_keys_come_before_first_and_last() {
    let mut list = at(Keys::new().last(&[HALF_DOWN]).first(&[HALF_UP]), 100, 10, 0);
    assert_eq!(list.key(HALF_DOWN), Step::Moved);
    assert_eq!(list.selected(), Some(5));
    assert_eq!(list.key(HALF_UP), Step::Moved);
    assert_eq!(list.selected(), Some(0));
}
