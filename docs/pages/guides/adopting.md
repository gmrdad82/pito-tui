---
title: Move an existing app onto the shell
description: For an app that draws with pito-header, pito-footer, pito-hourglass and pito-list and keeps its own loop, the steps to move onto pito-tui and prove nothing moved.
sidebar:
  order: 110
---

This guide is for an app that already draws with pito-header, pito-footer,
pito-hourglass and pito-list directly, and keeps its own loop. Moving to the
shell deletes that glue; the app's screens and data stay as they are.

## What the app keeps

- Its screens' content: what each one draws, its data, its rows and
  columns, its detail views. pito-list and pito-footer's `Input` stay the
  app's to use inside a screen, through `pito_tui::list` and
  `pito_tui::footer`.
- Its words, every one, in any language: `Words` for the shell's few
  places (the help word, the quit notice and question and its hint line,
  Yes and No, the confirm's hint line, the too-small message, the hint
  under the hourglass), `Activities` for the activity band and screen, the
  global hints, and each screen's hints, facts, notices, labels and
  messages. An empty word is simply not drawn.
- Its keys beyond the shell's: anything a screen's `key` takes, and
  app-wide actions through `on_key`. The shell's own keys are the app's to
  change too: `nav_keys` (pito-header's `NavKeys`, `NavKeys::NONE`
  included), `quit_keys`, `stop_keys`, `help(None)`, and the filter's
  `start_keys` and `clear_keys`. Mouse capture stays off unless the app
  turns it on with `Modes`.
- Its workers and their messages: the app's own enum is the `E` in
  `Tui<E>`, and its threads post through a `Waker<E>`.
- Its command line: the app parses its own flags and calls the shell's
  headless helpers.
- Its crash reporting: a panic hook the app installs before `run` still
  runs, after the shell has given the terminal back on a UI-thread panic. A
  hook that ends the process calls `pito_tui::restore()` before it exits,
  so a panic on a worker thread gives the terminal back too:

  ```rust
  std::panic::set_hook(Box::new(|info| {
      pito_tui::restore();
      eprintln!("{info}");
      std::process::exit(101);
  }));
  ```

## What the shell takes over

| The app's own code today | With pito-tui |
|---|---|
| `ratatui::init` and `ratatui::restore`, bracketed-paste and mouse toggles, panic hooks that restore the terminal | `Tui::run` (or `Term::enter` for a loop of its own) |
| An input thread feeding a channel, an enum with an input arm, `recv_timeout` | `listen` and `Wake<E>`, inside `run` |
| A frame pacer, a dirty flag, deadlines, draining a batch of wakes | `run`: `Pace` at 120 fps, synchronized updates, `deadline` and `animating` per screen |
| Polls on a timer, slow reads timed out, a clock threaded into drawing | `Screen::deadline` and `tick`, `Screen::moment`, `Tui::advance` headless |
| Code that hands the app's asks to threads and routes their answers back | `Screen::load` (tracked by generation) and `Cx::detach` (one-off work) |
| Key routing: release, quit guard, confirm, input, help, nav | `Tui::key`, in that order |
| Quit guard wiring: busy count, wording, tick, deadline, notice, bar | `Screen::busy`, `Words::again` and `Words::busy`, `quit`, `quit_window` |
| A confirm gathered from several places | `Cx::confirm` and `Screen::answer` |
| Header assembly: title, help word, a lead beside it, a status fitted to the room, facts, a breadcrumb kept in step | `Screen::lead`, `status_parts`, `badge`, `facts`, `crumb`, `selected` and `back`, `keep_facts` |
| Footer assembly: hints, notice, legend, confirm, input, version, height | `Screen::hints`, `input`, `notice`, `legend`, `Cx::say`, `Words::leave`; the version slot is wired |
| A panel of running work above the footer and a screen listing it, with a detail view | `Activities`, fed by `Cx::activity`, `activities` and `forget`; or the app's own `Band` and `Tui::overlay` screen |
| A screen of its own opened over any other by a key, with unlit tabs and its own breadcrumb | `Tui::overlay(section, keys, screen)` |
| Code that runs the app's own CLI, parses its `--progress json` lines and stops it | `Cx::run(id, label, Command)` (`Cx::run_from` when the lines come on stderr), `Cx::stop(id)`, `Screen::heard` for each line and the exit, and `command::read` for a single line |
| A picker drawn over a screen, its keys and its answer | `Cx::pick(Pick)` and `Screen::picked` |
| A long-log or trace pane: scroll, page, ends, find, copy, a cache of built rows | `Log`; a long list takes pito-list's `Shared` |
| Age and duration text, Braille bars, percent, spinners, label redraw timing | `clock`, `progress`, `Activity::estimate` |
| Frames compared cell by cell before and after a change, by hand | `capture::capture` and `capture::compare` behind `--capture` and `--compare`, `capture::Recorder` for the frames from before the move or from an app that keeps its own loop, and `capture::compare_dirs` for two captures |
| A tone module mapped to four crates' `Styles` | `Palette`, and `Cx::palette` at runtime |
| A loading wrapper around the hourglass | `Phase::Loading(label)` |
| The too-small guard, a centred message, OSC 52, text helpers | `min_size` and `Words::too_small`, `message`, `Cx::copy`, `text` |
| Dump size and key parsing, text and ANSI output, a bench loop | `dump::*`, `Tui::shot`, `Tui::bench` |

## Moving over, step by step

1. **One pin.** Replace the four `pito-*` lines in `Cargo.toml` with the
   pito-tui line from [Install](../../install/), and import through `pito_tui::header`,
   `pito_tui::footer`, `pito_tui::hourglass` and `pito_tui::list`. Keep
   ratatui and crossterm through `pito_tui::ratatui` and
   `pito_tui::crossterm`, or pin the same versions (0.30 and 0.29).
2. **The palette.** Build `Palette::new(accent)`, then set the tokens the
   app's tone module had (`good`, `bad`, `muted`, `base` for a painted
   background). Hand it to `Tui::palette`.
3. **The words.** Move every shell-facing string into one `Words` and the
   app-wide hints (`? help`, `ctrl+c twice quit`) into `Tui::hints`.
4. **One screen at a time.** Make each section or page a type that
   implements `Screen<E>`: its draw function becomes `draw`, its hints and
   facts become `hints` and `facts`, its loader becomes `load` (return the
   work as a closure) and `loaded` (take the answer from the app's enum, no
   downcast), its "stop waiting" becomes `cancel`, its detail view becomes
   `crumb` plus `back`, and its search box becomes a `Filter` or its own
   `Input` returned from `input` while `typing` is true. A password field is
   an `Input::new().masked(true)`; a bracketed paste arrives in `paste`.
5. **Groups.** `Tui::group(Group::new("Work"))` before the screens that
   belong to it; screens before any group join one unnamed group, and a
   single group draws no groups row.
6. **Workers.** One-off work goes through `Cx::detach(move || E::Done(..))`
   and comes back to the same screen's `event`. A long-lived watcher takes
   `cx.waker()` (or `tui.waker(index)` before `run`) and posts with
   `send`. A watcher that sees a source change sends an event whose screen
   calls `cx.changed("source")`. A poll on a timer is a screen's `deadline`
   with the poll in its `tick`, which gives the next deadline.
7. **Activities.** If the app shows what it has running (operations,
   exports, simulations), give `Tui::activities` an `Activities` with the
   screen's name (`Section::new("Operations")`), the key that opens it, its
   hints and its words (the empty message, the overflow line, the elapsed
   time, the facts). Whichever screen hears about the work calls
   `cx.activity(Activity::new(id, label, started)...)` each time it changes,
   with the same `id`; `cx.activities(..)` replaces the whole list, which
   suits an app that reads its work as a list, and `cx.forget(id)` drops
   one. The app's own panel and its screen go; their data stays where it
   was. The activity screen always comes after the app's screens, wherever
   `Tui::activities` sits in the builder chain, so the screens keep the
   indices `go`, `waker` and `event` use. The demo's fake jobs (`Home` in
   `examples/demo.rs`) are the pattern.
8. **The loop goes.** Replace the app's `run` with `Tui::run()`; delete
   the pacer, the terminal setup and restore, the restoring panic hooks,
   the input thread and the key routing. The app's crash hook stays where
   it is, installed before `run`; if it ends the process, it calls
   `pito_tui::restore()` before it exits.
9. **Headless flags.** Wire `--dump WxH` to `dump::size` and `Tui::shot`,
   `--keys` to `dump::keys`, `--ansi` to `dump::ansi`, `--loading` to
   `shot(.., settle: false)`, `--at MS` to `Tui::advance` and a second
   `frame`, `--bench N` to `Tui::bench`, and `--capture DIR` and
   `--compare DIR` to `capture::capture` and `capture::compare` with a
   `capture::Walk` of the app's deeper states (exit 1 when a frame
   differs); `main` and `walk` in `examples/demo.rs` do exactly this. A dump
   or a walk with fixtures builds the screens from fixture data, so the
   same build gives the same frames. A game that keeps its dev tools out of
   the shipped build puts these behind its own feature.
10. **Check.** Before the move, record every screen and deeper state at
    the sizes the app cares about with the old build, through
    `capture::Recorder` ([Prove the switch](#prove-the-switch)); after
    it, compare. The header, footer and spacing should match, since the shell
    draws them with the same crates and the same layout (a one-cell side
    margin for the header and footer, two cells for the content, one row
    above the footer). Then delete the captures: they are a safety net for
    the move, not a test to keep.
11. **What the app had built in.** Its own pickers become `Cx::pick`, its
    log and trace panes a `Log` (a long list a pito-list `Shared`), its
    age and duration text `clock`, its bars and spinners `progress`, and the
    code that ran its own CLI and read the progress lines `Cx::run` (or
    `Cx::run_from` for lines on stderr), with `Cx::stop` for its stop and
    `Screen::heard` for what it read from the lines and the exit; the app
    keeps its own words on the band by posting its activity for that id.

## Prove the switch

`capture::capture` walks an app that already runs on `Tui`, so it can't
record the old build. `capture::Recorder` writes the same frames from the
buffers the app's own headless dump draws: record them with the old build,
switch to `Tui::run`, and `capture::compare` walks the new build against
them, scenario names and sizes matched.

- `Recorder::new(dir)` wants a directory that is absent or empty, as
  `capture` does.
- `screen(index, name, &buffer)` saves a screen the way a walk names it:
  `index` is its place among the new build's screens, in the order the app
  adds them with `Tui::screen` (the activity screen comes after all of
  them), and `name` its section's name.
- `record(name, &buffer)` saves a deeper state under the name the new
  build's `capture::Script` carries, so `record("Next drilled in", ..)`
  meets `Script::new("Next drilled in", keys)`.
- A frame is the whole terminal at one size, as a `TestBackend` holds it
  after a draw; its size is the buffer's own. Record each state at every
  size the walk lists. The same state at the same size twice is an error,
  so two states never share a name by accident.
- Both builds draw from the same fixture data at the same moment, as any
  capture does.
- `Recorder::against(dir)` takes the same calls but compares each frame
  with the one of the same name in `dir`, an existing capture, instead of
  writing it; `finish()` then lists the differences, as `compare` does. So
  one function that draws every state into a `&mut Recorder` gives an app
  both `--capture DIR` (`Recorder::new`) and `--compare DIR`
  (`Recorder::against`), as below.
- `capture::compare_dirs(before, after)` compares two captures already on
  disk, frame by frame and cell by cell: each frame in both, each one only
  `after` holds (`Added`) and each one only `before` holds (`Removed`).
  Either may come from `capture::capture` or from a `Recorder`.

```rust
use std::path::Path;

use pito_tui::capture::Recorder;
use pito_tui::ratatui::{Frame, Terminal, backend::TestBackend, buffer::Buffer};

struct OldApp;

impl OldApp {
    fn open(_state: &str) -> Self {
        OldApp
    }

    fn draw(&self, _frame: &mut Frame) {}
}

fn shot(state: &str, (width, height): (u16, u16)) -> Buffer {
    let app = OldApp::open(state);
    let Ok(mut terminal) = Terminal::new(TestBackend::new(width, height));
    let Ok(_) = terminal.draw(|frame| app.draw(frame));
    terminal.backend().buffer().clone()
}

fn states(recorder: &mut Recorder) -> std::io::Result<()> {
    for size in [(80, 24), (120, 34)] {
        recorder.screen(0, "Home", &shot("home", size))?;
        recorder.screen(1, "Next", &shot("next", size))?;
        recorder.record("Next drilled in", &shot("next/detail", size))?;
    }
    Ok(())
}

fn main() -> std::io::Result<()> {
    let args: Vec<String> = std::env::args().collect();
    match args.get(1..3) {
        Some([flag, dir]) if flag == "--capture" => {
            let mut recorder = Recorder::new(Path::new(dir))?;
            states(&mut recorder)?;
            println!("{} frames", recorder.frames());
        }
        Some([flag, dir]) if flag == "--compare" => {
            let mut recorder = Recorder::against(Path::new(dir))?;
            states(&mut recorder)?;
            let differences = recorder.finish()?;
            for difference in &differences {
                println!("{difference}");
            }
            if !differences.is_empty() {
                std::process::exit(1);
            }
        }
        _ => {}
    }
    Ok(())
}
```

After the switch, `capture::compare` with
`Walk::new().sizes([(80, 24), (120, 34)]).script(Script::new("Next drilled
in", keys))` and the new build's `Fn() -> Tui<E>` lists each cell that
differs, each frame only the new build draws (`Added`) and each one it no
longer reaches (`Removed`). An empty list proves the switch; then delete the
directory.

An app that keeps its own loop (its own `Term`, `listen`, `Waker`, `Pace`
and `wait_until`) proves a change the same way without moving: `--capture
tmp/before` with the old build, then `--compare tmp/before` with the new
one, or `--capture tmp/after` with the new one and
`capture::compare_dirs(Path::new("tmp/before"), Path::new("tmp/after"))`.

## An app that already has a section trait

Most of it maps one to one: a section's name becomes the `header::Section`
passed to `Tui::screen`; its data sources, phase, cancel and retry become
`sources`, `phase`, `cancel` and `Cx::reload`; a loader that returns a job
and a delivery to downcast becomes `load` returning a closure and `loaded`
taking the app's own enum, with the generation kept by the shell; its notice
becomes `Cx::say`; its pending confirm becomes `Cx::confirm` with `answer`;
its asks become `Cx::detach`. A job that needs a connection or a client opens
or clones its own inside the closure, since it runs on a worker.

An overlay that lists the app's running work becomes `Activities`. Any other
overlay that sits over every section becomes a screen of its own (in its own
group if it should read as one), or stays the app's drawing inside a screen
with `crumb` naming where the user is.
