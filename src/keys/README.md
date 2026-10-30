One keymap per app. An app declares its keys once; from that one declaration
come the action a key does, the footer's hints and the `?` help sheet, so the
sheet, the hints and what a key really does never disagree. The model draws
nothing and holds no words of its own beyond the sheet's defaults in `Words`,
which the app replaces with its own.

## The model

```rust
use std::sync::LazyLock;

use pito_tui::keys::{Chord, Key, Keymap, Mode, Now, When, bind};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Act {
    Keep,
    Drop,
    Choose,
    Play,
    Stop,
    Undo,
}

const RAIL: Mode = Mode("clips");
const CUT: Mode = Mode("cut");
const PLAYING: When = When("while the cut plays");

static KEYS: LazyLock<Keymap<Act>> = LazyLock::new(|| {
    Keymap::new()
        .modes(&[RAIL, CUT])
        .group(
            "Rail",
            [
                bind("right", "keeps", Act::Keep).on(RAIL),
                bind("left", "drops", Act::Drop).on(RAIL),
                bind("up down", "choose", Act::Choose)
                    .on(RAIL)
                    .shown("up and down")
                    .rank(1),
            ],
        )
        .group(
            "Player",
            [
                bind("space esc", "stops it", Act::Stop)
                    .modes(&[RAIL, CUT])
                    .when(PLAYING)
                    .shown("space or esc"),
                bind("space", "plays the cut", Act::Play).modes(&[RAIL, CUT]),
            ],
        )
        .group(
            "Edit",
            [bind("ctrl+z", "undoes", Act::Undo).note("as often as you like")],
        )
});

let playing = Now::new(RAIL).when(&[PLAYING]);
assert_eq!(KEYS.action(playing, Chord::plain(Key::Space)), Some(&Act::Stop));
assert_eq!(KEYS.action(Now::new(CUT), Chord::plain(Key::Space)), Some(&Act::Play));
assert!(KEYS.clashes().is_empty());
```

- **Group**: a section of the sheet (Rail, Cut, Player, Export…), in the
  order declared.
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
  footer's priority, 0 first, as `footer::Hint::rank`. **.quiet()**: on the
  sheet only, never in the footer.
- **Now { mode, when }**: where the app is: `Now::new(CUT).when(&[PLAYING])`.

## What it answers

| call | gives |
| --- | --- |
| `keys.action(now, chord)` | the action that key does here, the first live bind in declared order |
| `keys.hints(now)` | the live hints for a footer, by rank: `Hint { keys, does, rank }`, `"? help"` among them |
| `keys.ticker(now)` | a `Line` per mode, the current one first (all its live hints), then the others' plain hints; `line.to_string()` is `"right keeps · left drops"` |
| `sheet.view(&keys, now)` | the sheet: `here` (the current mode's binds, grouped, `live` false for a context that doesn't hold) then `elsewhere` (the rest, grouped, with their modes), filtered by the search |
| `keys.clashes()` | binds that answer the same chord in the same place, where the first silently wins; assert it empty in the app's tests |
| `keys.button()` | the help button's words, `"? Help"` |

Every string is borrowed from the keymap; keep the keymap in a `static`
(`LazyLock`) and they are `&'static str`, which is what the shell wants.
`footer::Hint::from(hint)` turns a keymap hint into the footer's.

The keymap does its work once, when it is declared: each bind's written keys
and search text, and each mode's hints in rank order. So `action` is one pass
with no allocation, `ticker` and `hints` copy the ready lists, and the sheet's
search tests each bind's text without lowercasing it again. The search keeps
its first 64 characters and its first 8 words, so a pasted page can't stall
the frame.

## The conventions

`Sheet` holds them, so every app behaves the same:

- `?` opens and closes the sheet; esc closes it (first clearing a search);
  `/` searches it (typing goes to the search, enter keeps it, esc drops it);
  pgdn and pgup scroll it a page at a time.
- It never steals focus: `sheet.key(chord)` returns `Outcome::Took` only for
  those keys and `Outcome::Pass` for every other, which the app handles as if
  the sheet were closed. Opening it changes no selection, mode or playback,
  and the sheet shows the mode the user is in.
- A visible Help or `?` button calls `sheet.toggle()` (`cx.toggle_sheet()`
  from a screen in the shell).
- The sheet lists its own keys in a last group, `Help`; `Words` changes the
  wording to the app's voice, the sheet's title, its "Elsewhere" heading and
  its "nothing matches" line among them.
- The app calls `sheet.key` only when it isn't typing into its own text
  field, so a `?` typed there stays text, and closes the sheet when a key it
  passed on opens something of the app's own, so nothing acts hidden under it.

## In the shell

The shell does all of this for an app that hands it the keymap:
`Tui::keymap(&KEYS)` and the current place, `Tui::keys_now(now)` at the start
and `cx.keys_now(now)` from a screen whenever the mode or a context changes.
The footer then shows `keys.hints(now)` in the keymap's order, ranks and
words, beside the app's own `Tui::hints`; `?` opens the sheet, drawn as a
centred overlay over the screen, eased open and closed (at once with
`Tui::still(true)`), scrolling a page at a time when it outgrows the area,
with no allocation on a steady frame. While it is open, keys go to the sheet
first and those it passes on reach the screen as usual; `ctrl+c` always
quits. A screen still asks the keymap what a key does:

```rust,ignore
fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, Msg>) {
    let Some(chord) = Chord::from_crossterm(&key) else {
        return;
    };
    if let Some(act) = KEYS.action(self.now(), chord).copied() {
        self.act(act);
        cx.keys_now(self.now());
    }
}
```

An app that draws on its own keeps a `Panel`: `panel.key(chord, now)` before
its own keys and `panel.draw(buf, area, &palette, &KEYS, now, moment)` after
its screen, with `panel.animating(moment)` asking for the next frame while it
eases.

## Input

`Chord { key, mods }` is the keymap's own. A `Char` carries shift in its case
(`shift+a` is `Char('A')`), so `?` is `?` however the platform reports it, and
`Char(' ')` is `Space`. `Chord::from_crossterm(&KeyEvent)` converts
crossterm's events (a release gives `None`); anything else maps in a few lines
with `Chord::new(Key::…, Mods::…)`.
