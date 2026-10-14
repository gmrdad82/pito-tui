---
title: Keys and the quit guard
description: The fixed order every key goes through, app-wide keys, and the double ctrl+c quit guard that asks first while work runs.
sidebar:
  order: 30
---

![The quit guard: one ctrl+c arms it until it lapses, and with work running two ask first](../images/quit.gif)

## The quit guard

One `ctrl+c` arms the guard and shows the app's `Words::again` in the footer;
a second within 2 s quits. While any screen reports work in `busy()`, the
second asks first, in the app's words:

```rust
let words = Words::new()
    .again("ctrl+c again to quit")
    .busy(|running| format!("{running} still running. Quit anyway?"))
    .yes("Yes")
    .no("No")
    .choose("y/n choose · enter accept · esc cancel");
```

```text
$ demo --dump 80x12 --keys "ctrl+c"
 ──────────────────────────────────── demo ──────────────────── workspace one ─
                        1 Home  2 What to do next  3 Log
 ─ Home ───────────────────────────────────────────────────────────────────────

                                      demo
                                     v1.0.0


 ──────────────────────────────────────────────────────────────────────────────
 tab next screen · j start a job · c run a command · ctrl+c twice quit
 ctrl+c again to quit
```

The builder tunes it: `quit(Mode)` (ask while busy by default),
`quit_window(Duration)`, `quit_keys(&[..])`, `quit_sticky(true)` to keep the
armed guard through other keys until it lapses, `quit_tone(..)` and
`quit_hints(false)` to hide the hints while it is armed.

## The order every key goes through

So layers never fight over a key, each key goes through one fixed order:

1. a key release is ignored;
2. the quit guard;
3. a takeover, which hears only its cancel keys;
4. the modal on top;
5. an open confirm (unless the key is in `Tui::through_keys`, which reaches
   `on_key` and the screen with the confirm left open);
6. an open pick;
7. the app slot's `claim`;
8. a gate, which takes every other key;
9. a screen that is typing into an input;
10. the help toggle (`?`);
11. the header's focus keys, and its arrows while a header row has focus;
12. the keys that open an overlay (the activity screen or the app's own);
13. the nav keys, the group keys and the number keys;
14. the stop key while a screen loads;
15. the app's own keys (`on_key`);
16. the screen's `key`.

`Cx::key()` tells a callback which key led to it, so `back` knows `esc` from
`q`.

## App-wide keys

`Tui::on_key` sees a key before the screen does and returns `true` to take
it. The demo's `a` cycles the accent from anywhere:

```rust
let mut accent = 0;
Tui::new("demo", VERSION, ACCENTS[accent]).on_key(move |key, cx| {
    if key.code != KeyCode::Char('a') {
        return false;
    }
    accent = (accent + 1) % ACCENTS.len();
    cx.palette(Palette::new(ACCENTS[accent]));
    true
})
```

For keys that must win over a gate or a typing screen, use the app slot's
`claim` ([Screens, tabs and the header](../screens/#one-app-wide-screen)).

## Typing into an input

A screen that takes text returns `true` from `typing()` and its input bar
from `input()`; while it types, the nav and help keys reach it as text. A
bracketed paste arrives in `paste`.

```rust
fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, Msg>) {
    if let Edit::Submit(value) = self.input.key(key.into()) {
        self.input.clear();
        cx.say(format!("Saved {value}"), footer::Tone::Good);
    }
}

fn paste(&mut self, text: &str, _cx: &mut Cx<'_, Msg>) {
    self.input.paste(text);
}

fn typing(&self) -> bool {
    true
}

fn input(&self) -> Option<InputBar<'_>> {
    Some(InputBar::new("Name", &self.input).placeholder("type a name"))
}
```

`Input` and `InputBar` come from pito-footer, as `pito_tui::footer`; an
`Input::new().masked(true)` is a password field.

Next: [Colours and words](../palette-and-words/).
