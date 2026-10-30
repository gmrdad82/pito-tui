---
title: Show it while data loads
description: Draw the hourglass in a ratatui app's loop while a background job runs, then swap in the result.
sidebar:
  order: 1
---

The hourglass keeps no state. The app keeps one thing, the `Instant` the wait
started, and builds a fresh `Hourglass` from `started.elapsed()` every frame.
Two rules make it look right:

- **Redraw on a timer while it shows,** about every 50 ms, not only on input:
  the frames change every 200 to 430 ms and the shimmer moves faster.
- **Keep the same start for the whole wait.** A new `Instant` restarts the
  glass at the top; a redraw at any moment with the same start lands on the
  same picture.

## A full app

This app loads its data on a thread, shows the hourglass until the data
arrives, then shows the data. Esc or q quits at any time.

```rust
use std::{
    io,
    sync::mpsc,
    thread,
    time::{Duration, Instant},
};

use pito_tui::hourglass::Hourglass;
use pito_tui::ratatui::{
    DefaultTerminal, Frame,
    crossterm::event::{self, Event, KeyCode, KeyEventKind},
    style::{Color, Modifier, Style},
    widgets::Paragraph,
};

fn main() -> io::Result<()> {
    pito_tui::ratatui::run(run)
}

fn run(terminal: &mut DefaultTerminal) -> io::Result<()> {
    let (done, loaded) = mpsc::channel();
    thread::spawn(move || {
        thread::sleep(Duration::from_secs(5));
        let _ = done.send(String::from("42 rows loaded"));
    });
    let started = Instant::now();
    let mut data = None;
    loop {
        if data.is_none() {
            data = loaded.try_recv().ok();
        }
        terminal.draw(|frame| draw(frame, started, data.as_deref()))?;
        if event::poll(Duration::from_millis(50))?
            && let Event::Key(key) = event::read()?
            && key.kind == KeyEventKind::Press
            && matches!(key.code, KeyCode::Esc | KeyCode::Char('q'))
        {
            return Ok(());
        }
    }
}

fn draw(frame: &mut Frame, started: Instant, data: Option<&str>) {
    match data {
        Some(data) => frame.render_widget(Paragraph::new(data), frame.area()),
        None => frame.render_widget(
            Hourglass::new(started.elapsed())
                .label("Loading the data...")
                .hint(Some("esc stops waiting"))
                .glass(Style::new().fg(Color::Yellow))
                .accent(Style::new().fg(Color::Magenta))
                .muted(Style::new().fg(Color::DarkGray).add_modifier(Modifier::DIM)),
            frame.area(),
        ),
    }
}
```

`event::poll` with a 50 ms timeout is the redraw timer: it returns early on a
key, and otherwise the loop draws the next picture. The crossterm it uses is
ratatui's own (`pito_tui::ratatui::crossterm`), the same crate as
`pito_tui::crossterm`; the hourglass itself reads no terminal.

## Part of the screen

The widget draws into whatever `Rect` it's given and centres itself there, so
it can fill a pane while the rest of the screen stays live:

```rust
use std::time::Instant;

use pito_tui::hourglass::Hourglass;
use pito_tui::ratatui::{
    Frame,
    layout::{Constraint, Layout},
    widgets::{Block, Paragraph},
};

fn draw(frame: &mut Frame, started: Instant) {
    let [list, detail] =
        Layout::horizontal([Constraint::Length(30), Constraint::Fill(1)]).areas(frame.area());
    frame.render_widget(Paragraph::new("inbox\narchive").block(Block::bordered()), list);
    let pane = Block::bordered().title("Message");
    let inner = pane.inner(detail);
    frame.render_widget(pane, detail);
    frame.render_widget(Hourglass::new(started.elapsed()).label("Opening..."), inner);
}
```

What it draws when the pane is small is in [Small areas](../small-areas/).

## By reference

`Hourglass` is a 96-byte `Copy` value and a `Widget` both by value and by
reference, so an app can build it once a frame and render `&hourglass` if it
needs the value again afterwards.
