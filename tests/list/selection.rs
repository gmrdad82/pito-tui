use crate::common::*;
use pito_tui::list::{Key, Keys, List, Row, Step};

fn numbered(count: usize) -> List {
    List::new().with_rows((0..count).map(|n| Row::new([format!("row {n}")])))
}

#[test]
fn up_and_down_move_and_hold_at_the_ends() {
    let mut list = numbered(3);
    assert_eq!(list.selected(), Some(0));
    assert_eq!(list.up(), Step::Held);
    assert_eq!(list.down(), Step::Moved);
    assert_eq!(list.down(), Step::Moved);
    assert_eq!(list.down(), Step::Held);
    assert_eq!(list.selected(), Some(2));
    assert_eq!(list.up(), Step::Moved);
    assert_eq!(list.selected(), Some(1));
}

#[test]
fn an_empty_list_has_no_selection_and_holds_every_move() {
    let mut list = List::new();
    assert_eq!(list.selected(), None);
    assert_eq!(list.down(), Step::Held);
    assert_eq!(list.last(), Step::Held);
    assert_eq!(list.key(Key::Enter), Step::Held);
    assert!(list.selected_row().is_none());
}

#[test]
fn home_and_end_jump() {
    let mut list = numbered(50);
    assert_eq!(list.last(), Step::Moved);
    assert_eq!(list.selected(), Some(49));
    assert_eq!(list.last(), Step::Held);
    assert_eq!(list.first(), Step::Moved);
    assert_eq!(list.selected(), Some(0));
}

#[test]
fn paging_moves_by_the_drawn_page() {
    let mut list = numbered(100);
    draw(&mut list, 40, 11);
    assert_eq!(list.page(), 9);
    list.page_down();
    assert_eq!(list.selected(), Some(9));
    list.page_down();
    assert_eq!(list.selected(), Some(18));
    list.page_up();
    assert_eq!(list.selected(), Some(9));
    list.select(3);
    assert_eq!(list.page_up(), Step::Moved);
    assert_eq!(list.selected(), Some(0));
    assert_eq!(list.page_up(), Step::Held);
}

#[test]
fn paging_before_any_draw_uses_a_default_page() {
    let mut list = numbered(100);
    list.page_down();
    assert_eq!(list.selected(), Some(10));
}

#[test]
fn paging_past_the_end_lands_on_the_last_row() {
    let mut list = numbered(5);
    draw(&mut list, 40, 11);
    list.page_down();
    assert_eq!(list.selected(), Some(4));
}

#[test]
fn a_one_row_area_still_pages_by_one() {
    let mut list = numbered(10);
    draw(&mut list, 40, 2);
    assert_eq!(list.page(), 1);
}

#[test]
fn keys_are_the_apps() {
    let mut list = numbered(5);
    assert_eq!(list.key(Key::Char('j')), Step::Pass);
    assert_eq!(list.key(Key::Down), Step::Moved);
    assert_eq!(list.key(Key::Tab), Step::Pass);
    assert_eq!(list.key(Key::Esc), Step::Pass);
    assert_eq!(list.key(Key::Enter), Step::Open(1));
    let mut vim = numbered(5).keys(Keys::VIM);
    assert_eq!(vim.key(Key::Char('j')), Step::Moved);
    assert_eq!(vim.key(Key::Char('G')), Step::Moved);
    assert_eq!(vim.selected(), Some(4));
    assert_eq!(vim.key(Key::Char('g')), Step::Moved);
    assert_eq!(vim.selected(), Some(0));
    let mut own = numbered(5).keys(Keys::new().down(&[Key::Char('n')]).open(&[Key::Char('o')]));
    assert_eq!(own.key(Key::Down), Step::Pass);
    assert_eq!(own.key(Key::Char('n')), Step::Moved);
    assert_eq!(own.key(Key::Char('o')), Step::Open(1));
}

#[test]
fn replacing_the_rows_clamps_the_selection() {
    let mut list = numbered(10);
    list.select(8);
    list.set_rows((0..3).map(|n| Row::new([format!("new {n}")])));
    assert_eq!(list.selected(), Some(2));
    list.set_rows(Vec::<Row>::new());
    assert_eq!(list.selected(), None);
    list.set_rows((0..4).map(|n| Row::new([format!("again {n}")])));
    assert_eq!(list.selected(), Some(0));
}

#[test]
fn select_clamps() {
    let mut list = numbered(4);
    list.select(99);
    assert_eq!(list.selected(), Some(3));
    list.clear();
    assert!(list.is_empty());
    assert_eq!(list.len(), 0);
}

#[test]
fn scrolling_keeps_the_selection_in_view() {
    let mut list = numbered(40);
    let drawn = draw(&mut list, 30, 6);
    assert_eq!(list.top(), 0);
    assert_eq!(drawn.text[1], "▌ row 0");
    for _ in 0..12 {
        list.down();
    }
    let drawn = draw(&mut list, 30, 6);
    assert_eq!(list.top(), 8);
    assert!(drawn.text.iter().any(|line| line == "▌ row 12"));
    assert_eq!(drawn.text.last().unwrap(), "▌ row 12");
    for _ in 0..12 {
        list.up();
    }
    draw(&mut list, 30, 6);
    assert_eq!(list.top(), 0);
}

#[test]
fn scrolling_up_follows_the_selection() {
    let mut list = numbered(40);
    list.last();
    draw(&mut list, 30, 8);
    let top = list.top();
    assert!(top > 0);
    list.select(top - 1);
    draw(&mut list, 30, 8);
    assert_eq!(list.top(), top - 1);
}

#[test]
fn the_list_follows_a_resize() {
    let mut list = numbered(40);
    list.select(30);
    draw(&mut list, 30, 20);
    assert!(list.top() <= 30);
    let drawn = draw(&mut list, 30, 4);
    assert!(drawn.text.iter().any(|line| line.starts_with("▌ row 30")));
    let drawn = draw(&mut list, 30, 40);
    assert!(drawn.text.iter().any(|line| line.starts_with("▌ row 30")));
    assert_eq!(list.top(), 2);
}

#[test]
fn scroll_shows_the_tail_when_the_last_row_is_selected() {
    let mut list = numbered(10);
    list.last();
    let drawn = draw(&mut list, 30, 6);
    assert_eq!(drawn.text[4], "▌ row 9");
    assert_eq!(drawn.text[5], "  · end ·");
}

#[test]
fn rows_removed_under_a_scrolled_list_leave_no_blank_view() {
    let mut list = numbered(100);
    list.last();
    draw(&mut list, 30, 10);
    list.set_rows((0..3).map(|n| Row::new([format!("row {n}")])));
    let drawn = draw(&mut list, 30, 10);
    assert_eq!(list.top(), 0);
    assert_eq!(drawn.text[1], "  row 0");
}

#[test]
fn crossterm_keys_convert() {
    use crossterm::event::{KeyCode, KeyEvent, KeyEventKind, KeyEventState, KeyModifiers};
    let key = |code, modifiers| Key::from(KeyEvent::new(code, modifiers));
    assert_eq!(key(KeyCode::PageUp, KeyModifiers::NONE), Key::PageUp);
    assert_eq!(key(KeyCode::PageDown, KeyModifiers::NONE), Key::PageDown);
    assert_eq!(key(KeyCode::Home, KeyModifiers::NONE), Key::Home);
    assert_eq!(key(KeyCode::Char('G'), KeyModifiers::SHIFT), Key::Char('G'));
    assert_eq!(
        key(KeyCode::Char('D'), KeyModifiers::CONTROL),
        Key::Ctrl('d')
    );
    assert_eq!(key(KeyCode::Char('y'), KeyModifiers::ALT), Key::Alt('y'));
    assert_eq!(key(KeyCode::Down, KeyModifiers::ALT), Key::Other);
    assert_eq!(key(KeyCode::Tab, KeyModifiers::SHIFT), Key::BackTab);
    assert_eq!(key(KeyCode::F(5), KeyModifiers::NONE), Key::Other);
    let release = KeyEvent {
        code: KeyCode::Down,
        modifiers: KeyModifiers::NONE,
        kind: KeyEventKind::Release,
        state: KeyEventState::NONE,
    };
    assert_eq!(Key::from(release), Key::Other);
}
