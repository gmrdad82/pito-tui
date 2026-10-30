#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
#[non_exhaustive]
pub enum Key {
    Char(char),
    Ctrl(char),
    Alt(char),
    Tab,
    BackTab,
    Enter,
    Esc,
    Backspace,
    Left,
    Right,
    Up,
    Down,
    Home,
    End,
    Delete,
    Other,
}

impl From<crossterm::event::KeyEvent> for Key {
    fn from(event: crossterm::event::KeyEvent) -> Self {
        use crossterm::event::{KeyCode, KeyEventKind, KeyModifiers};
        if event.kind == KeyEventKind::Release {
            return Key::Other;
        }
        let modifiers = event.modifiers;
        let control = modifiers.contains(KeyModifiers::CONTROL);
        let alt = modifiers.contains(KeyModifiers::ALT);
        let other =
            modifiers.intersects(KeyModifiers::SUPER | KeyModifiers::HYPER | KeyModifiers::META);
        match event.code {
            KeyCode::Char(_) if other => Key::Other,
            KeyCode::Char(c) if control && alt => Key::Char(c),
            KeyCode::Char(c) if control => Key::Ctrl(c.to_ascii_lowercase()),
            KeyCode::Char(c) if alt => Key::Alt(c),
            KeyCode::Char(c) => Key::Char(c),
            _ if other || alt => Key::Other,
            KeyCode::Tab if event.modifiers.contains(KeyModifiers::SHIFT) => Key::BackTab,
            KeyCode::Tab => Key::Tab,
            KeyCode::BackTab => Key::BackTab,
            KeyCode::Enter => Key::Enter,
            KeyCode::Esc => Key::Esc,
            KeyCode::Backspace => Key::Backspace,
            KeyCode::Left => Key::Left,
            KeyCode::Right => Key::Right,
            KeyCode::Up => Key::Up,
            KeyCode::Down => Key::Down,
            KeyCode::Home => Key::Home,
            KeyCode::End => Key::End,
            KeyCode::Delete => Key::Delete,
            _ => Key::Other,
        }
    }
}
