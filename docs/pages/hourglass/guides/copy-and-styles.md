---
title: Your copy and your colours
description: Set the label, the hint and the glass, accent and muted styles, in any script.
sidebar:
  order: 2
---

![Four hourglasses at once, each a quarter turn apart: "Loading the data..." in magenta, "Se încarcă datele…" in blue, a Japanese label in green with no hint, and "Syncing 3 inboxes" in yellow](../images/gallery.gif)

Every word and every style is the app's. The widget has five setters, each
taking and returning the widget:

| Setter | What it sets | Default |
|---|---|---|
| `label(&str)` | the copy under the glass, which shimmers | `"Loading..."` |
| `hint(Option<&str>)` | a second line under the label, which never shimmers | `None` |
| `glass(Style)` | the hourglass itself | `Style::new()` |
| `accent(Style)` | the shimmer's band, usually the app's accent | `Style::new()` |
| `muted(Style)` | the rest of the label, and the whole hint | `Style::new()` |

```rust
use std::time::Duration;

use pito_tui::hourglass::Hourglass;
use ratatui::style::{Color, Modifier, Style};

fn syncing(elapsed: Duration) -> Hourglass<'static> {
    Hourglass::new(elapsed)
        .label("Syncing 3 inboxes")
        .hint(Some("q quits"))
        .glass(Style::new().fg(Color::Magenta))
        .accent(Style::new().fg(Color::LightYellow))
        .muted(Style::new().fg(Color::DarkGray).add_modifier(Modifier::DIM))
}
```

## The shimmer

A three-character band sweeps across the label, left to right, in 1.2 s, then
pauses for 0.3 s. Its centre character is the accent style plus bold, its two
neighbours are the accent style as given, and every other character is muted.
The sweep starts two characters before the label and ends two after it, so the
band slides in and out rather than popping.

An empty hint (`Some("")`) is the same as no hint.

## Any script

Widths come from Unicode, so the label and hint centre exactly whatever they
hold: Romanian diacritics, "…", two-cell CJK glyphs, combining marks. The band
counts characters (grapheme clusters), not bytes or cells, so a two-cell glyph
shimmers as one character and a combining mark rides with its letter. Control
characters are dropped.

## Several at once

The gallery above draws four hourglasses, one a quarter, each a quarter turn
behind the last by adding to the elapsed time:

```rust
use std::time::Duration;

use pito_tui::hourglass::{CYCLE, Hourglass};

fn lagged(elapsed: Duration, index: u32) -> Hourglass<'static> {
    Hourglass::new(elapsed + CYCLE * index / 4)
}
```

From a clone, `cargo run --example gallery` draws it; Esc or q quits.
