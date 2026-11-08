# Headless: dumps, benches and captures

Never launch the TUI on a desktop to check it. Every part of the shell runs without
a terminal; the app wires these to its own flags (`main` in `examples/demo.rs` does).

## Draw a frame after some keys

```rust
let keys = dump::keys("tab / foo enter").map_err(|word| format!("--keys can't read {word}"))?;
let mut tui = app();
let buffer = tui.shot(120, 34, &keys, true);
println!("{}", dump::text(&buffer));
```

- `dump::size("120x34")` reads a size; `dump::keys(..)` reads a key script.
- `Tui::shot(width, height, &keys, settle)` draws after the keys; with `settle` every
  read lands first, without it the frame shows the hourglass.
- `Tui::advance(duration)` moves the run's clock, then `Tui::frame(width, height)` draws again.
- `dump::text` prints plain text; `dump::ansi` adds colours.

From the command line, an app wired like the demo takes:

```
cargo run --release --example demo -- --dump 72x14 --keys "tab"
```

Flags: `--dump WxH`, `--keys "…"`, `--ansi`, `--loading` (`shot(.., settle: false)`),
`--at MS` (`Tui::advance` then a second `frame`), `--bench N`, `--capture DIR`, `--compare DIR`.

## Key-script syntax

`dump::keys("tab / foo enter ctrl+c")`, keys separated by spaces:

- Names, in any case: `enter`, `esc`, `up`, `down`, `left`, `right`, `pgup`, `pgdn`,
  `home`, `end`, `backspace`, `delete`, `space`, `tab`, `backtab`.
- `ctrl+` or `alt+` in lowercase followed by exactly one character (`ctrl+k`); more
  than one is an error returned as that word.
- Any other word types its characters one by one: `foo` is `f`, `o`, `o`.
- Sizes: `WIDTHxHEIGHT`, an `X` works too, plain digits above zero.

## Bench

`Tui::bench(width, height, frames)` draws each screen that many times in memory and
returns a `Bench` per screen (`screen`, `average`, `worst`). A frame at 120 fps has 8.333 ms.

## Capture and compare

`capture::capture(dir, &walk, app)` walks every screen plus the app's key scripts at
40x12, 80x24, 120x34 and 200x60 (override with `.sizes(..)`) and saves every cell into
a directory that is absent or empty. `capture::compare(dir, &walk, app)` walks again and
returns each moved cell. `Walk::new().script(Script::new(name, keys))`; `.loading()`
keeps a frame from settling, `.at(duration)` moves the clock first.

```
demo --capture tmp/before
demo --compare tmp/before
```

Capture, change, compare, then delete the captures: a safety net, not a test to keep.
For an app on its own loop, `capture::Recorder` writes frames from the app's own buffers
and `capture::compare_dirs(before, after)` compares two captures on disk.
Time comes from the run and data from fixtures, so the same build gives the same frames.
