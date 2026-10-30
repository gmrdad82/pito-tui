---
title: Install
description: Add pito-list to a ratatui app on Linux, Windows or macOS, and run the demo.
sidebar:
  order: 2
---

pito-list is a library for [ratatui](https://ratatui.rs) apps. It isn't on
crates.io: add it from git, pinned to a release tag. The same line works on
Linux, Windows and macOS.

## Add the dependency

```sh
cargo add pito-list --git https://github.com/gmrdad82/pito-list --tag v1.0.1
```

or in `Cargo.toml`:

```toml
[dependencies]
pito-list = { git = "https://github.com/gmrdad82/pito-list", tag = "v1.0.1" }
```

The crate is edition 2024, so it needs Rust 1.85 or newer, and it draws with
ratatui 0.30. Its own dependencies are ratatui (with no backend feature),
unicode-width and unicode-segmentation.

## The `crossterm` feature

Turn it on to convert crossterm's key events into the list's keys with
`Key::from(event)`:

```sh
cargo add pito-list --git https://github.com/gmrdad82/pito-list --tag v1.0.1 --features crossterm
```

```toml
pito-list = { git = "https://github.com/gmrdad82/pito-list", tag = "v1.0.1", features = ["crossterm"] }
```

It brings in crossterm 0.29 with its `events` feature only. Without it the
crate has no backend dependency, and an app on another backend builds the
`Key` values itself.

## Quick start

Three rows, three columns, the cursor moved down once, drawn into a ratatui
buffer at two widths:

```rust
use pito_tui::list::{Cell, Column, Key, List, ListView, Row, Styles};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

let columns = [
    Column::new("Job", 8, 14).pinned(),
    Column::new("Owner", 5, 7).priority(1),
    Column::new("Message", 8, 0),
];
let mut list = List::new().with_rows([
    Row::new(["deploy api", "web", "uploading the bundle"]),
    Row::new([
        Cell::new("build docs"),
        Cell::new("docs"),
        Cell::new("waiting for a runner").faint(),
    ]),
    Row::new(["rotate logs", "ops", "starts at 02:00"]),
]);
list.key(Key::Down);

let styles = Styles::new().selected(Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD));
for width in [44, 24] {
    let area = Rect::new(0, 0, width, 4);
    let mut buffer = Buffer::empty(area);
    ListView::new(&mut list, &columns)
        .styles(styles)
        .header(true)
        .render(area, &mut buffer);
#   let lines: Vec<String> = (0..area.height)
#       .map(|y| (0..area.width).map(|x| buffer[(x, y)].symbol()).collect::<String>().trim_end().to_owned())
#       .collect();
#   let expected: &[&str] = if width == 44 {
#       &[
#           "  Job             Owner    Message",
#           "  deploy api      web      uploading the…",
#           "▌ build docs      docs     waiting for a…",
#           "  rotate logs     ops      starts at 02:00",
#       ]
#   } else {
#       &[
#           "  Job           Message",
#           "  deploy api    …",
#           "▌ build docs    waiting…",
#           "  rotate logs   starts…",
#       ]
#   };
#   assert_eq!(lines, expected);
}
```

It draws, at 44 and then 24 cells wide (the Owner column drops when it
doesn't fit):

```text
  Job             Owner    Message
  deploy api      web      uploading the…
▌ build docs      docs     waiting for a…
  rotate logs     ops      starts at 02:00

  Job           Message
  deploy api    …
▌ build docs    waiting…
  rotate logs   starts…
```

[Draw a list in your app](guides/first-list.md) puts a list in a real
terminal app, with its keys.

## Run the demo

The demo is a small terminal app over sample jobs, the one recorded in the
clips on these pages:

```sh
git clone https://github.com/gmrdad82/pito-list
cd pito-list
cargo run --example demo --features crossterm
```

Move with the arrows or `j` and `k`, press `v` for a range, ctrl+d and ctrl+u
for half a page, Page Down and Page Up for a whole one, `g` and `G` for the
ends, enter to open, and ctrl+c twice to quit.
