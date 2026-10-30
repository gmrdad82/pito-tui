---
title: Install and first app
description: Add pito-tui to a Rust project on Linux, macOS or Windows, and run a first screen.
sidebar:
  order: 1
---

pito-tui is a Rust library, edition 2024, and it needs Rust 1.88 or newer. With
a current stable Rust installed ([rustup](https://rustup.rs) on every
platform), the same lines work on Linux, macOS and Windows, in a shell or in
PowerShell:

```sh
cargo new hello
cd hello
cargo add pito-tui --git https://github.com/gmrdad82/pito-tui --tag {{tag}}
```

```text
    Updating git repository `https://github.com/gmrdad82/pito-tui`
      Adding pito-tui (git) to dependencies
```

or by hand in `Cargo.toml`:

```toml
[dependencies]
pito-tui = { git = "https://github.com/gmrdad82/pito-tui", tag = "{{tag}}" }
```

The crate is not on crates.io; it is pinned by its git tag. That one line
brings ratatui 0.30 and crossterm 0.29, reachable as `pito_tui::ratatui` and
`pito_tui::crossterm`, and the header, footer, hourglass and list are part of
the crate, as `pito_tui::header`, `pito_tui::footer`, `pito_tui::hourglass` and
`pito_tui::list`, so the app needs no other line for them.

## A first screen

Put this in `src/main.rs`:

```rust,no_run
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

## The widgets on their own

The header, footer, hourglass and list also draw on their own into any
ratatui frame or buffer, for an app that keeps its own loop or wants one of
them inside a screen. They need nothing beyond the same pito-tui line: each
draws straight into ratatui's buffer, and the header's, footer's and list's
`Key` comes from crossterm's `KeyEvent` with `Key::from`.

### The header on its own

Build a nav, feed it keys and draw it at the top of the frame:

```rust
use pito_tui::header::{Group, Header, Key, Nav, Section};
use pito_tui::ratatui::{Frame, layout::Rect};

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

To see it move before you write any code, run the demo shown on
[the header's overview](../header/) from a checkout:

```sh
cargo run --example header_demo
```

`tab` and `shift+tab` change group, `]` and `[` change section, a digit jumps
to a section, `enter` drills in, `esc` goes back and `ctrl+c` pressed twice
quits. Two more examples show the other features:

```sh
cargo run --example header_fit
cargo run --example header_gallery -- rows
```

`header_fit` narrows the header with `<` or left and widens it with `>` or
right, so you can watch it abbreviate; `ctrl+c` quits. `header_gallery` takes
`rows`, `spans` or `drill` and draws that still; `q` quits.

### The footer on its own

Put hints at the bottom of the screen. The footer says how many rows it needs
for a width with `height(width)`, and the app gives it those rows:

```rust,no_run
use pito_tui::crossterm::event;
use pito_tui::footer::{Footer, Hint};
use pito_tui::ratatui::layout::Rect;

const HINTS: [Hint; 3] = [
    Hint::new("↑↓", "move"),
    Hint::new("enter", "open").rank(1),
    Hint::new("q", "quit").pinned(),
];

fn main() -> std::io::Result<()> {
    let mut terminal = pito_tui::ratatui::init();
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
    pito_tui::ratatui::restore();
    Ok(())
}
```

`cargo run` draws `↑↓ move · enter open · q quit` on the last row with
`app v1.0.0` at its right end; `q` quits. To turn crossterm key events into the
footer's `Key`, see [Feed it keys from crossterm](../footer/guides/keys/).

The footer's demo is a task list with the footer at its bottom: move with
↑↓, mark a task done with enter, search with `/`, delete with `d`, hide the
hints with `?`, and press ctrl+c twice to quit.

```sh
cargo run --example footer_demo
```

The gallery draws one scene for each part of the footer, the scenes the
pictures in the footer's guides come from:

```sh
cargo run --example footer_gallery -- fit
```

The scenes are `fit`, `notices`, `status`, `input` and `confirm`; any other
name prints `gallery: unknown scene '…', try fit, notices, status, input or
confirm` and exits with 2.

### The hourglass on its own

Keep the moment the wait started and draw the widget from the time since:

```rust
use std::time::Instant;

use pito_tui::hourglass::Hourglass;
use pito_tui::ratatui::{
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
elapsed time. From a checkout, the demo draws exactly that, full screen. Esc
or q quits:

```sh
cargo run --example hourglass_demo
```

![The demo: the hourglass drains and flips while "Loading the data..." shimmers under it](images/hourglass/demo.gif)

Next for the hourglass: [show it while data loads](../hourglass/guides/show-while-loading/).

### The list on its own

Three rows, three columns, the cursor moved down once, drawn into a ratatui
buffer at two widths:

```rust
use pito_tui::list::{Cell, Column, Key, List, ListView, Row, Styles};
use pito_tui::ratatui::{
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
#           "  deploy api    uploadi…",
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
  deploy api    uploadi…
▌ build docs    waiting…
  rotate logs   starts…
```

`Key::from(event)` turns crossterm's key events into the list's keys; an app
on another backend builds the `Key` values itself.
[Draw a list in your app](../list/guides/first-list/) puts a list in a real
terminal app, with its keys.

The list's demo is a small terminal app over sample jobs, the one recorded in
the clips on the list's pages:

```sh
cargo run --example list_demo
```

Move with the arrows or `j` and `k`, press `v` for a range, ctrl+d and ctrl+u
for half a page, Page Down and Page Up for a whole one, `g` and `G` for the
ends, enter to open, and ctrl+c twice to quit.

Next: [Screens, tabs and the header](../guides/screens/).
