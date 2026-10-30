---
title: Handle keys and clicks
description: Pick the list's keys, read the Step each key answers with, and select the row under a mouse click.
sidebar:
  order: 2
---

The list never reads input itself. Your app reads its events, hands the list
the keys it should handle as the crate's own `Key`, and asks it which row a
click landed on.

## Keys in, steps out

`List::key(key)` moves the selection and answers with a `Step`:

| Step | When |
|---|---|
| `Moved` | the selection moved |
| `Held` | a list key that moved nothing, such as up on the first row |
| `Open(index)` | the open key, with the selected row's index |
| `Pass` | a key that isn't the list's, left for your app |

The keys are a `Keys` set. `Keys::new()`, the default, is the arrows, ctrl+u
and ctrl+d for half a page, page up and page down for a whole one, home, end
and enter. `Keys::VIM` adds `j` and `k`, and `g` and `G` for the ends. The
parts are `up`, `down`, `half_up`, `half_down`, `page_up`, `page_down`,
`first`, `last` and `open`, and any of them can be replaced:

```rust
use pito_tui::list::{Column, Key, Keys, List, ListView, Row, Step};
use ratatui::{buffer::Buffer, layout::Rect, widgets::Widget};

const OPEN: &[Key] = &[Key::Enter, Key::Char('o')];

let mut list = List::new()
    .keys(Keys::VIM.open(OPEN))
    .with_rows((1..=40).map(|n| Row::new([format!("item {n}")])));
let columns = [Column::new("Item", 8, 0)];
let area = Rect::new(0, 0, 20, 10);
ListView::new(&mut list, &columns).render(area, &mut Buffer::empty(area));

assert_eq!(list.key(Key::Char('k')), Step::Held);
assert_eq!(list.key(Key::Char('j')), Step::Moved);
assert_eq!(list.key(Key::Ctrl('d')), Step::Moved);
assert_eq!(list.selected(), Some(6));
assert_eq!(list.key(Key::PageDown), Step::Moved);
assert_eq!(list.selected(), Some(15));
assert_eq!(list.key(Key::Char('G')), Step::Moved);
assert_eq!(list.key(Key::Char('o')), Step::Open(39));
assert_eq!(list.key(Key::Char('x')), Step::Pass);
```

Paging moves by the rows on screen less one, and half a page is half of that
rounded up, so the list needs one draw before the page keys know how far to
go; here the 10-line area pages by 9 and half-pages by
5 rows. `up`, `down`, `half_up`, `half_down`, `page_up`, `page_down`, `first`
and `last` do the same as their keys, for an app that maps its own.

With the `crossterm` feature, `Key::from(event)` converts a crossterm
`KeyEvent`. Alt, Super, Meta or Hyper on a key that isn't a character gives
`Key::Other`; Alt with a character stays apart as `Key::Alt(c)`; Ctrl+Alt
with a character passes on as that character, which is how AltGr arrives on
some platforms, so diacritics can still be typed. A key release is
`Key::Other`.

Ctrl and Shift on a named key (Tab, Enter, Esc, Backspace, the arrows, Home,
End, Page Up, Page Down, Delete) arrive as `Key::Mod(mods, named)`, with
`Mods::CTRL`, `Mods::SHIFT` or `Mods::CTRL_SHIFT`; the same key with neither
stays its plain variant, such as `Key::PageDown`. Shift+Tab is `Key::BackTab`,
and Ctrl+Shift+Tab is `Key::Mod(Mods::CTRL, Named::BackTab)`. The built-in
sets hold only plain keys, so a modified key passes to your app unless you map
it:

```rust
use pito_tui::list::{Column, Key, Keys, List, ListView, Mods, Named, Row, Step};
use ratatui::{buffer::Buffer, layout::Rect, widgets::Widget};

const NEXT_PAGE: &[Key] = &[Key::Mod(Mods::CTRL, Named::PageDown)];

let mut list = List::new().with_rows((1..=40).map(|n| Row::new([format!("item {n}")])));
let columns = [Column::new("Item", 8, 0)];
let area = Rect::new(0, 0, 20, 10);
ListView::new(&mut list, &columns).render(area, &mut Buffer::empty(area));

assert_eq!(list.key(Key::Mod(Mods::CTRL, Named::PageDown)), Step::Pass);
assert_eq!(list.selected(), Some(0));

let mut list = list.keys(Keys::new().page_down(NEXT_PAGE));
assert_eq!(list.key(Key::PageDown), Step::Pass);
assert_eq!(list.key(Key::Mod(Mods::CTRL, Named::PageDown)), Step::Moved);
assert_eq!(list.selected(), Some(9));
```

## Paging that keeps the view still

By default paging moves the selection and the view follows it. With
`List::new().paging(Paging::View)` the view moves by the same rows as the
selection, so the selected row stays on its line of the screen until the
list's start or end stops the view. `scroll_to(top)` moves the view itself;
the next draw still keeps the selection in view.

## Clicks

`List::hit(area, header, column, row)` names the row under a terminal cell,
following the scroll: give it the area the list was drawn in, whether the
header was drawn, and the mouse event's column and row. It answers `None`
for the header, a section heading, a blank line, the end line, a cell below
the rows, the scrollbar's column and the count's line.

```rust
use pito_tui::list::{Column, List, ListView, Row};
use ratatui::{buffer::Buffer, layout::Rect, widgets::Widget};

let mut list = List::new().with_rows((1..=40).map(|n| Row::new([format!("item {n}")])));
let columns = [Column::new("Item", 8, 0)];
let area = Rect::new(0, 2, 20, 10);
list.select(25);
ListView::new(&mut list, &columns).header(true).render(area, &mut Buffer::empty(Rect::new(0, 0, 20, 12)));

assert_eq!(list.top(), 17);
assert_eq!(list.hit(area, true, 5, 2), None);
assert_eq!(list.hit(area, true, 5, 3), Some(17));
if let Some(index) = list.hit(area, true, 5, 6) {
    list.select(index);
}
assert_eq!(list.selected(), Some(20));
```

The list was drawn from line 2, its header on that line, scrolled so that
row 25 is on screen: a click on line 3 is row 17, the first one shown, and a
click on line 6 selects row 20. `hit` answers for the last draw, so call it
with the area that draw used.
