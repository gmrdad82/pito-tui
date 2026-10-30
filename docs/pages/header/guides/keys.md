---
title: Map keys to moves
description: Feed keys to the nav, choose which keys it takes with NavKeys, and keep every other key for the app.
sidebar:
  order: 11
---

The crate never reads the terminal. The app hands it a `Key`, and the nav
answers whether that key moves it; any key it doesn't take stays the app's.

## Feed it keys

`key(key)` looks the key up and moves, and answers what happened:

```rust
use pito_tui::header::{Group, Key, Nav, Section, Step};

let mut nav = Nav::new(vec![
    Group::new("Mail").section(Section::new("Inbox")).section(Section::new("Drafts")),
    Group::new("Work").section(Section::new("Checks")),
]);
match nav.key(Key::Tab) {
    Some(Step::Moved(place)) => assert_eq!(place.number, 3),
    Some(Step::Back { selected }) => println!("back, select row {selected}"),
    Some(_) => {}
    None => println!("not a nav key: the app handles it"),
}
```

It works in two halves when the app needs to look first: `action(key)`
only answers the `Action` a key maps to, and `apply(action)` moves. `key`
is the two in one.

- `Step::Moved(place)` says the nav moved, and where to.
- `Step::Back { selected }` says a drill level closed, with the list row it
  kept; see [Drill in with a breadcrumb](drill-in.md).
- `Step::Swallowed` says the key was taken and nothing moved.
- `None` says the key is not the nav's.

`Step`, `Action` and `Key` are `#[non_exhaustive]`, so a `match` on them
needs a wildcard arm.

## The default keys

`Nav::new` uses `NavKeys::HEY`, the keys of HEY's terminal UI:

| Key | Action |
|---|---|
| `tab` / `shift+tab` | next / previous group (next / previous section when there is only one group) |
| `]` / `[` | next / previous section |
| `1` to `9`, `0` | jump to that section (`0` is the tenth) |
| `esc`, `q` | back one drill level |

Back at the top level is never an action, so `esc` and `q` stay the app's
there. A digit past the last section is not taken either, unless
`swallow_digits` is on: then it maps to `Action::Swallow`, and `apply`
answers `Step::Swallowed`, so the key is taken and nothing moves.

## Number from zero

With `zero_first` on, `0` is the first section and `1` to `9` the second
through the tenth, and the section labels show those digits. A section past
the tenth gets no digit and no number on its label; `]`, `[` and the group
keys still reach it.

```rust
use pito_tui::header::{Action, Group, Key, Nav, NavKeys, Section};

let nav = Nav::new(vec![
    Group::new("Mail").section(Section::new("Inbox")).section(Section::new("Drafts")),
]).keys(NavKeys::HEY.zero_first(true));
assert_eq!(nav.action(Key::Char('0')), Some(Action::Go(1)));
assert_eq!(nav.action(Key::Char('1')), Some(Action::Go(2)));
```

## Choose your own keys

Build a `NavKeys` from `HEY` or `NONE` with its const methods, and give it to
the nav. Here `q` keeps its meaning in the app, the arrows change section,
and stray digits are taken:

```rust
use pito_tui::header::{Group, Key, Nav, NavKeys, Section};

const KEYS: NavKeys = NavKeys::HEY
    .back(&[Key::Esc])
    .next_section(&[Key::Char(']'), Key::Right])
    .prev_section(&[Key::Char('['), Key::Left])
    .swallow_digits(true);

let mut nav = Nav::new(vec![
    Group::new("Mail").section(Section::new("Inbox")).section(Section::new("Drafts")),
]).keys(KEYS);
assert!(nav.key(Key::Right).is_some());
assert_eq!(nav.place().number, 2);
```

`NavKeys::NONE` takes nothing; turn on only what you want, such as
`NavKeys::NONE.digits(true)`. `NavKeys::default()` is `HEY`.

## Keys from crossterm

With the `crossterm` feature, `Key::from(KeyEvent)` turns a crossterm 0.29
key event into a `Key`:

```rust
use crossterm::event::{self, Event};
use pito_tui::header::{Key, Nav};

fn on_event(nav: &mut Nav) -> std::io::Result<()> {
    if let Event::Key(event) = event::read()? {
        let key = Key::from(event);
        if nav.key(key).is_none() {
            // the app's own keys
        }
    }
    Ok(())
}
```

It keeps Alt apart (`Key::Alt('q')` is not `Key::Char('q')`, so Alt+q is not
Back), turns Alt, Super, Meta or Hyper on any other key into `Key::Other`,
and passes Ctrl+Alt plus a character on as that character, which is how AltGr
arrives on some platforms, so diacritics can still be typed. Ctrl plus a
character is `Key::Ctrl` with the character in lower case, and a key release
is `Key::Other`.
