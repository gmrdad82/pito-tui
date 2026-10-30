---
title: Install and first use
description: Add pito-footer to a ratatui app on Linux, Windows or macOS and draw a first footer.
sidebar:
  order: 1
---

pito-footer is a Rust library for [ratatui](https://ratatui.rs) 0.30. It is
taken from its Git repository by tag, and the line is the same on Linux,
Windows and macOS. In the app's `Cargo.toml`:

```toml
[dependencies]
pito-footer = { git = "https://github.com/gmrdad82/pito-footer", tag = "v1.0.1" }
ratatui = "0.30"
```

To turn crossterm key events into the crate's `Key` (see
[Feed it keys from crossterm](guides/keys.md)), add the `crossterm` feature:

```toml
pito-footer = { git = "https://github.com/gmrdad82/pito-footer", tag = "v1.0.1", features = ["crossterm"] }
```

Without it the crate has no backend dependency.

## A first footer

Put hints at the bottom of the screen. The footer says how many rows it needs
for a width with `height(width)`, and the app gives it those rows:

```rust
use pito_tui::footer::{Footer, Hint};
use ratatui::{crossterm::event, layout::Rect};

const HINTS: [Hint; 3] = [
    Hint::new("↑↓", "move"),
    Hint::new("enter", "open").rank(1),
    Hint::new("q", "quit").pinned(),
];

fn main() -> std::io::Result<()> {
    let mut terminal = ratatui::init();
    loop {
        terminal.draw(|frame| {
            let footer = Footer::new(&HINTS).version("app", "1.0.0");
            let area = frame.area();
            let height = footer.height(area.width);
            let bottom = Rect { y: area.bottom().saturating_sub(height), height, ..area };
            frame.render_widget(footer, bottom);
        })?;
        let quit = event::read()?
            .as_key_press_event()
            .is_some_and(|key| key.code == event::KeyCode::Char('q'));
        if quit {
            break;
        }
    }
    ratatui::restore();
    Ok(())
}
```

`cargo run` draws `↑↓ move · enter open · q quit` on the last row with
`app v1.0.0` at its right end; `q` quits.

## See every part working

The repository's demo is a task list with the footer at its bottom: move with
↑↓, mark a task done with enter, search with `/`, delete with `d`, hide the
hints with `?`, and press ctrl+c twice to quit.

```sh
git clone https://github.com/gmrdad82/pito-footer && cd pito-footer
cargo run --example demo --features crossterm
```

The gallery draws one scene for each part of the footer, the scenes the
pictures in these guides come from:

```sh
cargo run --example gallery --features crossterm -- fit
```

The scenes are `fit`, `notices`, `status`, `input` and `confirm`; any other
name prints `gallery: unknown scene '…', try fit, notices, status, input or
confirm` and exits with 2.
