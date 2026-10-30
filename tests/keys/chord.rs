use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyEventKind, KeyModifiers};
use pito_tui::keys::{Chord, Key, Mods};

fn event(code: KeyCode, mods: KeyModifiers) -> KeyEvent {
    KeyEvent::new(code, mods)
}

#[test]
fn crossterm_keys_become_the_chords_the_keymap_parses() {
    let none = KeyModifiers::NONE;
    let cases = [
        (event(KeyCode::Char('z'), KeyModifiers::CONTROL), "ctrl+z"),
        (
            event(
                KeyCode::Char('Z'),
                KeyModifiers::CONTROL | KeyModifiers::SHIFT,
            ),
            "ctrl+shift+z",
        ),
        (event(KeyCode::Char('A'), KeyModifiers::SHIFT), "shift+a"),
        (event(KeyCode::Char('?'), KeyModifiers::SHIFT), "?"),
        (event(KeyCode::Char('?'), none), "?"),
        (event(KeyCode::Char(' '), none), "space"),
        (
            event(KeyCode::Char(' '), KeyModifiers::SHIFT),
            "shift+space",
        ),
        (event(KeyCode::BackTab, KeyModifiers::SHIFT), "shift+tab"),
        (event(KeyCode::BackTab, none), "shift+tab"),
        (event(KeyCode::F(2), none), "f2"),
        (event(KeyCode::PageDown, none), "pgdn"),
        (event(KeyCode::Left, KeyModifiers::ALT), "alt+left"),
        (event(KeyCode::Enter, none), "enter"),
        (event(KeyCode::Esc, none), "esc"),
    ];
    for (event, text) in cases {
        assert_eq!(
            Chord::from_crossterm(&event),
            Chord::parse(text).ok(),
            "{text}"
        );
    }
    let command = Chord::from_crossterm(&event(KeyCode::Char('s'), KeyModifiers::SUPER));
    assert_eq!(command, Some(Chord::new(Key::Char('s'), Mods::SUPER)));
}

#[test]
fn releases_and_keys_without_a_name_give_no_chord() {
    let release = KeyEvent::new_with_kind(
        KeyCode::Char('a'),
        KeyModifiers::NONE,
        KeyEventKind::Release,
    );
    assert_eq!(Chord::from_crossterm(&release), None);
    let repeat =
        KeyEvent::new_with_kind(KeyCode::Char('a'), KeyModifiers::NONE, KeyEventKind::Repeat);
    assert_eq!(Chord::from_crossterm(&repeat), Some(Chord::char('a')));
    for code in [KeyCode::Null, KeyCode::CapsLock, KeyCode::Menu] {
        assert_eq!(
            Chord::from_crossterm(&event(code, KeyModifiers::NONE)),
            None
        );
    }
}
