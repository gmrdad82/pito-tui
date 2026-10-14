---
title: Give the terminal back, always
description: How the shell restores the terminal on quit, error, panic and signals, how an app's own panic hook fits in, and the paced loop underneath.
sidebar:
  order: 100
---

## What the shell turns on, and off again

`Tui::run` enters the terminal through `Term`: raw mode, the alternate screen
and bracketed paste, plus mouse capture and focus events only when the app
asks with `Tui::modes(Modes::new().mouse(true))`. `Term` remembers each one it
turned on, and turns exactly those off again:

- on quit, and when `run` returns an error;
- on a panic on the UI thread, before the panic message prints;
- on Unix, on `SIGHUP` (the terminal going away: a closed tmux pane or
  window), `SIGTERM` and `SIGINT` while the loop runs. These end the loop as a
  quit does, without asking: every command the shell started gets stopped,
  the terminal is given back, and then the process ends by that same signal,
  as it would have without the shell. Before the loop and after it, those
  signals keep their default action.

A panic on a worker thread leaves the running app alone.

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
holds.

Next: [Move an existing app onto the shell](../adopting/).
