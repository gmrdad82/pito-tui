# The hourglass

<p align="center"><img src="docs/images/hourglass/demo.gif" width="640" alt="The example: the hourglass drains and flips while &quot;Loading the data...&quot; shimmers under it, above the hint &quot;esc stops waiting&quot;"></p>

`pito_tui::hourglass` is an hourglass for ratatui apps to show while data
loads: HEY's 13-frame Braille hourglass, the app's copy (the label)
shimmering under it in the app's accent, and an optional hint under that. One
widget, one call a frame. The shell shows it while a screen loads; an app with
its own loop draws it on its own.

- **4.8 µs a draw** with the glass, the label and the hint, under 0.03% of a
  60 fps frame; 1.0 µs on a single row ([how it's measured](#fast-and-allocation-free)).
- **Zero allocations** when it draws, held by a counting-allocator test in
  the gate.
- **Small:** one 284-line source file and a 96-byte `Copy` widget.

## A first hourglass

Draw it from the time the wait started:

```rust,standalone_crate
use std::time::Instant;

use pito_tui::hourglass::Hourglass;
use pito_tui::ratatui::{
    Frame,
    style::{Color, Modifier, Style},
};

fn draw(frame: &mut Frame, started: Instant) {
    let hourglass = Hourglass::new(started.elapsed())
        .label("Loading the data...")
        .hint(Some("esc stops waiting"))
        .glass(Style::new().fg(Color::Yellow))
        .accent(Style::new().fg(Color::Magenta))
        .muted(Style::new().fg(Color::DarkGray).add_modifier(Modifier::DIM));
    frame.render_widget(hourglass, frame.area());
}
```

Redraw about every 50 ms while it shows; the picture depends only on the
elapsed time. That code is
[examples/hourglass_demo.rs](examples/hourglass_demo.rs), the clip at the top
of this page. From a checkout of pito-tui, Esc or q quits:

```sh
cargo run --example hourglass_demo
```

## Your copy, your accent, any script

<p align="center"><img src="docs/images/hourglass/gallery.gif" width="640" alt="Four hourglasses at once, each a quarter turn apart: &quot;Loading the data...&quot; in magenta, &quot;Se încarcă datele…&quot; in blue, a Japanese label in green with no hint, and &quot;Syncing 3 inboxes&quot; in yellow"></p>

Every word and every style is the app's: the label, the hint, the glass, the
accent the label shimmers in and the muted colour of the rest. A
three-character band sweeps across the label and pauses: its centre character
is the accent plus bold, its two neighbours the accent, everything else
muted. The hint never shimmers. Widths come from Unicode, so Romanian
diacritics, "…" and two-cell CJK glyphs line up and centre exactly
([examples/hourglass_gallery.rs](examples/hourglass_gallery.rs)).

## Fits whatever space it gets

<p align="center"><img src="docs/images/hourglass/fit.gif" width="640" alt="One hourglass in a box that shrinks step by step: 27×9 and 13×8 show the glass, label and hint; at 13×7 the glass goes; 21×2 shows the label and hint; 21×1 the label alone; 11×1 and 6×1 the label cut at a character, still shimmering"></p>

- **Room for everything** (at least 11 cells wide and 7 rows high, 8 with a
  hint): the glass, the label and the hint are centred as one block.
- **Less room:** the glass goes and the shimmering label stays, centred, with
  the hint on the next row when there is a second row.
- **Long copy is cut at a character,** never mid-glyph (a combining mark
  rides with its letter), and the band counts only the characters drawn, so a
  cut label still shimmers end to end.
- **No room at all** (a zero width or height): nothing is drawn.

The clip is [examples/hourglass_fit.rs](examples/hourglass_fit.rs).

## A picture of the time alone

<p align="center"><img src="docs/images/hourglass/frames.png" width="800" alt="The 13 frames: seven drain frames starting at 0, 429, 858, 1286, 1715, 2143 and 2572 ms, then six flip frames at 3000, 3200, 3400, 3600, 3800 and 4000 ms"></p>

The frame and the band are pure functions of the elapsed time, so a redraw
at any moment lands on the same picture, and nothing keeps state between
frames. The still is [examples/hourglass_frames.rs](examples/hourglass_frames.rs),
drawn with `frame_at` itself.

```text
frame_at(elapsed)   e = elapsed mod CYCLE (4200 ms)
                    e <  3000: frame = floor(e * 7 / 3000)            the 7 drain frames, ~430 ms each
                    e >= 3000: frame = 7 + floor((e - 3000) * 6 / 1200)  the 6 flip frames, 200 ms each

band_at(elapsed, n) t = elapsed mod 1500 ms
                    t <  1200: centre = floor(t * (n + 4) / 1200) - 2
                    t >= 1200: None, the pause
```

`band_at` returns the centre's index into the `n` characters of the label,
and `None` whenever the centre is off the label (below 0, or at `n` or
beyond) and through the pause. The sweep starts two characters before the
label and ends two after it, so the band slides in and out.

## Fast and allocation-free

<p align="center"><img src="docs/images/hourglass/draw-time.png" width="640" alt="Hourglass draw time in nanoseconds per draw, lower is better: 4807 at 200×50 and 4622 at 80×24 with the glass, label and hint; 2690 at 40×2 with the label and hint; 981 at 12×1 with the label cut"></p>

[examples/hourglass_bench.rs](examples/hourglass_bench.rs) draws the widget
into a ratatui `Buffer` a million times per size, 1 ms of elapsed time apart
(so every frame and band position comes round), after a warm-up, and prints
the mean time per draw as [pito-compare](https://github.com/gmrdad82/pito-compare)
data. Measured once, on 2026-10-14, on an AMD Ryzen 9 9950X with Rust
1.97.1, a release build on one thread; the chart's machine line and note were
added to the printed data by hand
([docs/images/hourglass/draw-time.toml](docs/images/hourglass/draw-time.toml)):

```sh
cargo run --release --example hourglass_bench > draw-time.toml
pcmp draw-time.toml --out .
```

| Area | What's drawn | Per draw |
|---|---|---|
| 200×50 | glass, label, hint | 4.8 µs |
| 80×24 | glass, label, hint | 4.6 µs |
| 40×2 | label, hint | 2.7 µs |
| 12×1 | label, cut | 1.0 µs |

The time doesn't grow with the area: only the glass's 66 cells and the
label's and hint's characters are written. Drawing allocates nothing:
`tests/hourglass_allocation.rs` counts every allocation through a global
allocator while 200 frames are drawn at six sizes, and holds the count at
zero.

## The hourglass's API

```text
pub const WIDTH: u16;                    // 11, every frame's width
pub const GLASS_ROWS: u16;               // 6, every frame's height
pub const FRAMES: [[&str; 6]; 13];       // HEY's frames, verbatim
pub const CYCLE: Duration;               // one full turn, drain plus flip: 4.2 s
pub fn frame_at(elapsed: Duration) -> usize;
pub fn band_at(elapsed: Duration, n: usize) -> Option<usize>;

pub struct Hourglass<'a>;                // Widget, by value and by reference
impl<'a> Hourglass<'a> {
    pub fn new(elapsed: Duration) -> Self;
    pub fn label(self, label: &'a str) -> Self;        // the copy, "Loading..." until set
    pub fn hint(self, hint: Option<&'a str>) -> Self;  // e.g. Some("esc stops waiting")
    pub fn glass(self, style: Style) -> Self;          // the glass, per state if the app likes
    pub fn accent(self, style: Style) -> Self;         // the shimmer, the TUI's accent
    pub fn muted(self, style: Style) -> Self;          // the label's base and the hint
}
```

The styles default to `Style::new()`. The centre of the band adds bold to the
accent style it's given, and the neighbours use it as is.

The hourglass frames and their timing come from
[basecamp/hey-cli](https://github.com/basecamp/hey-cli) under its MIT
licence; see [NOTICE.md](NOTICE.md). The
[hourglass's guides](https://docs.pitomd.com/pito-tui/hourglass/) take one task
a page.
