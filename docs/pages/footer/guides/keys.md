---
title: Feed it keys from crossterm
description: Turn crossterm key events into the footer's Key, and what happens to Alt and AltGr.
sidebar:
  order: 16
---

Every part of the footer that takes keys takes the footer's own `Key`: `Help`,
`Confirm`, `Input` and `QuitGuard`. It is a small enum, so drawing and
answering keys need no backend:

```rust
pub enum Key {
    Char(char), Ctrl(char), Alt(char), Tab, BackTab, Enter, Esc, Backspace,
    Left, Right, Up, Down, Home, End, Delete, Other,
}
```

It is `#[non_exhaustive]`: match it with a wildcard arm.

## From crossterm

`Key::from` takes a crossterm 0.29 `KeyEvent`, the crossterm pito-tui brings
as `pito_tui::crossterm`:

```rust
use pito_tui::footer::Key;
use pito_tui::crossterm::event::{self, Event};

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

An app on another backend builds the `Key` itself: `Key::Char('q')`,
`Key::Ctrl('c')`, `Key::Enter`. Anything the footer doesn't use can be
`Key::Other`.
