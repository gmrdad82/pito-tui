---
title: Feed it keys from crossterm
description: Turn crossterm key events into the crate's Key with the crossterm feature, and what happens to Alt and AltGr.
sidebar:
  order: 16
---

Every part of the footer that takes keys takes the crate's own `Key`: `Help`,
`Confirm`, `Input` and `QuitGuard`. It is a small enum, so the crate needs no
backend:

```rust
pub enum Key {
    Char(char), Ctrl(char), Alt(char), Tab, BackTab, Enter, Esc, Backspace,
    Left, Right, Up, Down, Home, End, Delete, Other,
}
```

It is `#[non_exhaustive]`: match it with a wildcard arm.

## From crossterm

With the `crossterm` feature (crossterm 0.29), `Key::from` takes a crossterm
`KeyEvent`:

```toml
pito-footer = { git = "https://github.com/gmrdad82/pito-footer", tag = "v1.0.1", features = ["crossterm"] }
```

```rust
use pito_tui::footer::Key;
use ratatui::crossterm::event::{self, Event};

if let Event::Key(event) = event::read()? {
    let key = Key::from(event);
}
```

The conversion:

- keeps Alt apart: `Key::Alt('y')` is not `Key::Char('y')`, so Alt+y does not
  answer Yes in a confirm;
- turns Alt, Super, Meta or Hyper on any other key into `Key::Other`;
- passes Ctrl+Alt plus a character on as that character, which is how AltGr
  arrives on some platforms, so diacritics can still be typed into an `Input`.

## From another backend

Without the feature, build the `Key` in the app: `Key::Char('q')`,
`Key::Ctrl('c')`, `Key::Enter`. Anything the footer doesn't use can be
`Key::Other`.
