---
title: Draw a list in your app
description: Keep a List in your app's state, draw it in a ratatui frame and pass it your keys.
sidebar:
  order: 1
---

A list is two things: a `List`, which holds the rows, the selection and the
scroll offset and lives in your app's state, and a `ListView`, the widget that
draws it in one frame with your columns, words and styles.

## Rows and columns

A `Row` is a list of cells, one for each column. A `Column` has a title, a
minimum width, a preferred width (`0` for none) and options such as
`pinned()` or `priority(n)`; the last column takes whatever width is left.

```rust
use pito_tui::list::{Column, Key, List, ListView, Row, Step};
use ratatui::{Frame, Terminal, backend::TestBackend};

const COLUMNS: [Column; 2] = [
    Column::new("Name", 6, 12).pinned(),
    Column::new("Status", 6, 0),
];

struct App {
    list: List,
}

fn draw(frame: &mut Frame, app: &mut App) {
    let view = ListView::new(&mut app.list, &COLUMNS)
        .header(true)
        .end(Some("· end ·"));
    frame.render_widget(view, frame.area());
}

let mut app = App {
    list: List::new().with_rows([
        Row::new(["alpha", "ready"]),
        Row::new(["beta", "building"]),
        Row::new(["gamma", "failed"]),
    ]),
};
assert_eq!(app.list.key(Key::Down), Step::Moved);

let mut terminal = Terminal::new(TestBackend::new(30, 6))?;
terminal.draw(|frame| draw(frame, &mut app))?;
# let buffer = terminal.backend().buffer();
# let lines: Vec<String> = (0..6)
#     .map(|y| (0..30).map(|x| buffer[(x, y)].symbol()).collect::<String>().trim_end().to_owned())
#     .collect();
# assert_eq!(lines, [
#     "  Name          Status",
#     "  alpha         ready",
#     "▌ beta          building",
#     "  gamma         failed",
#     "  · end ·",
#     "",
# ]);
# Ok::<(), Box<dyn std::error::Error>>(())
```

The frame shows:

```text
  Name          Status
  alpha         ready
▌ beta          building
  gamma         failed
  · end ·
```

The first selectable row is selected when the rows arrive, so one `Down`
moves the "▌ " marker to `beta`. The header and the end line are opt-in, and
their words are yours: the crate has none of its own.

## In a real terminal

With ratatui's own `crossterm` backend and pito-list's `crossterm` feature
(see [Install](../install.md)), the app loop draws the list, reads a key and
hands it over:

```rust,no_run
# use pito_tui::list::{Column, Key, List, ListView, Row, Step};
# use ratatui::Frame;
# const COLUMNS: [Column; 2] = [Column::new("Name", 6, 12).pinned(), Column::new("Status", 6, 0)];
# struct App { list: List }
# fn draw(frame: &mut Frame, app: &mut App) {
#     frame.render_widget(ListView::new(&mut app.list, &COLUMNS).header(true), frame.area());
# }
use crossterm::event::{self, Event, KeyCode};

fn main() -> std::io::Result<()> {
    let mut app = App {
        list: List::new().with_rows([Row::new(["alpha", "ready"]), Row::new(["beta", "building"])]),
    };
    ratatui::run(|terminal| loop {
        terminal.draw(|frame| draw(frame, &mut app))?;
        if let Event::Key(press) = event::read()? {
            if press.code == KeyCode::Char('q') {
                return Ok(());
            }
            if let Step::Open(index) = app.list.key(Key::from(press)) {
                println!("open row {index}");
            }
        }
    })
}
```

`List::key` answers with a `Step`: `Moved` when the selection moved, `Held`
for a list key that moved nothing (up on the first row), `Open(index)` for
enter, and `Pass` for a key that isn't the list's, which your app can use for
its own commands. [Handle keys and clicks](keys-and-clicks.md) has the
details.

## Changing the rows

Replace the rows with `set_rows` when your data changes: the selection stays
on its index, clamped to the new length. `push` adds one row and `clear`
empties the list. To change a few texts or colours between frames without
building new rows, see [Update rows in place](live-rows.md).
