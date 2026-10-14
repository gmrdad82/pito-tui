---
title: Dumps, benches and frame captures
description: Draw the app at any size without a terminal, time every screen, and record every cell before a big change to compare after it.
sidebar:
  order: 90
---

Every part of the shell runs without a terminal. Time comes from the run,
never the wall clock, and data from the app's own fixtures, so the same app
gives the same frames. The app wires these to its own flags; `main` in
`examples/demo.rs` does exactly this, and the commands on this page run it.

## Draw a frame after some keys

```rust
let keys = dump::keys("tab / foo enter").map_err(|word| format!("--keys can't read {word}"))?;
let mut tui = app();
let buffer = tui.shot(120, 34, &keys, true);
println!("{}", dump::text(&buffer));
```

- `dump::size("120x34")` reads a size, `dump::keys(..)` a key script
  ([the syntax](../../reference/formats/#key-scripts)).
- `Tui::shot(width, height, &keys, settle)` draws after the keys; with
  `settle` it waits for every read to land first, without it the frame shows
  the hourglass.
- `Tui::advance(duration)` moves the run's clock (the hourglass as if it had
  run that long, deadlines ticking), then `Tui::frame(width, height)` draws
  again.
- `dump::text` prints the cells as plain text; `dump::ansi` with colours, for
  a terminal.

```text
$ cargo run --release --example demo -- --dump 72x14 --keys "tab"
 ──────────────────────────────── demo ──────────────── workspace one ─
                    1 Home  2 What to do next  3 Log
                                5 steps
 ─ What to do next ────────────────────────────────────────────────────
    Step                        How
  ▌ Replace this screen         Home and Next are two Screen types; s…
    Set your accent             Tui::new takes one colour, or Palette…
    Add your first command      a key in Screen::key, its work in Cx:…
    Load real data              Screen::load returns the read; Screen…
    Wire the headless flags     --dump, --keys, --at, --bench, --capt…

 ──────────────────────────────────────────────────────────────────────
 ↑↓ move · enter open · / filter · ctrl+k go to · ctrl+c twice quit
```

## Time every screen

`Tui::bench(width, height, frames)` draws each screen that many times into an
in-memory terminal, the whole shell each time, moving the clock a frame at a
time, and returns a `Bench` per screen with its average and worst frame:

```rust
for bench in app().bench(120, 34, 1000) {
    println!(
        "{}: average {:.3} ms, worst {:.3} ms",
        bench.screen,
        bench.average.as_secs_f64() * 1000.0,
        bench.worst.as_secs_f64() * 1000.0,
    );
}
```

```text
$ cargo run --release --example demo -- --bench 200 --dump 120x34
demo --bench: 200 frames of the Home screen at 120x34: average 0.048 ms, worst 0.093 ms
demo --bench: 200 frames of the What to do next screen at 120x34: average 0.074 ms, worst 0.231 ms
demo --bench: 200 frames of the Log screen at 120x34: average 0.086 ms, worst 0.168 ms
demo --bench: 200 frames of the Jobs screen at 120x34: average 0.052 ms, worst 0.093 ms
```

One run on an AMD Ryzen 9 9950X on Linux; numbers vary with the machine and
its load. A frame at 120 fps has 8.333 ms.

## Frames before and after a big change

`capture::capture` walks every screen on its own, plus the app's key scripts
for deeper states, at a list of sizes, and saves every cell (symbol, colours
and modifiers) to a directory. `capture::compare` walks the same again and
reports each cell that moved.

```rust
fn walk() -> Walk {
    let script = |name: &str, keys: &str| Script::new(name, dump::keys(keys).unwrap_or_default());
    Walk::new()
        .script(script("drill-in", "tab enter"))
        .script(script("quit guard", "ctrl+c"))
        .script(script("jobs", "j j").at(Duration::from_millis(2500)))
        .script(script("hourglass", "tab").loading().at(Duration::from_millis(500)))
}

let frames = capture::capture(Path::new("tmp/before"), &walk(), app)?;
let differences = capture::compare(Path::new("tmp/before"), &walk(), app)?;
for difference in &differences {
    println!("{difference}");
}
```

`Walk::new()` captures at 40x12, 80x24, 120x34 and 200x60 unless `.sizes(..)`
says otherwise, into a directory that is absent or empty. `.loading()` keeps
the frame from settling, and `.at(..)`
moves the clock before the frame is taken.

```text
$ demo --capture tmp/before
demo: 84 frames captured in tmp/before
$ demo --compare tmp/before
demo: every frame matches tmp/before
```

After a change, each difference prints as one line: the scenario, the size,
the cell and what it held before and after
([the format](../../reference/formats/#differences)). It is a safety net for a
big change: capture, change, compare, then delete the captures. It is not a
test to keep, and it pins no wording.

An app still on its own loop records the same frames from its own buffers
with `capture::Recorder`, and `capture::compare_dirs(before, after)` compares
any two captures on disk ([Move an existing app onto the
shell](../adopting/#prove-the-switch)).

Next: [Give the terminal back, always](../terminal/).
