---
title: Install
description: Add pito-header to a ratatui app on Linux, Windows or macOS, and draw a first header.
sidebar:
  order: 1
---

pito-header is a Rust library, not on crates.io. Add it from git, pinned to a
release tag. The same lines work on Linux, Windows and macOS, with Rust's
edition 2024 (Rust 1.85 or later):

```sh
cargo add ratatui
cargo add pito-header --git https://github.com/gmrdad82/pito-header --tag @TAG@
```

That writes this line into `Cargo.toml`:

```toml
pito-header = { git = "https://github.com/gmrdad82/pito-header", tag = "@TAG@" }
```

The crate draws into ratatui's buffer and needs no terminal backend. If your
app reads keys with crossterm 0.29, turn on the `crossterm` feature to get
`Key::from(KeyEvent)`:

```sh
cargo add pito-header --git https://github.com/gmrdad82/pito-header --tag @TAG@ --features crossterm
```

## Quick start

Build a nav, feed it keys and draw it at the top of the frame:

```rust
use pito_tui::header::{Group, Header, Key, Nav, Section};
use ratatui::{Frame, layout::Rect};

let mut nav = Nav::new(vec![
    Group::new("Mail").section(Section::new("Inbox")).section(Section::new("Drafts")),
    Group::new("Work").section(Section::new("Checks")),
]);
nav.key(Key::Char('3'));
assert_eq!(nav.place().number, 3);

fn draw(frame: &mut Frame, nav: &Nav) {
    let area = frame.area();
    let header = Header::new(nav).title(Some("Desk"));
    frame.render_widget(header, Rect { height: header.height(), ..area });
}
```

`header.height()` says how many rows the header takes for what it was given,
so the rest of the frame goes to your content.

## Run the demo

To see it move before you write any code, clone the repository and run the
demo shown on the [overview](index.md):

```sh
git clone https://github.com/gmrdad82/pito-header
cd pito-header
cargo run --example demo --features crossterm
```

`tab` and `shift+tab` change group, `]` and `[` change section, a digit jumps
to a section, `enter` drills in, `esc` goes back and `ctrl+c` pressed twice
quits. Two more examples show the other features:

```sh
cargo run --example fit --features crossterm
cargo run --example gallery --features crossterm -- rows
```

`fit` narrows the header with `<` or left and widens it with `>` or right,
so you can watch it abbreviate; `ctrl+c` quits. `gallery` takes `rows`,
`spans` or `drill` and draws that still; `q` quits.
