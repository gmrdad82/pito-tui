---
title: Load data without blocking
description: Hand the shell a screen's read, show the hourglass while it runs, drop answers that come back late, and wake on a timer.
sidebar:
  order: 20
---

The UI thread never waits on a read. A screen hands the shell its work as a
closure; the shell runs it on a worker and delivers the answer only if it is
still the newest one.

## A read with the hourglass

```rust
enum Msg {
    Steps(Vec<Step>),
}

impl Screen<Msg> for Next {
    fn phase(&self) -> Phase<'_> {
        match (&self.steps, self.stopped) {
            (Some(_), _) => Phase::Ready,
            (None, false) => Phase::Loading("Finding what to do next...".into()),
            (None, true) => Phase::Trouble(
                vec![
                    Line::from("Stopped waiting for the steps."),
                    Line::from("r reads them again."),
                ]
                .into(),
            ),
        }
    }

    fn load(&mut self) -> Option<Job<Msg>> {
        self.steps = None;
        self.stopped = false;
        Some(Box::new(|| Msg::Steps(read_steps())))
    }

    fn loaded(&mut self, event: Msg, _cx: &mut Cx<'_, Msg>) {
        let Msg::Steps(steps) = event;
        self.steps = Some(steps);
    }

    fn cancel(&mut self) {
        self.stopped = true;
    }
}
```

- `load` runs the first time the screen is shown (`Tui::eager(true)` reads
  every screen at start), and again on `Cx::reload()`.
- While `phase` says `Loading(label)`, the shell draws the hourglass with the
  screen's words, and the hint under it from `Words::waiting`.
- A stop key (`esc` unless `Tui::stop_keys` picks others) stops waiting and
  calls `cancel`; `Phase::Trouble(lines)` then shows the screen's own lines.
- Each read carries a generation: an answer that comes back after the user
  stopped waiting, or after a newer read, is dropped and never reaches
  `loaded`.

This is the demo's "What to do next" tab half a second into its read:

```text
$ demo --dump 72x14 --keys "tab" --loading --at 500
 ──────────────────────────────── demo ──────────────── workspace one ─
                    1 Home  2 What to do next  3 Log
 ─ What to do next ────────────────────────────────────────────────────


                       Finding what to do next...
                           esc stops waiting




 ──────────────────────────────────────────────────────────────────────
 r read again · o jobs · ctrl+k go to · ? help · ctrl+c twice quit
```

## Loads that never flash

`Tui::hourglass_timing(delay, least)`: a load shorter than `delay` shows
nothing, and an hourglass once shown stays at least `least`, so quick reads
stay quiet and slow ones never blink. The demo uses 150 ms and 500 ms.

## Read again when the data changes

A screen lists what it reads in `sources()` (`&["steps"]`, say).
`Cx::changed("steps")` marks every screen that reads that source stale, and
they read again. `Cx::epoch()` marks the app changing mode, another account
or workspace: work started before it is dropped and every started screen
reads again.

## One-off work and long-lived watchers

- `Cx::detach(move || Msg::Saved(..))` runs any closure on a worker; its
  answer comes back to the same screen's `event`.
- A watcher thread takes `cx.waker()` (or `tui.waker(index)` before `run`)
  and posts with `waker.send(msg)`; each message wakes the loop and reaches
  that screen's `event`.

## Work on a timer, without a key

A screen's `deadline()` wakes the loop; when the loop's moment reaches it,
the shell calls `tick`, whichever screen is showing. The demo keeps "last
started 2s ago" fresh this way:

```rust
fn moment(&mut self, now: Instant) {
    self.now = Some(now);
}

fn deadline(&self) -> Option<Instant> {
    self.last
        .zip(self.now)
        .map(|(last, now)| clock::next_age(last, now))
}

fn facts(&self) -> Vec<(String, Style)> {
    let (Some(last), Some(now)) = (self.last, self.now) else {
        return Vec::new();
    };
    let ago = clock::age(now.saturating_duration_since(last), &Units::new("s", "m", "h", "d"));
    vec![(format!("last started {ago} ago"), Style::new())]
}
```

Drawing reads the loop's moment (`moment`, `Cx::now()`), never the wall
clock, so a headless run that moves the clock with `Tui::advance` gives the
same frames every time. `clock::next_age` and `next_span` give the moment the
text may next change, so the loop wakes exactly then and sleeps otherwise.

Next: [Keys and the quit guard](../keys/).
