---
title: Guard ctrl+c against an accidental quit
description: Quit on a second ctrl+c, ask first while work is running, and let the event loop wake when the armed notice ends.
sidebar:
  order: 15
---

![An import runs; d asks "Delete the 12 photos?" over the hint line, the choice moves between Yes and No and No keeps them; ctrl+c shows "Press ctrl+c again to quit", a second ctrl+c asks "The import is still running. Quit anyway?", and y quits](../../images/footer/confirm.gif)

`QuitGuard` sees every key first and says what to do with it. It never reads
input or the clock itself: the app passes the key, the time and whether work
is running.

```rust
use std::time::Instant;
use pito_tui::footer::{Guard, Mode, QuitGuard, Wording};

let mut guard = QuitGuard::new(
    Wording::new("Press ctrl+c again to quit")
        .ask("The import is still running. Quit anyway?"),
)
.mode(Mode::Ask);

match guard.key(key, Instant::now(), importing) {
    Guard::Quit => return Ok(()),
    Guard::Held => {}
    _ => app.key(key),
}
```

| `Guard` | Means |
|---|---|
| `Pass` | the key is the app's |
| `Held` | the guard used it: it armed, or it is asking |
| `Quit` | quit now |

## Modes

- `Mode::Twice`, the default: the first ctrl+c arms and shows the app's
  "again" notice, a second inside the window quits, and any other key
  disarms.
- `Mode::Ask`: the same, but when the app says work is running the second
  press opens a confirm with the `ask` question first. A further ctrl+c while
  it asks answers No.
- `Mode::Once`: the first ctrl+c quits.

The window is 2 seconds unless the app sets `window(Duration)`, and the keys
are ctrl+c unless it sets `triggers(&[..])`. A screen that passes every key
through, such as an embedded terminal, simply doesn't hand its keys to the
guard.

## Draw it

The guard gives the footer its notice while armed and its confirm while
asking:

```rust
use pito_tui::footer::{Footer, Notice, Words};

let words = Words::new("Yes", "No").hint("y/n choose · enter accept · esc cancel");
let footer = Footer::new(&HINTS)
    .notice(guard.notice().map(Notice::accent))
    .confirm(guard.bar(words));
```

## Clear the notice on time

`deadline()` is when the armed notice ends. Wait for input until then, and call
`tick(now)`, which returns true when it disarmed, so the app redraws:

```rust
use std::time::{Duration, Instant};
use pito_tui::crossterm::event;

let wait = guard
    .deadline()
    .map_or(Duration::from_secs(60), |at| at.saturating_duration_since(Instant::now()));
if !event::poll(wait)? {
    guard.tick(Instant::now());
    continue;
}
```

`armed()` and `asking()` tell the app the guard's state, and `cancel()` drops
both.
