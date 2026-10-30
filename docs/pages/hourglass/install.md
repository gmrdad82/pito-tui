---
title: Install
description: Add pito-hourglass to a ratatui app on Linux, Windows or macOS and draw the first hourglass.
sidebar:
  order: 2
---

pito-hourglass is a Rust library for [ratatui](https://ratatui.rs) 0.30 apps (0.30.1 or later).
It isn't on crates.io; add it from its repository, pinned to a release tag.

## Linux, Windows and macOS

The same command works on all three, in any shell, from the app's folder:

```sh
cargo add pito-hourglass --git https://github.com/gmrdad82/pito-hourglass --tag @TAG@
```

```text
    Updating git repository `https://github.com/gmrdad82/pito-hourglass`
      Adding pito-hourglass (git) to dependencies
```

or by hand in `Cargo.toml`:

```toml
[dependencies]
pito-hourglass = { git = "https://github.com/gmrdad82/pito-hourglass", tag = "@TAG@" }
ratatui = "0.30.1"
```

It needs nothing else: no backend, no build script, no system library. Its two
dependencies (ratatui 0.30.1 or later with no default features, and
unicode-segmentation) are already in a ratatui app's graph.

## Quick start

Keep the moment the wait started and draw the widget from the time since:

```rust
use std::time::Instant;

use pito_tui::hourglass::Hourglass;
use ratatui::{
    Frame,
    style::{Color, Modifier, Style},
};

fn draw(frame: &mut Frame, started: Instant) {
    let hourglass = Hourglass::new(started.elapsed())
        .label("Loading the data...")
        .hint(Some("esc stops waiting"))
        .glass(Style::new().fg(Color::Yellow))
        .accent(Style::new().fg(Color::Magenta))
        .muted(Style::new().fg(Color::DarkGray).add_modifier(Modifier::DIM));
    frame.render_widget(hourglass, frame.area());
}
```

Redraw about every 50 ms while it shows; the picture depends only on the
elapsed time.

## Run the example

From a clone, the demo draws exactly that, full screen. Esc or q quits:

```sh
git clone https://github.com/gmrdad82/pito-hourglass
cd pito-hourglass
cargo run --example demo
```

![The demo: the hourglass drains and flips while "Loading the data..." shimmers under it](images/demo.gif)

Next: [show it while data loads](guides/show-while-loading.md).
