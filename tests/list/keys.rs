use crate::common::*;
use pito_tui::list::{Key, Keys, List, Mods, Named, Row, Step};

const UP: &[Key] = &[Key::Char('k')];
const DOWN: &[Key] = &[Key::Char('j')];
const PAGE_UP: &[Key] = &[Key::Ctrl('b')];
const PAGE_DOWN: &[Key] = &[Key::Ctrl('f')];
const HALF_UP: &[Key] = &[Key::Ctrl('p')];
const HALF_DOWN: &[Key] = &[Key::Ctrl('n')];
const FIRST: &[Key] = &[Key::Char('<')];
const LAST: &[Key] = &[Key::Char('>')];
const OPEN: &[Key] = &[Key::Char('o'), Key::Char(' ')];

fn remapped() -> Keys {
    Keys::new()
        .up(UP)
        .down(DOWN)
        .page_up(PAGE_UP)
        .page_down(PAGE_DOWN)
        .half_up(HALF_UP)
        .half_down(HALF_DOWN)
        .first(FIRST)
        .last(LAST)
        .open(OPEN)
}

fn numbered(count: usize) -> List {
    List::new().with_rows((0..count).map(|n| Row::new([format!("row {n}")])))
}

#[test]
fn the_builders_replace_each_map_and_the_default_is_new() {
    let keys = remapped();
    assert_eq!(keys.up, UP);
    assert_eq!(keys.down, DOWN);
    assert_eq!(keys.page_up, PAGE_UP);
    assert_eq!(keys.page_down, PAGE_DOWN);
    assert_eq!(keys.half_up, HALF_UP);
    assert_eq!(keys.half_down, HALF_DOWN);
    assert_eq!(keys.first, FIRST);
    assert_eq!(keys.last, LAST);
    assert_eq!(keys.open, OPEN);
    assert_eq!(Keys::default(), Keys::new());
    assert_eq!(Keys::default().up, &[Key::Up]);
    assert_eq!(Keys::default().open, &[Key::Enter]);
    let only_up = Keys::new().up(UP);
    assert_eq!(only_up.down, Keys::new().down);
    assert_eq!(only_up.open, Keys::new().open);
    assert_ne!(only_up, Keys::new());
}

#[test]
fn an_app_remaps_its_keys_and_the_old_ones_do_nothing() {
    let mut list = numbered(30).keys(remapped());
    for old in [
        Key::Down,
        Key::Up,
        Key::PageDown,
        Key::PageUp,
        Key::End,
        Key::Home,
        Key::Enter,
    ] {
        assert_eq!(list.key(old), Step::Pass, "{old:?}");
        assert_eq!(list.selected(), Some(0), "{old:?}");
    }
    assert_eq!(list.key(Key::Char('j')), Step::Moved);
    assert_eq!(list.selected(), Some(1));
    assert_eq!(list.key(Key::Char('k')), Step::Moved);
    assert_eq!(list.selected(), Some(0));
    assert_eq!(list.key(Key::Char('k')), Step::Held);
    assert_eq!(list.key(Key::Ctrl('f')), Step::Moved);
    assert_eq!(list.selected(), Some(10));
    assert_eq!(list.key(Key::Ctrl('b')), Step::Moved);
    assert_eq!(list.selected(), Some(0));
    assert_eq!(list.key(Key::Char('>')), Step::Moved);
    assert_eq!(list.selected(), Some(29));
    assert_eq!(list.key(Key::Char('j')), Step::Held);
    assert_eq!(list.key(Key::Char('o')), Step::Open(29));
    assert_eq!(list.key(Key::Char(' ')), Step::Open(29));
    assert_eq!(list.key(Key::Char('<')), Step::Moved);
    assert_eq!(list.selected(), Some(0));
    assert_eq!(list.key(Key::Char('o')), Step::Open(0));
    let drawn = draw(&mut list, 30, 6);
    assert_eq!(drawn.text[1], "▌ row 0");
}

#[test]
fn a_key_in_no_map_passes_and_moves_nothing() {
    let mut list = numbered(5).keys(remapped());
    list.select(2);
    for key in [
        Key::Char('x'),
        Key::Ctrl('j'),
        Key::Alt('j'),
        Key::Char('J'),
        Key::Tab,
        Key::Esc,
        Key::Other,
    ] {
        assert_eq!(list.key(key), Step::Pass, "{key:?}");
        assert_eq!(list.selected(), Some(2), "{key:?}");
    }
    let mut plain = numbered(5);
    assert_eq!(plain.key(Key::Char('j')), Step::Pass);
    assert_eq!(plain.key(Key::Char('o')), Step::Pass);
    assert_eq!(plain.selected(), Some(0));
}

#[test]
fn an_empty_list_holds_on_its_mapped_keys_and_passes_the_rest() {
    let mut list = List::default().keys(remapped());
    assert!(list.is_empty());
    for key in [
        Key::Char('j'),
        Key::Char('k'),
        Key::Char('>'),
        Key::Char('o'),
    ] {
        assert_eq!(list.key(key), Step::Held, "{key:?}");
    }
    assert_eq!(list.key(Key::Down), Step::Pass);
    assert_eq!(list.selected(), None);
    let mut fresh = List::default();
    assert_eq!(fresh.len(), 0);
    assert_eq!(fresh.key(Key::Down), Step::Held);
    assert_eq!(fresh.key(Key::Enter), Step::Held);
    assert_eq!(draw(&mut fresh, 20, 3).text[1], "  Nothing here.");
}

#[test]
fn a_key_in_two_maps_takes_the_first_map_in_the_fixed_order() {
    let mut list = numbered(5).keys(Keys::new().down(DOWN).open(DOWN));
    assert_eq!(list.key(Key::Char('j')), Step::Moved);
    assert_eq!(list.selected(), Some(1));
    let mut list = numbered(5).keys(Keys::new().up(OPEN).open(OPEN));
    list.select(2);
    assert_eq!(list.key(Key::Char('o')), Step::Moved);
    assert_eq!(list.selected(), Some(1));
    let mut list = numbered(5).keys(Keys::new().last(OPEN).open(OPEN));
    assert_eq!(list.key(Key::Char(' ')), Step::Moved);
    assert_eq!(list.selected(), Some(4));
    assert_eq!(list.key(Key::Char(' ')), Step::Held);
}

#[test]
fn mods_read_back_their_own_modifiers() {
    assert!(Mods::CTRL.ctrl() && !Mods::CTRL.shift());
    assert!(!Mods::SHIFT.ctrl() && Mods::SHIFT.shift());
    assert!(Mods::CTRL_SHIFT.ctrl() && Mods::CTRL_SHIFT.shift());
    assert_ne!(Mods::CTRL, Mods::SHIFT);
    assert_ne!(Mods::CTRL, Mods::CTRL_SHIFT);
    assert_ne!(Mods::SHIFT, Mods::CTRL_SHIFT);
}

#[test]
fn mods_print_their_names_in_debug() {
    assert_eq!(format!("{:?}", Mods::CTRL), "CTRL");
    assert_eq!(format!("{:?}", Mods::SHIFT), "SHIFT");
    assert_eq!(format!("{:?}", Mods::CTRL_SHIFT), "CTRL_SHIFT");
    assert_eq!(
        format!("{:?}", Key::Mod(Mods::CTRL_SHIFT, Named::PageDown)),
        "Mod(CTRL_SHIFT, PageDown)"
    );
}

mod crossterm_keys {
    use crossterm::event::{KeyCode, KeyEvent, KeyEventKind, KeyEventState, KeyModifiers};
    use pito_tui::list::{Key, Keys, List, Mods, Named, Row, Step};

    const NAMED: [(KeyCode, Named, Key); 13] = [
        (KeyCode::Enter, Named::Enter, Key::Enter),
        (KeyCode::Esc, Named::Esc, Key::Esc),
        (KeyCode::Backspace, Named::Backspace, Key::Backspace),
        (KeyCode::Left, Named::Left, Key::Left),
        (KeyCode::Right, Named::Right, Key::Right),
        (KeyCode::Up, Named::Up, Key::Up),
        (KeyCode::Down, Named::Down, Key::Down),
        (KeyCode::Home, Named::Home, Key::Home),
        (KeyCode::End, Named::End, Key::End),
        (KeyCode::PageUp, Named::PageUp, Key::PageUp),
        (KeyCode::PageDown, Named::PageDown, Key::PageDown),
        (KeyCode::Delete, Named::Delete, Key::Delete),
        (KeyCode::Tab, Named::Tab, Key::Tab),
    ];

    fn rows(count: usize) -> List {
        List::new().with_rows((0..count).map(|n| Row::new([format!("row {n}")])))
    }

    fn key(code: KeyCode, modifiers: KeyModifiers) -> Key {
        Key::from(KeyEvent::new(code, modifiers))
    }

    #[test]
    fn super_hyper_and_meta_make_any_key_other() {
        for held in [KeyModifiers::SUPER, KeyModifiers::HYPER, KeyModifiers::META] {
            assert_eq!(key(KeyCode::Char('j'), held), Key::Other, "{held:?}");
            assert_eq!(key(KeyCode::Down, held), Key::Other, "{held:?}");
            assert_eq!(key(KeyCode::Enter, held), Key::Other, "{held:?}");
            assert_eq!(
                key(KeyCode::Char('d'), held | KeyModifiers::CONTROL),
                Key::Other,
                "{held:?}"
            );
            assert_eq!(key(KeyCode::Tab, held | KeyModifiers::SHIFT), Key::Other);
        }
    }

    #[test]
    fn control_and_alt_on_chars() {
        let both = KeyModifiers::CONTROL | KeyModifiers::ALT;
        assert_eq!(key(KeyCode::Char('@'), both), Key::Char('@'));
        assert_eq!(key(KeyCode::Char('Q'), both), Key::Char('Q'));
        assert_eq!(
            key(KeyCode::Char('U'), KeyModifiers::CONTROL),
            Key::Ctrl('u')
        );
        assert_eq!(
            key(KeyCode::Char('u'), KeyModifiers::CONTROL),
            Key::Ctrl('u')
        );
        assert_eq!(
            key(
                KeyCode::Char('U'),
                KeyModifiers::CONTROL | KeyModifiers::SHIFT
            ),
            Key::Ctrl('u')
        );
        assert_eq!(key(KeyCode::Char('x'), KeyModifiers::ALT), Key::Alt('x'));
        assert_eq!(key(KeyCode::Char('X'), KeyModifiers::ALT), Key::Alt('X'));
        assert_eq!(key(KeyCode::Char('é'), KeyModifiers::NONE), Key::Char('é'));
    }

    #[test]
    fn alt_on_a_non_char_key_is_other() {
        for code in [
            KeyCode::Enter,
            KeyCode::Tab,
            KeyCode::Left,
            KeyCode::PageDown,
            KeyCode::Backspace,
        ] {
            assert_eq!(key(code, KeyModifiers::ALT), Key::Other, "{code:?}");
        }
    }

    #[test]
    fn named_keys_convert() {
        let none = KeyModifiers::NONE;
        let cases = [
            (KeyCode::Tab, KeyModifiers::SHIFT, Key::BackTab),
            (KeyCode::Tab, none, Key::Tab),
            (KeyCode::BackTab, KeyModifiers::SHIFT, Key::BackTab),
            (KeyCode::BackTab, none, Key::BackTab),
            (KeyCode::Enter, none, Key::Enter),
            (KeyCode::Esc, none, Key::Esc),
            (KeyCode::Backspace, none, Key::Backspace),
            (KeyCode::Delete, none, Key::Delete),
            (KeyCode::Home, none, Key::Home),
            (KeyCode::End, none, Key::End),
            (KeyCode::PageUp, none, Key::PageUp),
            (KeyCode::PageDown, none, Key::PageDown),
            (KeyCode::Left, none, Key::Left),
            (KeyCode::Right, none, Key::Right),
            (KeyCode::Up, none, Key::Up),
            (KeyCode::Down, none, Key::Down),
            (
                KeyCode::Down,
                KeyModifiers::SHIFT,
                Key::Mod(Mods::SHIFT, Named::Down),
            ),
            (
                KeyCode::Up,
                KeyModifiers::CONTROL,
                Key::Mod(Mods::CTRL, Named::Up),
            ),
            (KeyCode::F(5), none, Key::Other),
            (KeyCode::Insert, none, Key::Other),
            (KeyCode::Null, none, Key::Other),
        ];
        for (code, modifiers, expected) in cases {
            assert_eq!(key(code, modifiers), expected, "{code:?} {modifiers:?}");
        }
    }

    #[test]
    fn ctrl_and_shift_on_every_named_key_reach_the_app() {
        let ctrl = KeyModifiers::CONTROL;
        let shift = KeyModifiers::SHIFT;
        for (code, named, plain) in NAMED {
            assert_eq!(key(code, KeyModifiers::NONE), plain, "{code:?}");
            assert_eq!(key(code, ctrl), Key::Mod(Mods::CTRL, named), "{code:?}");
            if code == KeyCode::Tab {
                continue;
            }
            assert_eq!(key(code, shift), Key::Mod(Mods::SHIFT, named), "{code:?}");
            assert_eq!(
                key(code, ctrl | shift),
                Key::Mod(Mods::CTRL_SHIFT, named),
                "{code:?}"
            );
        }
    }

    #[test]
    fn a_converted_key_reads_back_ctrl_and_shift() {
        let Key::Mod(both, Named::PageDown) = key(
            KeyCode::PageDown,
            KeyModifiers::CONTROL | KeyModifiers::SHIFT,
        ) else {
            panic!("Ctrl+Shift+PageDown is not Key::Mod");
        };
        assert!(both.ctrl() && both.shift());
        let Key::Mod(ctrl, Named::Up) = key(KeyCode::Up, KeyModifiers::CONTROL) else {
            panic!("Ctrl+Up is not Key::Mod");
        };
        assert!(ctrl.ctrl() && !ctrl.shift());
        let Key::Mod(shift, Named::Enter) = key(KeyCode::Enter, KeyModifiers::SHIFT) else {
            panic!("Shift+Enter is not Key::Mod");
        };
        assert!(!shift.ctrl() && shift.shift());
    }

    #[test]
    fn shift_is_consumed_by_tab_and_back_tab() {
        let ctrl = KeyModifiers::CONTROL;
        let shift = KeyModifiers::SHIFT;
        let ctrl_back_tab = Key::Mod(Mods::CTRL, Named::BackTab);
        assert_eq!(key(KeyCode::Tab, shift), Key::BackTab);
        assert_eq!(key(KeyCode::Tab, ctrl), Key::Mod(Mods::CTRL, Named::Tab));
        assert_eq!(key(KeyCode::Tab, ctrl | shift), ctrl_back_tab);
        assert_eq!(key(KeyCode::BackTab, KeyModifiers::NONE), Key::BackTab);
        assert_eq!(key(KeyCode::BackTab, shift), Key::BackTab);
        assert_eq!(key(KeyCode::BackTab, ctrl), ctrl_back_tab);
        assert_eq!(key(KeyCode::BackTab, ctrl | shift), ctrl_back_tab);
    }

    #[test]
    fn modifiers_on_chars_alt_super_and_releases_are_unchanged() {
        let ctrl = KeyModifiers::CONTROL;
        let shift = KeyModifiers::SHIFT;
        let alt = KeyModifiers::ALT;
        assert_eq!(key(KeyCode::Char('J'), shift), Key::Char('J'));
        assert_eq!(key(KeyCode::Char('c'), ctrl), Key::Ctrl('c'));
        assert_eq!(key(KeyCode::Char('C'), ctrl | shift), Key::Ctrl('c'));
        assert_eq!(key(KeyCode::Char('y'), alt), Key::Alt('y'));
        assert_eq!(key(KeyCode::Char('ă'), ctrl | alt), Key::Char('ă'));
        assert_eq!(key(KeyCode::F(5), ctrl), Key::Other);
        assert_eq!(key(KeyCode::Insert, shift), Key::Other);
        for (code, _, _) in NAMED {
            assert_eq!(key(code, alt), Key::Other, "{code:?}");
            assert_eq!(key(code, alt | ctrl), Key::Other, "{code:?}");
            assert_eq!(key(code, alt | shift), Key::Other, "{code:?}");
            for held in [KeyModifiers::SUPER, KeyModifiers::HYPER, KeyModifiers::META] {
                assert_eq!(key(code, held | ctrl), Key::Other, "{code:?} {held:?}");
                assert_eq!(key(code, held | shift), Key::Other, "{code:?} {held:?}");
            }
            let release = KeyEvent {
                code,
                modifiers: ctrl,
                kind: KeyEventKind::Release,
                state: KeyEventState::NONE,
            };
            assert_eq!(Key::from(release), Key::Other, "{code:?}");
        }
    }

    #[test]
    fn a_plain_page_down_still_pages() {
        let mut list = rows(40);
        let _ = super::draw(&mut list, 20, 10);
        assert_eq!(
            list.key(key(KeyCode::PageDown, KeyModifiers::NONE)),
            Step::Moved
        );
        assert_eq!(list.selected(), Some(list.page()));
        let mut vim = rows(40).keys(Keys::VIM);
        let _ = super::draw(&mut vim, 20, 10);
        assert_eq!(
            vim.key(key(KeyCode::PageDown, KeyModifiers::NONE)),
            Step::Moved
        );
        assert_eq!(vim.selected(), Some(vim.page()));
        assert!(vim.page() > 1);
    }

    #[test]
    fn ctrl_page_down_passes_under_the_built_in_maps_and_moves_nothing() {
        for keys in [Keys::new(), Keys::VIM] {
            let mut list = rows(40).keys(keys);
            let _ = super::draw(&mut list, 20, 10);
            list.select(5);
            for modifiers in [
                KeyModifiers::CONTROL,
                KeyModifiers::SHIFT,
                KeyModifiers::CONTROL | KeyModifiers::SHIFT,
            ] {
                for code in [
                    KeyCode::PageDown,
                    KeyCode::PageUp,
                    KeyCode::Down,
                    KeyCode::Enter,
                ] {
                    assert_eq!(list.key(key(code, modifiers)), Step::Pass, "{code:?}");
                    assert_eq!(list.selected(), Some(5), "{code:?}");
                }
            }
        }
    }

    #[test]
    fn an_app_that_binds_ctrl_page_down_gets_its_step() {
        const NEXT: &[Key] = &[Key::Mod(Mods::CTRL, Named::PageDown)];
        const OPEN: &[Key] = &[Key::Mod(Mods::CTRL_SHIFT, Named::Enter)];
        let mut list = rows(40).keys(Keys::new().page_down(NEXT).open(OPEN));
        let _ = super::draw(&mut list, 20, 10);
        assert_eq!(
            list.key(key(KeyCode::PageDown, KeyModifiers::NONE)),
            Step::Pass
        );
        assert_eq!(list.selected(), Some(0));
        assert_eq!(
            list.key(key(KeyCode::PageDown, KeyModifiers::CONTROL)),
            Step::Moved
        );
        let paged = list.page();
        assert!(paged > 1);
        assert_eq!(list.selected(), Some(paged));
        assert_eq!(
            list.key(key(
                KeyCode::PageDown,
                KeyModifiers::CONTROL | KeyModifiers::SHIFT
            )),
            Step::Pass
        );
        assert_eq!(
            list.key(key(KeyCode::Enter, KeyModifiers::NONE)),
            Step::Pass
        );
        assert_eq!(
            list.key(key(KeyCode::Enter, KeyModifiers::CONTROL)),
            Step::Pass
        );
        assert_eq!(
            list.key(key(
                KeyCode::Enter,
                KeyModifiers::CONTROL | KeyModifiers::SHIFT
            )),
            Step::Open(paged)
        );
    }

    #[test]
    fn alt_and_super_page_down_stay_other_and_pass() {
        const NEXT: &[Key] = &[Key::Mod(Mods::CTRL, Named::PageDown)];
        let mut list = rows(40).keys(Keys::new().page_down(NEXT));
        let _ = super::draw(&mut list, 20, 10);
        for held in [
            KeyModifiers::ALT,
            KeyModifiers::SUPER,
            KeyModifiers::ALT | KeyModifiers::CONTROL,
            KeyModifiers::SUPER | KeyModifiers::CONTROL,
        ] {
            let converted = key(KeyCode::PageDown, held);
            assert_eq!(converted, Key::Other, "{held:?}");
            assert_eq!(list.key(converted), Step::Pass, "{held:?}");
            assert_eq!(list.selected(), Some(0), "{held:?}");
        }
    }

    #[test]
    fn a_repeat_converts_and_a_release_does_not() {
        let repeat = KeyEvent {
            code: KeyCode::Char('j'),
            modifiers: KeyModifiers::NONE,
            kind: KeyEventKind::Repeat,
            state: KeyEventState::NONE,
        };
        assert_eq!(Key::from(repeat), Key::Char('j'));
        let held = KeyEvent {
            code: KeyCode::PageDown,
            modifiers: KeyModifiers::CONTROL,
            ..repeat
        };
        assert_eq!(Key::from(held), Key::Mod(Mods::CTRL, Named::PageDown));
        let release = KeyEvent {
            kind: KeyEventKind::Release,
            ..repeat
        };
        assert_eq!(Key::from(release), Key::Other);
    }

    #[test]
    fn crossterm_events_drive_a_remapped_list() {
        const DOWN: &[Key] = &[Key::Char('j'), Key::Ctrl('n')];
        const OPEN: &[Key] = &[Key::Char('l')];
        let mut list = List::new()
            .with_rows((0..4).map(|n| Row::new([format!("row {n}")])))
            .keys(Keys::new().down(DOWN).open(OPEN));
        let none = KeyModifiers::NONE;
        assert_eq!(list.key(key(KeyCode::Char('j'), none)), Step::Moved);
        assert_eq!(
            list.key(key(KeyCode::Char('N'), KeyModifiers::CONTROL)),
            Step::Moved
        );
        assert_eq!(list.selected(), Some(2));
        assert_eq!(
            list.key(key(KeyCode::Char('j'), KeyModifiers::SUPER)),
            Step::Pass
        );
        assert_eq!(
            list.key(key(KeyCode::Char('j'), KeyModifiers::ALT)),
            Step::Pass
        );
        assert_eq!(list.key(key(KeyCode::Enter, none)), Step::Pass);
        assert_eq!(list.key(key(KeyCode::Char('l'), none)), Step::Open(2));
        assert_eq!(list.key(key(KeyCode::F(5), none)), Step::Pass);
    }
}
