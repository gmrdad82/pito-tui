---
title: Style a section in several colours
description: Give a section label several styled spans, such as a host's name next to its state, and change it while the app runs.
sidebar:
  order: 15
---

![A Fleet header: under Hosts, the sections web-1 up and web-2 up with green dots, db down with a red dot and lit as current, and cache busy with a yellow dot; the facts rule reads 2 up, 1 down, 1 busy, checked 4s ago](../../images/header/spans.png)

A section label can be several spans, each in its own style.
`Section::spans` takes them as `(text, Style)` pairs, and `short` or
`short_spans` gives the form it falls back to when the width runs out:

```rust
use pito_tui::header::{Group, Nav, Section};
use pito_tui::ratatui::style::{Color, Style};

const UP: Style = Style::new().fg(Color::Green);
const DOWN: Style = Style::new().fg(Color::Red);

let nav = Nav::new(vec![
    Group::new("Hosts")
        .section(Section::spans(&[("web-1 ", Style::new()), ("● up", UP)]).short("web-1"))
        .section(Section::spans(&[("db ", Style::new()), ("● down", DOWN)])
            .short_spans(&[("db ", Style::new()), ("●", DOWN)])),
]);
assert_eq!(nav.groups()[0].sections()[0].name(), "web-1 ● up");
```

Each span draws over the cell's style, so the current section keeps its
accent and bold under the span's colour. The image above is
`cargo run --example header_gallery -- spans`.

## Change a label while the app runs

`section_mut(group, section)` reaches a section in place, and `set_name`,
`set_spans`, `set_short` and `set_short_spans` change its label. The place,
every group's remembered section and every drill stack stay as they were:

```rust
use pito_tui::header::{Group, Nav, Section};
use pito_tui::ratatui::style::{Color, Style};

let mut nav = Nav::new(vec![
    Group::new("Hosts").section(Section::new("db")).section(Section::new("cache")),
]);
nav.go(2);
nav.open("connections", 5);
if let Some(db) = nav.section_mut(0, 0) {
    db.set_spans(&[("db ", Style::new()), ("● down", Style::new().fg(Color::Red))]);
}
assert_eq!(nav.place().number, 2);
assert_eq!(nav.depth(), 1);
```

`section_mut` answers `None` for indexes past the end.
