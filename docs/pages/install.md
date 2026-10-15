---
title: Install and first app
description: Add pito-tui to a Rust project on Linux, macOS or Windows, and run a first screen.
sidebar:
  order: 1
---

pito-tui is a Rust library. With a current stable Rust installed
([rustup](https://rustup.rs) on every platform), the same lines work on Linux,
macOS and Windows, in a shell or in PowerShell:

```sh
cargo new hello
cd hello
cargo add pito-tui --git https://github.com/gmrdad82/pito-tui --tag {{tag}}
```

or by hand in `Cargo.toml`:

```toml
[dependencies]
pito-tui = { git = "https://github.com/gmrdad82/pito-tui", tag = "{{tag}}" }
```

The crate is not on crates.io; it is pinned by its git tag. That one line
brings ratatui 0.30, crossterm 0.29, pito-header v1.0.1, pito-footer v1.0.1,
pito-hourglass v1.0.1 and pito-list v1.0.1, reachable as
`pito_tui::ratatui`, `pito_tui::crossterm`, `pito_tui::header`,
`pito_tui::footer`, `pito_tui::hourglass` and `pito_tui::list`, so the app
needs no other line for them.

## A first screen

Put this in `src/main.rs`:

```rust
use pito_tui::crossterm::event::KeyEvent;
use pito_tui::footer::Hint;
use pito_tui::header::Section;
use pito_tui::ratatui::{Frame, layout::Rect, style::Color, text::Line};
use pito_tui::{Cx, Palette, Screen, Tui, Words, message};

struct Hello {
    presses: usize,
}

impl Screen<()> for Hello {
    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        let text = format!("{} keys pressed", self.presses);
        message(frame, area, Line::styled(text, palette.ink));
    }

    fn key(&mut self, _key: KeyEvent, _cx: &mut Cx<'_, ()>) {
        self.presses += 1;
    }

    fn hints(&self) -> Vec<Hint<'_>> {
        vec![Hint::new("any key", "count")]
    }
}

fn main() -> std::io::Result<()> {
    let words = Words::new()
        .help("? help")
        .again("ctrl+c again to quit")
        .yes("Yes")
        .no("No")
        .too_small("Make the window bigger");
    Tui::new("hello", env!("CARGO_PKG_VERSION"), Color::Rgb(0x5b, 0x8c, 0xff))
        .words(words)
        .hints([Hint::new("ctrl+c", "twice quit").pinned()])
        .screen(Section::new("Hello"), Hello { presses: 0 })
        .run()
}
```

Then `cargo run`: a titled screen with a key count, the hints and the version
in the footer, and `ctrl+c` twice to quit. This is the same program the crate
tests as a doctest on every change.

What each part does:

- `Tui::new(name, version, accent)` starts the shell; the accent is the one
  colour the whole palette is derived from.
- `Words` holds the few words the shell draws itself. It has none of its own:
  a word the app leaves empty is simply not drawn.
- `.screen(section, screen)` adds a tab. A `Screen` only has to `draw`; every
  other method has a default.
- `.run()` takes the terminal, runs the 120 fps loop until the user quits,
  and gives the terminal back, on an error or a panic too.

## The demo

The demo shows every feature live. From a checkout:

```sh
git clone https://github.com/gmrdad82/pito-tui
cd pito-tui
cargo run --release --example demo
```

`ctrl+k` opens a "Go to" list that reaches every screen and layer, `j` starts
a job, `c` runs a real command, `p` picks the accent, and `ctrl+c` twice
quits. `-- --stack` shows the header as a row stack with groups, and
`-- --mono` the monochrome palette.

The demo also draws itself without a terminal, which is how the frames on
these pages were taken:

```text
$ cargo run --release --example demo -- --dump 72x14 --keys "tab"
 ──────────────────────────────── demo ──────────────── workspace one ─
                    1 Home  2 What to do next  3 Log
                                5 steps
 ─ What to do next ────────────────────────────────────────────────────
    Step                        How
  ▌ Replace this screen         Home and Next are two Screen types; s…
    Set your accent             Tui::new takes one colour, or Palette…
    Add your first command      a key in Screen::key, its work in Cx:…
    Load real data              Screen::load returns the read; Screen…
    Wire the headless flags     --dump, --keys, --at, --bench, --capt…

 ──────────────────────────────────────────────────────────────────────
 ↑↓ move · enter open · / filter · ctrl+k go to · ctrl+c twice quit
```

Next: [Screens, tabs and the header](../guides/screens/).
