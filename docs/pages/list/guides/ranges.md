---
title: Select a range
description: Draw every row between an anchor and the cursor as selected, for a Visual mode, and keep cell colours under the selection.
sidebar:
  order: 5
---

![The cursor moves down into the Queued section, v starts a range, and every row from the anchor to the cursor draws selected while the section headings stay as they are](../images/range.gif)

A range is the app's: the list draws it but never changes it. Keep an anchor
in your app state when the user starts a Visual mode, and hand the list the
anchor and the cursor after every key.

## Anchor and cursor

`List::select_range(Some((from, to)))` draws every selectable row from `from`
to `to`, in either order and both included, in the selected style. The "▌ "
marker stays on the selected row alone, so the cursor still shows inside the
range. Headings and blank lines inside it draw as they are. `None`, the
default, goes back to the one selected row.

```rust
use pito_tui::list::{Key, List, ListView, Column, Row, Styles};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Style},
    widgets::Widget,
};

let mut list = List::new().with_rows((1..=6).map(|n| Row::new([format!("item {n}")])));
let anchor = list.selected();
list.key(Key::Down);
list.key(Key::Down);
list.select_range(anchor.zip(list.selected()));
assert_eq!(list.range(), Some((0, 2)));

let styles = Styles::new().selected(Style::new().bg(Color::Blue));
let area = Rect::new(0, 0, 12, 6);
let mut buffer = Buffer::empty(area);
ListView::new(&mut list, &[Column::new("Item", 6, 0)])
    .styles(styles)
    .render(area, &mut buffer);
# let lines: Vec<String> = (0..area.height)
#     .map(|y| (0..area.width).map(|x| buffer[(x, y)].symbol()).collect::<String>().trim_end().to_owned())
#     .collect();
# assert_eq!(lines, ["  item 1", "  item 2", "▌ item 3", "  item 4", "  item 5", "  item 6"]);
# let blue: Vec<bool> = (0..6).map(|y| buffer[(5, y)].style().bg == Some(Color::Blue)).collect();
# assert_eq!(blue, [true, true, true, false, false, false]);

list.select_range(None);
```

Rows 1 to 3 draw on blue, with the marker on row 3, the cursor:

```text
  item 1
  item 2
▌ item 3
  item 4
  item 5
  item 6
```

Keys, paging, `select`, `set_rows`, `update` and `clear` all leave the range
as it was set; only `select_range` changes it. Indices past the end of the
list simply have no rows to draw. `range()` reads it back.

## Keep cell colours under the selection

By default the selected style replaces each cell's, part's and mark's own
style on the selected row and the range. `Styles::keep_colours(true)` patches
the selected style over them instead, so a selection of a background and bold
keeps a red number red and a faint note faint. Whatever the selected style
does set still wins: a foreground it sets colours every cell.

```rust
use pito_tui::list::Styles;
use ratatui::style::{Color, Modifier, Style};

let styles = Styles::new()
    .selected(Style::new().bg(Color::DarkGray).add_modifier(Modifier::BOLD))
    .cursor(Style::new().fg(Color::Yellow))
    .keep_colours(true);
```

`Styles::cursor(style)` gives the marker a colour of its own, so a
bold-only selection that keeps the cell colours can still colour the marker.
[Theme the list](styles.md) has every style.
