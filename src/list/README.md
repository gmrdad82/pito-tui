# The list

<p align="center"><img src="docs/images/list/demo.gif" alt="The demo example: jobs in sections, sizes that grow in place, a selected range, and the view following the selection to the end with a scrollbar and a count of the rows on screen"></p>

`pito_tui::list` is the list behind the [PITO MD](https://pitomd.com)
terminal apps: selectable list rows for [ratatui](https://ratatui.rs) apps,
in the style of HEY's terminal UI. The selected row gets a bold "▌ " marker,
the columns pad to line up and drop by priority when the terminal narrows,
and the list scrolls, pages, selects ranges and answers clicks. It has no
words of its own: every word, style and key comes from your app, so any
language works. The shell draws every list it shows with it; a screen draws
its own lists with it too.

It is fast and stays fast as the list grows: **about 0.3 ms a frame** for a
20,000-row list at 150×40, the same for 100,000 items built on demand, and
**zero heap allocations** in a warm frame. Drawing the same 20,000 items, it
is **70× faster** than a ratatui `Table` built from every row.

## List at a glance

| | |
|---|---|
| Frame time, 20,000 owned rows, 150×40 | 289 µs mean |
| Frame time, 100,000 items built on demand | 292 µs mean (lazy, in groups), 354 µs (shared vector, scrollbar and count) |
| Heap allocations in a warm frame, owned rows | 0 (held by a counting-allocator test) |
| Against ratatui 0.30 `Table`, 20,000 items | 309 µs vs 21,692 µs, 0 vs 260,062 allocations a frame |
| Source | 2,165 lines of Rust in `src/list/` |

Measured once on 2026-10-14 on an AMD Ryzen 9 9950X under Linux, Rust 1.97.1,
release profile, with `cargo run --release --example list_bench` and
`cargo run --release --example list_versus`. Windows and macOS weren't timed.

## Sections, a selection and a range

<p align="center"><img src="docs/images/list/range.gif" alt="The cursor moves down into the Queued section, v starts a range, and every row from the anchor to the cursor draws selected while the section headings stay as they are"></p>

Rows can be grouped under headings (`Row::heading`) with blank lines between
them; the cursor, paging and clicks skip both. `select_range` draws every row
between two ends in the selected style for a Visual mode, with the marker on
the cursor row alone. Cells keep their own colours under the selection when
you ask (`Styles::keep_colours(true)`), and a cell can be several styled parts
(`Cell::parts`), like "v1.4" followed by a faint "→ v1.5".

## Columns that make room

<p align="center"><img src="docs/images/list/columns.gif" alt="The demo list as the terminal narrows from 96 to 38 columns and widens back: Size drops first, then Version, the messages clip with an ellipsis while Job and Owner stay, and both columns return in order as the width comes back"></p>

The terminal narrows from 96 to 38 columns and widens back. Each column has a
minimum, a preferred width and a drop priority: when the width runs out the
highest priority drops first (Size, then Version here), a `pinned()` column
never drops, and the flexible column (the message) takes what's left and cuts
after a whole word with "…", or keeps the start of a single long word, such as
an email, when no word fits (`pito_tui::list::cut` gives apps the same cut).
`fit(max)` sizes a column to its widest cell on screen, `right()` aligns
numbers, and `rest()` lets one cell run across the columns after it. Widths
are Unicode-aware, so diacritics, wide glyphs and "…" measure and clip
cleanly.

![The demo list at 96 columns: Job, Version, Owner, Size and Message all shown](docs/images/list/columns-96.png)

The same list at 46 and 38 columns: first Size drops, then Version, while Job
and Owner stay and the message clips with "…".

![The same list at 46 columns: the Size column dropped, the messages clipped with an ellipsis](docs/images/list/columns-46.png)
![The same list at 38 columns: Size and Version dropped, Job and Owner kept](docs/images/list/columns-38.png)

## Scrolling, paging, a scrollbar and a count

<p align="center"><img src="docs/images/list/scroll.gif" alt="Page Down moves a whole page and ctrl+d half a page, G jumps to the end where the end line shows, ctrl+u and Page Up move back and g returns to the top, the scrollbar's thumb and the count at its foot following"></p>

The view keeps the selection in view, paging moves by the rows on screen
(`Paging::View` keeps the cursor on its screen line instead), and an end line
scrolls in after the last row. `ListView::scrollbar(Some(Bar::LINE))` draws a
slim bar with half-cell steps, and `ListView::count(..)` writes "1–13 of 25" in
your words and number format, both only when the list overflows. Keys come in
as the list's own `Key` (arrows, ctrl+d and ctrl+u for half a page, Page Down
and Page Up for a whole one, home, end and enter, or `Keys::VIM` for j/k and
g/G) and go out as a `Step`; `hit`
names the row under a mouse click.

## Rows rewritten in place

<p align="center"><img src="docs/images/list/live.gif" alt="Two running jobs: their sizes grow and turn from green to yellow to red, and their progress messages change, every 120 ms, while the rest of the list stays still"></p>

`row_mut`, `Cell::set_text`, `Cell::set_style` and `Mark::set_text` rewrite a
row's texts and colours in the strings already there, so a frame that updates
counters every 120 ms and draws allocates nothing once the strings have grown
to their texts.

## Big lists, built on demand

<p align="center"><img src="docs/images/list/bench-bars.png" alt="Bar chart of the list's mean frame time at 150x40: 289 µs plain, 302 µs with a range, theming and four rows rewritten, 305 µs with columns that fit their cells, 292 µs for a lazy 100,000-row list in groups and 354 µs for a shared 100,000-item vector with a scrollbar and a count"></p>

A list can own its rows, or read them from a `Source`: `Shared::new(items,
builder)` wraps your `Arc<Vec<T>>` and builds a row only when a draw asks for
it, so 100,000 jobs, pings or log lines cost what the 40 rows on screen cost.
Hand it a new vector every frame for the price of an `Arc` clone.

## Speed against ratatui's Table

<p align="center"><img src="docs/images/list/frame-time-paired.png" alt="Paired bar chart, lower is better: for the whole list of 20,000 items the list takes 309 µs a frame and a ratatui Table built from every item 21,692 µs, 70 times faster; for visible rows only the list built on demand takes 285 µs and a Table of the rows the app slices itself 167 µs, 1.7 times slower"></p>

[`examples/list_versus.rs`](examples/list_versus.rs) draws the same 20,000
items (five columns, a header, the selection moving every frame) at 150×40,
with ratatui 0.30's own layout cache on:

| | Mean frame | Allocations a frame | Bytes a frame |
|---|---:|---:|---:|
| The list, owned rows | 309 µs | 0 | 0 |
| The list, shared vector built on demand | 285 µs | 292 | 19,663 |
| ratatui `Table`, every row built each frame | 21,692 µs | 260,062 | 51,844,944 |
| ratatui `Table`, the app slices the visible rows | 167 µs | 569 | 106,032 |

A `Table` handed every row, the way ratatui's own examples build one, is 70×
slower and allocates 52 MB a frame. A `Table` handed only the rows on screen is
1.7× faster than the list, but then the app does the scrolling itself, and it
still allocates where the list's owned rows don't; the list also does the
column dropping, sections, ranges, the end line, the scrollbar and the count
in that time.

## Keys from crossterm

`Key::from(crossterm::event::KeyEvent)` (crossterm 0.29, as
`pito_tui::crossterm`) turns a key event into the list's `Key`; drawing needs
no backend, and an app on another backend builds the `Key` values itself. The
conversion turns Alt, Super, Meta or Hyper on a non-character key into
`Key::Other`, keeps Alt apart on a character (`Key::Alt('y')`), and passes
Ctrl+Alt plus a character on as that character, which is how AltGr arrives on
some platforms, so diacritics can still be typed. Ctrl and Shift on a named
key (Tab, Enter, Esc, Backspace, the arrows, Home, End, Page Up, Page Down,
Delete) arrive as `Key::Mod(mods, named)`, such as
`Key::Mod(Mods::CTRL, Named::PageDown)`, so an app can bind them; the plain
key stays `Key::PageDown`. Shift+Tab is `Key::BackTab`. The built-in key maps
hold only plain keys, so a modified key moves nothing in the list and passes to
your app unless you map it.

## A first list

Three rows, three columns, the cursor moved down once:

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
#       .map(|y| {
#           let line: String = (0..area.width).map(|x| buffer[(x, y)].symbol()).collect();
#           line.trim_end().to_owned()
#       })
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

draws, at 44 and then 24 cells wide (the Owner column drops when it doesn't
fit):

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

In an app, keep the `List` in your state, draw it with
`frame.render_widget(ListView::new(&mut list, &columns), area)`, and pass it
your key events with `list.key(Key::from(event))`: it answers
`Step::Open(index)` for enter.

## Try the list's demo

From a checkout of pito-tui:

```sh
cargo run --example list_demo
```

The demo is the clip at the top of this page:
sample jobs in three sections under a styled header, sizes that grow and warm
in place, the view following the selection with a scrollbar and the rows on
screen counted at its foot, `v` for a range, ctrl+d and ctrl+u for half a
page, Page Down and Page Up for a whole one, `g` and `G` for the ends, enter to
open and ctrl+c twice to quit, as in every PITO MD terminal app (the quit
guard comes from [the footer](crate::footer)). Every clip and still on this
page is recorded from it with [prec](https://github.com/gmrdad82/prec), the
tapes kept in `render/list/`.

## Everything else about the list

[Every feature and the API](https://docs.pitomd.com/pito-tui/list/reference/api/)
has every feature in detail, the whole API and a full example with lazy and
shared sources, sections, a scrollbar and a count. The
[list's guides](https://docs.pitomd.com/pito-tui/list/) take one task a page,
from a first list in an app to 100,000 items built on demand.
