---
title: Screens, tabs and the header
description: Plug screens in as tabs, group them, drill in with a breadcrumb, and build the header as a stack of rows.
sidebar:
  order: 10
---

![The header as a row stack: groups with an underlined key, a caption, tabs numbered in their group, focus moving through the rows and an after-tabs selector](../images/stack.gif)

Each screen is a type that implements `Screen<E>`, where `E` is the app's own
message type (the answers its workers send back; `()` when it has none). A
screen draws its area, takes its keys, and says what it shows in the header
and the footer. Every method but `draw` has a default.

## Add screens as tabs

```rust
Tui::new("demo", VERSION, accent)
    .screen(Section::new("Home"), home)
    .screen(Section::new("What to do next").short("Next"), Next::new())
    .screen(Section::new("Log"), Journal::new())
    .run()
```

The shell draws them as numbered tabs. With the default keys (`NavKeys::HEY`)
`tab` and `shift+tab` move between groups (between tabs when there is one
group, as here), `]` and `[` between the tabs of a group, a digit goes to that
tab, and `esc` or `q` go back. `Tui::nav_keys(..)` changes them, and
`NavKeys::NONE` turns them off. Tabs are numbered from 1, the tenth on `0`;
`NavKeys::HEY.zero_first(true)` numbers them from `0` instead, and tabs past
the tenth then show no digit and are reached with `]`, `[` and the group keys.
`.short(..)` is the name used when the full one doesn't fit. A screen is
started the first time it is shown, and hears `entered` each time the nav
brings it up again.

## What a screen tells the header and footer

```rust
impl Screen<Msg> for Journal {
    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        self.log.draw(frame, area, palette);
    }

    fn facts(&self) -> Vec<(String, Style)> {
        let line = format!("line {} of {}", self.log.top() + 1, self.log.len());
        vec![(line, Style::new())]
    }

    fn hints(&self) -> Vec<Hint<'_>> {
        vec![
            Hint::new("↑↓", "scroll"),
            Hint::new("g/G", "ends").rank(2),
            Hint::new("/", "find").rank(1),
        ]
    }
}
```

- `facts` is the row under the tabs ("line 11998 of 20000").
- `hints` are the footer's key hints. The footer fits them to the width: a
  hint with a higher `rank` is dropped first, and a `pinned()` hint (the
  app's `ctrl+c twice quit`, say) always stays.
- `fact_spans()` and `hint_list()` are the borrowed forms of `facts` and
  `hints`, as `badge()` is: the screen keeps the `Span` parts and the
  `Hint<'static>` list in its own state, changes them only when they change,
  and returns a slice. When either slice holds anything it is drawn in place
  of the owned hook's answer; an empty one falls back to it. With these and
  the defaults a steady frame allocates nothing.
- `status` or `status_parts(room)` sit on the right of the title rule, and
  `lead(room)` on its left beside the help word; each is told the room it
  has, so it can drop a part rather than see it cut.
- `badge()` draws a short pill of `Span` parts (a licence tag, say) at the
  right end of the same rule, after the status and one space. It borrows parts
  the screen keeps in its own state and updates only when they change, so a
  steady badge allocates nothing per frame; a badge wider than the room is left
  out rather than cut.
- `notice` keeps a line in the footer in any tone for as long as the screen
  returns it; `Cx::say(text, tone)` shows one until the next key.

## Drill in with a breadcrumb

A screen that opens something returns its name from `crumb`; the sections
row becomes the breadcrumb, and the nav's back key calls `back`:

```rust
fn crumb(&self) -> Option<String> {
    self.opened().map(|step| step.title.to_string())
}

fn back(&mut self, _cx: &mut Cx<'_, Msg>) {
    self.open = None;
}
```

The facts row goes while a screen is drilled in, unless the app keeps it with
`Tui::keep_facts(true)`.

## Group tabs

`Tui::group(..)` before the screens that belong to it starts a group; screens
before any group join one unnamed group, and a single group draws no groups
row. `Tui::group_keys(&[..])` gives the group added last its own keys, which
return to that group's last-used tab.

## The header as a stack of rows

`Tui::head(Head::new([..]))` draws the header from rows in the app's order.
This is the demo's `--stack` header in the clip above:

```rust
let head = Head::new([
    Row::Title,
    Row::Groups,
    Row::Caption,
    Row::Sections,
    Row::Facts,
    Row::Crumbs,
])
.numbers(Numbers::InGroup)
.lone(true);

let key = Style::new().add_modifier(Modifier::UNDERLINED);
tui.head(head)
    .focus_keys(&[Key::Tab], &[Key::BackTab])
    .group(Label::spans([("W", key), ("ork", Style::new())]))
    .group_keys(&[Key::Char('w')])
    .screen(Section::new("Home"), home)
    .screen(Section::new("What to do next").short("Next"), Next::new())
    .group(Label::spans([("T", key), ("ools", Style::new())]))
    .group_keys(&[Key::Char('t')])
    .screen(Section::new("Log"), Journal::new())
```

- `Numbers::Across` numbers tabs across the app, `InGroup` from 1 in each
  group, `Off` not at all; `lone(true)` leaves a group of one tab
  unnumbered. Number keys follow what is drawn.
- `Row::Caption` is a ruled label between two rows: the current group's name,
  or the screen's `caption()`.
- `focus_keys(next, previous)` moves keyboard focus through the groups row,
  the sections row, the after-tabs slot and the content. Left and right move
  along the focused row, up and down between rows, `esc` or `enter` give
  focus back. `Screen::after_tabs()` puts the screen's own spans after the
  tabs, a "‹ Blue ›" selector here, and keys reach the screen there with
  `Cx::focus()` saying `Focus::Slot`.
- `Tui::show(..)` and `Cx::show(..)` show or hide groups and tabs by the
  app's predicate (`Item::Group(n)`, `Item::Tab(screen)`).

Without `head`, the header is pito-header's default layout: title, groups,
sections, facts and crumbs.

## One app-wide screen

`Tui::app(screen)` adds a screen that is never drawn as content. Its `lead`,
`left`, `status_parts` and `badge` feed the title rule on every screen, and it
hears app-wide keys first in `claim`:

```rust
impl Screen<Msg> for App {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn claim(&mut self, key: KeyEvent, cx: &mut Cx<'_, Msg>) -> bool {
        if key.code != KeyCode::Char('k') || !key.modifiers.contains(KeyModifiers::CONTROL) {
            return false;
        }
        cx.open(go_to());
        true
    }
}
```

Next: [Load data without blocking](../loading/).
