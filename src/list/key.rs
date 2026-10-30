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
    PageUp,
    PageDown,
    Delete,
    Mod(Mods, Named),
    Other,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
#[non_exhaustive]
pub enum Named {
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
    PageUp,
    PageDown,
    Delete,
}

#[derive(Clone, Copy, PartialEq, Eq, Hash)]
pub struct Mods(u8);

impl Mods {
    pub const CTRL: Mods = Mods(1);
    pub const SHIFT: Mods = Mods(2);
    pub const CTRL_SHIFT: Mods = Mods(3);

    pub const fn ctrl(self) -> bool {
        self.0 & 1 != 0
    }

    pub const fn shift(self) -> bool {
        self.0 & 2 != 0
    }

    const fn name(self) -> &'static str {
        match (self.ctrl(), self.shift()) {
            (true, true) => "CTRL_SHIFT",
            (true, false) => "CTRL",
            _ => "SHIFT",
        }
    }

    const fn of(ctrl: bool, shift: bool) -> Option<Mods> {
        match (ctrl, shift) {
            (true, true) => Some(Mods::CTRL_SHIFT),
            (true, false) => Some(Mods::CTRL),
            (false, true) => Some(Mods::SHIFT),
            (false, false) => None,
        }
    }
}

impl std::fmt::Debug for Mods {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(self.name())
    }
}

impl Named {
    const fn key(self) -> Key {
        match self {
            Named::Tab => Key::Tab,
            Named::BackTab => Key::BackTab,
            Named::Enter => Key::Enter,
            Named::Esc => Key::Esc,
            Named::Backspace => Key::Backspace,
            Named::Left => Key::Left,
            Named::Right => Key::Right,
            Named::Up => Key::Up,
            Named::Down => Key::Down,
            Named::Home => Key::Home,
            Named::End => Key::End,
            Named::PageUp => Key::PageUp,
            Named::PageDown => Key::PageDown,
            Named::Delete => Key::Delete,
        }
    }
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
        let shift = modifiers.contains(KeyModifiers::SHIFT);
        let other =
            modifiers.intersects(KeyModifiers::SUPER | KeyModifiers::HYPER | KeyModifiers::META);
        let (named, shift) = match event.code {
            KeyCode::Char(_) if other => return Key::Other,
            KeyCode::Char(c) if control && alt => return Key::Char(c),
            KeyCode::Char(c) if control => return Key::Ctrl(c.to_ascii_lowercase()),
            KeyCode::Char(c) if alt => return Key::Alt(c),
            KeyCode::Char(c) => return Key::Char(c),
            _ if other || alt => return Key::Other,
            KeyCode::Tab if shift => (Named::BackTab, false),
            KeyCode::Tab => (Named::Tab, false),
            KeyCode::BackTab => (Named::BackTab, false),
            KeyCode::Enter => (Named::Enter, shift),
            KeyCode::Esc => (Named::Esc, shift),
            KeyCode::Backspace => (Named::Backspace, shift),
            KeyCode::Left => (Named::Left, shift),
            KeyCode::Right => (Named::Right, shift),
            KeyCode::Up => (Named::Up, shift),
            KeyCode::Down => (Named::Down, shift),
            KeyCode::Home => (Named::Home, shift),
            KeyCode::End => (Named::End, shift),
            KeyCode::PageUp => (Named::PageUp, shift),
            KeyCode::PageDown => (Named::PageDown, shift),
            KeyCode::Delete => (Named::Delete, shift),
            _ => return Key::Other,
        };
        match Mods::of(control, shift) {
            Some(mods) => Key::Mod(mods, named),
            None => named.key(),
        }
    }
}
