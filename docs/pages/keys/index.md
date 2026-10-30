---
title: Keys
description: One keymap per app, declared once, gives the action a key does, the footer's hints and the ? help sheet.
sidebar:
  order: 0
---


`pito_tui::keys` is one keymap per app. The app declares its keys once, in
groups, with the modes and contexts each answers in; from that one
declaration come the action a key does, the footer's hints for wherever the
app is, and the `?` help sheet with every key, so the sheet, the hints and
what a key really does never disagree. The keymap has no words of the
shell's: every key and phrase is the app's, and the sheet's own few words
(its title, "Elsewhere", "nothing matches" and its Help group) are defaults
the app replaces through `keys::Words`.

- **One declaration** for the action, the footer hints and the `?` sheet.
- **Zero allocations** on a steady frame with the sheet open, searched or
  shut, held by a counting-allocator test.
- **About 1 µs** for a mode's hints and 4 µs for the open sheet's view of a
  200-bind keymap in three modes.

## A first keymap in the shell

Two modes, a list and an editor: the footer shows `down / j next`,
`? help` and `e edits` while listing and `enter saves` while editing, and
`?` opens the sheet with every key.

```rust,no_run
use std::sync::LazyLock;

use pito_tui::crossterm::event::KeyEvent;
use pito_tui::keys::{Chord, Keymap, Mode, Now, bind};
use pito_tui::ratatui::{Frame, layout::Rect, style::Color};
use pito_tui::{Cx, Palette, Screen, Tui};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Act {
    Next,
    Edit,
    Save,
}

const LIST: Mode = Mode("list");
const EDIT: Mode = Mode("edit");

static KEYS: LazyLock<Keymap<Act>> = LazyLock::new(|| {
    Keymap::new()
        .modes(&[LIST, EDIT])
        .group(
            "List",
            [
                bind("down j", "next", Act::Next).on(LIST),
                bind("e", "edits", Act::Edit).on(LIST).rank(1),
            ],
        )
        .group("Editor", [bind("enter", "saves", Act::Save).on(EDIT)])
});

#[derive(Default)]
struct Home {
    editing: bool,
    row: usize,
}

impl Home {
    fn now(&self) -> Now<'static> {
        Now::new(if self.editing { EDIT } else { LIST })
    }
}

impl Screen<()> for Home {
    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        let text = if self.editing { "editing" } else { "listing" };
        frame.buffer_mut().set_string(area.x, area.y, text, palette.ink);
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, ()>) {
        let Some(chord) = Chord::from_crossterm(&key) else {
            return;
        };
        match KEYS.action(self.now(), chord) {
            Some(Act::Next) => self.row += 1,
            Some(Act::Edit) => self.editing = true,
            Some(Act::Save) => self.editing = false,
            None => return,
        }
        cx.keys_now(self.now());
    }
}

fn main() -> std::io::Result<()> {
    Tui::new("hello", "0.1.0", Color::Cyan)
        .screen("Home", Home::default())
        .keymap(&KEYS)
        .keys_now(Now::new(LIST))
        .run()
}
```

`Tui::keymap` takes a `&'static` keymap, which a `LazyLock` gives; every hint
then borrows its words from it. `Tui::keys_now` sets where the app starts,
and a screen calls `cx.keys_now(now)` whenever its mode or a context changes;
the next frame's hints and the open sheet follow. Hints the app passes to
`Tui::hints` still show, after the keymap's.

## The model

- **Group**: a section of the sheet (List, Editor, Player, Export…), in the
  order declared. `Keymap::modes(&[..])` orders the modes; a mode a bind
  names joins the order on its own.
- **bind(keys, does, action)**: `keys` is one or more chords, space-separated
  (`"ctrl+z"`, `"up down"`, `"shift+space"`, `"f2"`, `"?"`). A misspelled key
  panics at declaration with `pito-tui: 'ctlr+z' is not a key`, so any test
  that builds the keymap catches it. `does` is the one phrase both the sheet
  and the footer show.
- **.on(mode) / .modes(&[..])**: the modes or screens it answers in; none
  means everywhere. **.when(context)**: only while that context holds, such
  as `When("while the cut plays")`; a contextual bind declared before a
  general one on the same key overrides it while the context holds.
- **.shown(text)** writes the keys differently (`"up and down"`); otherwise
  the chords are written `ctrl+z`, `shift+a`, `space`, `pgdn`, `f2`,
  `cmd+…` on macOS and `super+…` elsewhere, joined by `" / "` (`.joiner(..)`).
- **.note(text)**: longer words shown only on the sheet. **.rank(n)**: the
  footer's priority, 0 first, as `footer::Hint::rank`; the footer drops the
  highest ranks first when the width runs out. **.quiet()**: on the sheet
  only, never in the footer.
- **Now { mode, when }**: where the app is: `Now::new(CUT).when(&[PLAYING])`.

## What it answers

| call | gives |
| --- | --- |
| `keys.action(now, chord)` | the action that key does here, the first live bind in declared order |
| `keys.hints(now)` | the live hints for a footer, by rank: `Hint { keys, does, rank }`, `"? help"` among them; `footer::Hint::from(hint)` makes the footer's |
| `keys.ticker(now)` | a `Line` per mode, the current one first (all its live hints), then the others' plain hints; `line.to_string()` is `"right keeps · left drops"` |
| `sheet.view(&keys, now)` | the sheet: `here` (the current mode's binds, grouped, `live` false for a context that doesn't hold) then `elsewhere` (the rest, grouped, with their modes), filtered by the search |
| `keys.clashes()` | binds that answer the same chord in the same place, where the first silently wins; assert it empty in the app's tests |
| `keys.button()` | the help button's words, `"? Help"` |

The keymap does its work once, when it is declared: each bind's written keys
and search text, and each mode's hints in rank order. So `action` is one pass
with no allocation, the footer's hints are copied from a ready list, and the
sheet's search tests each bind's text without lowercasing it again. The
search keeps its first 64 characters and its first 8 words, so a pasted page
can't stall the frame.

## The conventions

`keys::Sheet` holds them, so every app behaves the same:

- `?` opens and closes the sheet; esc closes it (first clearing a search);
  `/` searches it (typing goes to the search, enter keeps it, esc drops it);
  pgdn and pgup scroll it a page at a time.
- It never steals focus: `sheet.key(chord)` returns `Outcome::Took` only for
  those keys and `Outcome::Pass` for every other, which reaches the screen as
  if the sheet were closed. Opening it changes no selection, mode or
  playback, and the sheet shows the mode the user is in.
- The sheet lists its own keys in a last group, `Help`; `keys::Words`
  changes the wording to the app's voice.
- A screen typing into its own input gets `?` as text: the shell asks the
  sheet only when no screen is typing.
- `ctrl+c` quits from anywhere, the open sheet and its search included.

## The `?` sheet

The shell draws the sheet as a centred overlay over the screen, in the
palette's rule, accent and ink: the current mode's name, then each group of
the keys that answer here, each row its keys then what they do, its note,
and its context in parentheses, muted while the context doesn't hold; then,
under the keymap's "Elsewhere", the keys of the other modes with the modes
they answer in. While a search is typed its query shows on the first row.
When the keys outgrow the area the sheet keeps its height and scrolls a page
at a time, with an arrow on its border for each way there is more. It fits
80x24 to 200x60, eases open and closed over 180 ms (`Tui::still(true)` shows
and hides it at once), and draws from the loop's moment, so a headless run
gives the same frames. An app with its own loop draws the same sheet with
`keys::Panel`: `panel.key(chord, now)` before its own keys,
`panel.draw(buf, area, &palette, &KEYS, now, moment)` after its screen, and
another frame while `panel.animating(moment)`.

## Input

`keys::Chord { key, mods }` is the keymap's own. A `Char` carries shift in
its case (`shift+a` is `Char('A')`), so `?` is `?` however the platform
reports it, and `Char(' ')` is `Space`. `Chord::from_crossterm(&KeyEvent)`
converts crossterm's events (a release gives `None`, shift+tab is
`shift+tab`); a desk's own key type or winit maps in a few lines with
`Chord::new(Key::…, Mods::…)`.

## Try the keys demo

```sh
cargo run --example keys_demo
cargo run --example keys_demo -- --dump 90x24 right '?'
```

A rail of clips and a cut in two modes, a player with a context, a note, a
quiet bind and the `?` sheet with search; `--dump WxH keys…` prints one
frame as text after those keys. Its tape for prec is kept in
`render/keys/`.

## Where to go

- [Install](../install/): the one pito-tui line; the keymap comes with it.
- [Keys and the quit guard](../guides/keys/): the order every key goes
  through, the sheet's place in it, and app-wide keys.
- [The footer](../footer/): how the hints fit the width and drop by rank.
- [API reference](../api/pito_tui/keys/index.html): the `cargo doc` pages.
- [Changelog](../changelog/): every release, with a link to the keymap's
  earlier releases.

## Part of pito-tui

The keymap sits beside pito-tui's four widgets, [the header](../header/),
[the footer](../footer/), [the hourglass](../hourglass/) and
[the list](../list/); [the shell](../) shows its hints in the footer and its
sheet over the screen.
