---
title: Give the terminal back, always
description: How the shell restores the terminal on quit, error, panic and signals, how an app's own panic hook fits in, and the paced loop underneath.
sidebar:
  order: 100
---

## What the shell turns on, and off again

`Tui::run` enters the terminal through `Term`: raw mode, the alternate screen
and bracketed paste, plus mouse capture and focus events only when the app
asks with `Tui::modes(Modes::new().mouse(true))`; with mouse capture on, a
drag selects and copies text through the shell instead of the terminal
([Select and copy text](../selection/)). `Term` remembers each one it turned
on, and turns exactly those off again:

- on quit, and when `run` returns an error;
- on a panic on the UI thread, before the panic message prints;
- on Unix, on `SIGHUP` (the terminal going away: a closed tmux pane or
  window), `SIGTERM` and `SIGINT` while the loop runs. These end the loop as a
  quit does, without asking: every command the shell started gets stopped,
  the terminal is given back, and then the process ends by that same signal,
  as it would have without the shell. Before the loop and after it, those
  signals keep their default action.

A panic on a worker thread leaves the running app alone.

On the alternate screen (not on Windows), `Term` also pushes the keyboard
event-type flags and turns off the terminal's alternate scroll, and undoes
both before it leaves that screen. Where the terminal honours alternate
scroll, the wheel then sends no arrows at all. In kitty, with the mouse not
captured, each wheel step still arrives as an arrow press and its release a
few milliseconds later; the shell drops those pairs, so the wheel doesn't move
a list like arrow keys. Real arrows and held keys are unchanged.

## An app's own panic hook

A hook the app installs before `run` still runs, after the shell has given
the terminal back on a UI-thread panic. A hook that ends the process calls
`pito_tui::restore()` first, so a panic on a worker thread gives the terminal
back too:

```rust
std::panic::set_hook(Box::new(|info| {
    pito_tui::restore();
    eprintln!("{info}");
    std::process::exit(101);
}));
```

Any thread may call `restore()`: the first call gives the terminal back and
waits for a frame being drawn to finish, later calls do nothing, and the
shell draws nothing after it.

## The loop underneath

The loop draws at most one frame every 8.333 ms (120 fps), batches every wake
that came in since, and draws nothing while nothing changes. An input thread
and the app's workers wake one channel; otherwise the loop sleeps until the
next wake or deadline. Each frame is a synchronized update, so the terminal
never shows half of one.

An app that keeps its own loop uses the same parts: `Term::enter(Modes)`,
`listen(sender)` for the input thread, `Waker` for its workers, `Pace` and
`wait_until(now, dirty, &pace, deadlines)` for the pacing, and
`Tui::run_in(&mut term)` to run the shell inside a terminal it already
holds. It reads input through `listen`, never `crossterm::event::read()`
directly: under the keyboard flags kitty also reports key releases and
repeats, and `listen` drops the releases and the wheel's pairs and hands a
repeat on as a press.

Next: [Move an existing app onto the shell](../adopting/).
