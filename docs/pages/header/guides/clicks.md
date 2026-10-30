---
title: Handle clicks
description: Turn a mouse click into a move by asking the header which group or section is under the pointer.
sidebar:
  order: 16
---

The crate never reads the mouse. It answers where a click landed:
`hit(area, column, row)` names the group or section under the pointer, or
`None`. Give it the same area the header was drawn in and the click's
terminal column and row; the answer is a `Place` to move to.

```rust
use pito_tui::header::{Header, Nav};
use ratatui::layout::Rect;

fn click(nav: &mut Nav, area: Rect, column: u16, row: u16) {
    let place = Header::new(nav).title(Some("Desk")).hit(area, column, row);
    if let Some(place) = place {
        nav.go_to(place.group, place.section);
    }
}
```

Build the header for the hit test with the same rows as the one you drew
(here the title), so the rows line up. Then:

- A click on a group answers that group at the section it remembers.
- A click on a section answers that section.
- A click outside the area, on another row, or on the breadcrumb rule while
  drilled in answers `None`.

`NavBar::hit` does the same for a nav bar drawn on its own. A hit test reads
the layout the way drawing does and allocates nothing; it takes about a
tenth of a microsecond ([Measure draw time](measure.md)).

With crossterm, the column and row come from a mouse event:

```rust
use crossterm::event::{Event, MouseButton, MouseEventKind};
use pito_tui::header::{Header, Nav};
use ratatui::layout::Rect;

fn on_event(nav: &mut Nav, area: Rect, event: Event) {
    if let Event::Mouse(mouse) = event
        && mouse.kind == MouseEventKind::Down(MouseButton::Left)
        && let Some(place) = Header::new(nav).hit(area, mouse.column, mouse.row)
    {
        nav.go_to(place.group, place.section);
    }
}
```

The terminal sends mouse events only once the app turns mouse capture on
(crossterm's `EnableMouseCapture`).
