---
title: Theme the list
description: Set the list's styles from your app's theme, from the selected row and the header to the page background and the scrollbar.
sidebar:
  order: 7
---

Every style the list draws with comes from your app, through one `Styles`
value handed to the `ListView`. Anything left unset falls back to the
terminal's own look.

```rust
use pito_tui::list::{Column, List, ListView, Row, Styles};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

let styles = Styles::new()
    .selected(Style::new().bg(Color::DarkGray).add_modifier(Modifier::BOLD))
    .ink(Style::new().fg(Color::White))
    .faint(Style::new().fg(Color::Gray).add_modifier(Modifier::DIM))
    .base(Style::new().bg(Color::Black))
    .header(Style::new().fg(Color::Gray).bg(Color::Indexed(236)))
    .header_key(Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD))
    .cursor(Style::new().fg(Color::Yellow));

let columns = [Column::new("Name", 6, 10), Column::new("Size", 4, 0).right()];
let mut list = List::new().with_rows([Row::new(["alpha", "4 kB"]), Row::new(["beta", "12 kB"])]);
let area = Rect::new(0, 0, 24, 3);
let mut buffer = Buffer::empty(area);
ListView::new(&mut list, &columns)
    .styles(styles)
    .header(true)
    .key_column(Some(1))
    .render(area, &mut buffer);
# assert_eq!(buffer[(20, 0)].style().fg, Some(Color::Yellow));
# assert_eq!(buffer[(0, 1)].style().fg, Some(Color::Yellow));
# assert_eq!(buffer[(3, 1)].style().bg, Some(Color::DarkGray));
# assert_eq!(buffer[(3, 2)].style().bg, Some(Color::Black));
```

| Style | What it paints | Unset |
|---|---|---|
| `selected` | the selected row and the range, across the full width | the terminal's look |
| `ink` | cells and marks with no style of their own | the terminal's look |
| `faint` | `faint()` cells, parts and marks; the header, the end and empty lines | the terminal's look, so set it (to dim, say) |
| `base` | every cell of the list's area first, so a page background shows under the list | the terminal's background |
| `header` | the column titles, painted across the whole header line | faint |
| `header_key` | the title of the column `ListView::key_column(Some(index))` names, such as the sort key | the header's style |
| `cursor` | the "▌ " marker | the selected style |
| `keep_colours(true)` | patches the selected style over cell colours instead of replacing them | off |
| `track` | the scrollbar's track | faint |
| `thumb` | the scrollbar's thumb | the cursor style, else the selected one |
| `count` | the count of the rows on screen | faint |

Every style is drawn over the base, and the selected row's style wins on its
row: the base shows through only what the selected style leaves unset.

A cell, part or mark with a style of its own (`style(..)`, or `set_style` in
place) draws as given over the base; one with no style takes `ink`, and a
`faint()` one takes `faint`, at each draw, so a theme change only needs new
`Styles`.

The marker's text is yours too: `ListView::cursor("> ")` replaces "▌ ". And
since the list has no words of its own, the header titles, the end line, the
empty line and the count's words all come from your app, in any language.
