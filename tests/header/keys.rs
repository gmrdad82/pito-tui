use crate::common::nav;
use crossterm::event::{KeyCode, KeyEvent, KeyEventKind, KeyModifiers as M};
use pito_tui::header::{Action, Key, Place, Step};

fn key(code: KeyCode, modifiers: M) -> Key {
    Key::from(KeyEvent::new(code, modifiers))
}

fn place(group: usize, section: usize, number: usize) -> Place {
    Place {
        group,
        section,
        number,
    }
}

#[test]
fn plain_codes_map_to_their_keys() {
    let cases = [
        (KeyCode::Tab, Key::Tab),
        (KeyCode::BackTab, Key::BackTab),
        (KeyCode::Enter, Key::Enter),
        (KeyCode::Esc, Key::Esc),
        (KeyCode::Backspace, Key::Backspace),
        (KeyCode::Left, Key::Left),
        (KeyCode::Right, Key::Right),
        (KeyCode::Up, Key::Up),
        (KeyCode::Down, Key::Down),
        (KeyCode::Char('x'), Key::Char('x')),
        (KeyCode::Char('7'), Key::Char('7')),
    ];
    for (code, want) in cases {
        assert_eq!(key(code, M::NONE), want, "{code:?}");
    }
}

#[test]
fn codes_the_crate_does_not_name_are_other() {
    for code in [
        KeyCode::F(1),
        KeyCode::Home,
        KeyCode::End,
        KeyCode::PageUp,
        KeyCode::PageDown,
        KeyCode::Delete,
        KeyCode::Insert,
        KeyCode::Null,
    ] {
        assert_eq!(key(code, M::NONE), Key::Other, "{code:?}");
    }
}

#[test]
fn shift_keeps_a_key_except_on_tab() {
    assert_eq!(key(KeyCode::Tab, M::SHIFT), Key::BackTab);
    assert_eq!(key(KeyCode::BackTab, M::SHIFT), Key::BackTab);
    assert_eq!(key(KeyCode::Enter, M::SHIFT), Key::Enter);
    assert_eq!(key(KeyCode::Esc, M::SHIFT), Key::Esc);
    assert_eq!(key(KeyCode::Down, M::SHIFT), Key::Down);
    assert_eq!(key(KeyCode::Char('A'), M::SHIFT), Key::Char('A'));
}

#[test]
fn control_lowers_a_char_and_leaves_other_codes_alone() {
    assert_eq!(key(KeyCode::Char('A'), M::CONTROL), Key::Ctrl('a'));
    assert_eq!(
        key(KeyCode::Char('a'), M::CONTROL | M::SHIFT),
        Key::Ctrl('a')
    );
    assert_eq!(key(KeyCode::Tab, M::CONTROL), Key::Tab);
    assert_eq!(key(KeyCode::Tab, M::CONTROL | M::SHIFT), Key::BackTab);
    assert_eq!(key(KeyCode::Enter, M::CONTROL), Key::Enter);
    assert_eq!(key(KeyCode::Up, M::CONTROL), Key::Up);
}

#[test]
fn control_with_alt_is_altgr_and_types_the_char() {
    assert_eq!(key(KeyCode::Char('€'), M::CONTROL | M::ALT), Key::Char('€'));
    assert_eq!(key(KeyCode::Char('Q'), M::CONTROL | M::ALT), Key::Char('Q'));
    assert_eq!(
        key(KeyCode::Char('ș'), M::CONTROL | M::ALT | M::SHIFT),
        Key::Char('ș')
    );
}

#[test]
fn alt_carries_a_char_and_blanks_every_other_code() {
    assert_eq!(key(KeyCode::Char('x'), M::ALT), Key::Alt('x'));
    assert_eq!(key(KeyCode::Char(']'), M::ALT), Key::Alt(']'));
    for code in [
        KeyCode::Tab,
        KeyCode::BackTab,
        KeyCode::Esc,
        KeyCode::Backspace,
        KeyCode::Right,
        KeyCode::Up,
        KeyCode::Down,
    ] {
        assert_eq!(key(code, M::ALT), Key::Other, "{code:?}");
        assert_eq!(key(code, M::ALT | M::CONTROL), Key::Other, "{code:?}");
    }
    assert_eq!(key(KeyCode::Tab, M::ALT | M::SHIFT), Key::Other);
}

#[test]
fn super_hyper_and_meta_blank_any_code() {
    for modifier in [M::SUPER, M::HYPER, M::META] {
        for code in [
            KeyCode::Char('1'),
            KeyCode::Tab,
            KeyCode::BackTab,
            KeyCode::Esc,
            KeyCode::Enter,
            KeyCode::Left,
        ] {
            assert_eq!(key(code, modifier), Key::Other, "{code:?} {modifier:?}");
        }
        assert_eq!(
            key(KeyCode::Char('c'), modifier | M::CONTROL),
            Key::Other,
            "{modifier:?}"
        );
        assert_eq!(
            key(KeyCode::Char('c'), modifier | M::CONTROL | M::ALT),
            Key::Other,
            "{modifier:?}"
        );
    }
}

#[test]
fn a_release_is_other_and_a_repeat_is_the_key() {
    let mut release = KeyEvent::new(KeyCode::Tab, M::NONE);
    release.kind = KeyEventKind::Release;
    assert_eq!(Key::from(release), Key::Other);
    let mut repeat = KeyEvent::new(KeyCode::Tab, M::NONE);
    repeat.kind = KeyEventKind::Repeat;
    assert_eq!(Key::from(repeat), Key::Tab);
}

#[test]
fn nav_acts_on_the_mapped_keys() {
    let mut nav = nav();
    assert_eq!(
        nav.key(key(KeyCode::Tab, M::NONE)),
        Some(Step::Moved(place(1, 0, 4)))
    );
    assert_eq!(
        nav.key(key(KeyCode::Tab, M::SHIFT)),
        Some(Step::Moved(place(0, 0, 1)))
    );
    assert_eq!(
        nav.key(key(KeyCode::BackTab, M::SHIFT)),
        Some(Step::Moved(place(2, 0, 7)))
    );
    assert_eq!(
        nav.key(key(KeyCode::Char(']'), M::NONE)),
        Some(Step::Moved(place(2, 1, 8)))
    );
    assert_eq!(
        nav.key(key(KeyCode::Char('2'), M::NONE)),
        Some(Step::Moved(place(0, 1, 2)))
    );
    nav.open("detail", 4);
    assert_eq!(
        nav.key(key(KeyCode::Esc, M::NONE)),
        Some(Step::Back { selected: 4 })
    );
    assert_eq!(nav.depth(), 0);
}

#[test]
fn nav_ignores_keys_a_modifier_blanked() {
    let mut nav = nav();
    nav.open("detail", 0);
    let blanked = [
        key(KeyCode::Tab, M::ALT),
        key(KeyCode::Esc, M::ALT),
        key(KeyCode::Char(']'), M::SUPER),
        key(KeyCode::Char('3'), M::META),
        key(KeyCode::Char('q'), M::HYPER),
        key(KeyCode::Char('3'), M::CONTROL),
        key(KeyCode::Enter, M::NONE),
        key(KeyCode::Up, M::NONE),
    ];
    for pressed in blanked {
        assert_eq!(nav.action(pressed), None, "{pressed:?}");
        assert_eq!(nav.key(pressed), None, "{pressed:?}");
    }
    assert_eq!(nav.place(), place(0, 0, 1));
    assert_eq!(nav.depth(), 1);
    assert_eq!(
        nav.action(key(KeyCode::Char('3'), M::CONTROL | M::ALT)),
        Some(Action::Go(3))
    );
}
