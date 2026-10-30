---
title: Update rows in place
description: Rewrite a row's texts and colours between frames in the strings already there, so a live list allocates nothing.
sidebar:
  order: 8
---

![Two running jobs: their sizes grow and turn from green to yellow to red, and their progress messages change, every 120 ms, while the rest of the list stays still](../images/live.gif)

A list that shows live numbers, such as sizes, rates or progress, changes a
few cells many times a second. Building new rows each time allocates; writing
into the rows the list already holds doesn't, once each string has grown to
its longest text.

## Rewrite texts and styles

`List::row_mut(index)` hands out one of the list's own rows.
`Row::cells_mut()` and `Row::leading_mut()` reach its cells and its mark, and
the setters write into what is there:

| Setter | Changes |
|---|---|
| `Cell::set_text(&str)` | the cell's text; a cell of parts becomes one part |
| `Cell::set_part(index, &str) -> bool` | one part's text, such as a heading's count; `false` when there is no such part |
| `Cell::set_style(Style)` | the cell's style |
| `Cell::set_part_style(index, Style) -> bool` | one part's style |
| `Mark::set_text(&str)`, `Mark::set_style(Style)` | the mark's text and style |

```rust
use std::fmt::Write;

use pito_tui::list::{Cell, Column, List, ListView, Mark, Row};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Style},
    widgets::Widget,
};

let mut list = List::new().with_rows([
    Row::new([Cell::new("upload"), Cell::new("0 MB")]).mark(Mark::new("⧗")),
    Row::new([Cell::new("backup"), Cell::new("done")]).mark(Mark::new("✓")),
]);
let columns = [Column::new("Job", 6, 10), Column::new("Size", 6, 0).right()];
let area = Rect::new(0, 0, 22, 2);
let mut buffer = Buffer::empty(area);
let mut size = String::with_capacity(16);

for mb in [3, 9, 14] {
    size.clear();
    let _ = write!(size, "{mb} MB");
    let warm = if mb > 10 { Color::Red } else { Color::Yellow };
    if let Some(cell) = list.row_mut(0).and_then(|row| row.cells_mut().get_mut(1)) {
        cell.set_text(&size);
        cell.set_style(Style::new().fg(warm));
    }
    ListView::new(&mut list, &columns).render(area, &mut buffer);
}
# let lines: Vec<String> = (0..area.height)
#     .map(|y| (0..area.width).map(|x| buffer[(x, y)].symbol()).collect::<String>().trim_end().to_owned())
#     .collect();
# assert_eq!(lines, ["▌ ⧗ upload       14 MB", "  ✓ backup        done"]);
assert_eq!(list.rows()[0].cells()[1], Cell::new("14 MB").style(Style::new().fg(Color::Red)));
```

After the third frame:

```text
▌ ⧗ upload       14 MB
  ✓ backup        done
```

A style set this way replaces the ink or faint paint the cell had. The
selected row and a range still draw it in the selected style, or with the
selected style patched over it under `keep_colours(true)`.

`row_mut` is for the list's own rows (`List::new()`). For a list over your own
data, change the data and let the rows be built again: see
[Show 100,000 items](big-lists.md).

## Nothing allocated in a warm frame

A counting-allocator test in the crate rewrites cells, parts and marks, draws
a range under a styled header and reads the drawn columns every frame, and
holds that a warm frame allocates nothing. The mark column follows a
rewritten mark at the next draw; a widest mark that narrowed costs one scan
of the rows then.

To change every row at once, `List::update(|rows| ..)` hands over the whole
row vector and keeps the selection on its index, clamped.
