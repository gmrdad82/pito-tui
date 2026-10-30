use crate::footer::Hint as Shown;
use crate::keys::chord::{Chord, Key, Mods};
use crate::keys::keymap::Hint;

impl Chord {
    pub fn from_crossterm(event: &crossterm::event::KeyEvent) -> Option<Chord> {
        use crossterm::event::{KeyCode, KeyEventKind, KeyModifiers};
        if event.kind == KeyEventKind::Release {
            return None;
        }
        let mut mods = Mods::NONE;
        for (bit, ours) in [
            (KeyModifiers::CONTROL, Mods::CTRL),
            (KeyModifiers::ALT, Mods::ALT),
            (KeyModifiers::SHIFT, Mods::SHIFT),
            (KeyModifiers::SUPER, Mods::SUPER),
        ] {
            if event.modifiers.contains(bit) {
                mods = mods.with(ours);
            }
        }
        let key = match event.code {
            KeyCode::Char(c) => Key::Char(c),
            KeyCode::F(n) => Key::F(n),
            KeyCode::Enter => Key::Enter,
            KeyCode::Esc => Key::Esc,
            KeyCode::Tab => Key::Tab,
            KeyCode::BackTab => {
                mods = mods.with(Mods::SHIFT);
                Key::Tab
            }
            KeyCode::Backspace => Key::Backspace,
            KeyCode::Delete => Key::Delete,
            KeyCode::Insert => Key::Insert,
            KeyCode::Home => Key::Home,
            KeyCode::End => Key::End,
            KeyCode::PageUp => Key::PageUp,
            KeyCode::PageDown => Key::PageDown,
            KeyCode::Up => Key::Up,
            KeyCode::Down => Key::Down,
            KeyCode::Left => Key::Left,
            KeyCode::Right => Key::Right,
            _ => return None,
        };
        Some(Chord::new(key, mods))
    }
}

impl<'k> From<Hint<'k>> for Shown<'k> {
    fn from(hint: Hint<'k>) -> Self {
        Shown::new(hint.keys, hint.does).rank(hint.rank)
    }
}

impl<'k> From<&Hint<'k>> for Shown<'k> {
    fn from(hint: &Hint<'k>) -> Self {
        Shown::new(hint.keys, hint.does).rank(hint.rank)
    }
}
