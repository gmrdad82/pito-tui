---
title: Group rows under headings
description: Split a list into sections with headings and blank lines, put a mark in front of each row, and build cells from styled parts.
sidebar:
  order: 4
---

Rows can be grouped under headings, with blank lines between the groups. The
cursor, the keys, paging and clicks skip both, so only real rows are ever
selected.

## Headings, blank lines and marks

`Row::heading(parts)` is a line drawn across the row from where the marks
start, each part in its own style (a bold title and a faint count, say).
`Row::blank()` is an empty line. `Row::mark(Mark::new(text))` puts a mark
such as a state sign in front of a row's cells; the widest mark sets one mark
column for the whole list, so the cells still line up.

```rust
use pito_tui::list::{Cell, Column, List, ListView, Mark, Part, Row};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

let bold = Style::new().add_modifier(Modifier::BOLD);
let done = Style::new().fg(Color::Green);
let mut list = List::new().with_rows([
    Row::heading([Part::new("Running").style(bold), Part::new(" · 1").faint()]),
    Row::new([
        Cell::new("deploy api"),
        Cell::parts([Part::new("v1.4"), Part::new(" → v1.5").faint()]),
    ])
    .mark(Mark::new("⧗").style(Style::new().fg(Color::Yellow))),
    Row::blank(),
    Row::heading([Part::new("Done").style(bold), Part::new(" · 2").faint()]),
    Row::new(["update tool", "v0.2"]).mark(Mark::new("✓").style(done)),
    Row::new(["rotate logs", "v3.0"]).mark(Mark::new("✓").style(done)),
]);
assert_eq!(list.selected(), Some(1));

let columns = [Column::new("Job", 10, 14).pinned(), Column::new("Version", 7, 0)];
let area = Rect::new(0, 0, 34, 6);
let mut buffer = Buffer::empty(area);
ListView::new(&mut list, &columns).render(area, &mut buffer);
# let lines: Vec<String> = (0..area.height)
#     .map(|y| (0..area.width).map(|x| buffer[(x, y)].symbol()).collect::<String>().trim_end().to_owned())
#     .collect();
# assert_eq!(lines, [
#     "  Running · 1",
#     "▌ ⧗ deploy api      v1.4 → v1.5",
#     "",
#     "  Done · 2",
#     "  ✓ update tool     v0.2",
#     "  ✓ rotate logs     v3.0",
# ]);
```

draws:

```text
  Running · 1
▌ ⧗ deploy api      v1.4 → v1.5

  Done · 2
  ✓ update tool     v0.2
  ✓ rotate logs     v3.0
```

The list selects its first selectable row, `deploy api` at index 1, past the
heading. Moving down from it skips the blank line and the second heading and
lands on `update tool`. Headings and blank lines count as rows for indices,
`len()`, `select` and the count of the rows on screen.

When the view follows the selection up to the first row of a group, the
group's heading comes into view above it. `Row::selectable()` tells a real
row from a heading or a blank line.

## Cells of several parts

`Cell::parts(parts)` builds one cell from `Part`s, each with its own style;
a part without one takes the cell's. The parts pad, align and clip as one
cell, and a text wider than the column ends in "…" in the style of the part
it cuts. A selected row draws every part in the selected style, unless the
list keeps cell colours (see [Select a range](ranges.md)).

Cells, parts and marks take the list's ink style by default, `faint()` for
secondary text, or `style(..)` for their own; [Theme the list](styles.md)
sets ink and faint for the whole list.
