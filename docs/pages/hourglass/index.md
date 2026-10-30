---
title: pito-hourglass
description: An hourglass for ratatui apps to show while data loads, with a label that shimmers in the app's accent.
sidebar:
  order: 1
---

![The example: the hourglass drains and flips while "Loading the data..." shimmers under it, above the hint "esc stops waiting"](images/demo.gif)

An hourglass for ratatui apps to show while data loads: HEY's 13-frame Braille
hourglass, the app's copy (the label) shimmering under it in the app's accent,
and an optional hint under that. One widget, one call a frame. It's part of
[PITO MD](https://pitomd.com).

- **4.8 µs a draw** with the glass, the label and the hint, under 0.03% of a
  60 fps frame; 1.0 µs on a single row ([how it's measured](guides/performance.md)).
- **Zero allocations** when it draws, held by a counting-allocator test in
  the gate.
- **No new crates for a ratatui app:** its two dependencies (ratatui 0.30.1
  with no default features, and unicode-segmentation) are all
  already in ratatui's own graph, and it brings no backend, so it sits beside
  whatever crossterm the app uses.
- **Small:** one 284-line source file and a 96-byte `Copy` widget.

Works on Linux, Windows and macOS.

## Where to go

- [Install](install.md): add the crate and draw the first hourglass.
- [Show it while data loads](guides/show-while-loading.md): wire it into an app's event loop.
- [Your copy and your colours](guides/copy-and-styles.md): the label, the hint and the three styles.
- [Small areas](guides/small-areas.md): what it draws as the space shrinks.
- [Frames and timing](guides/frames-and-timing.md): `frame_at`, `band_at`, `CYCLE` and `FRAMES`.
- [Performance](guides/performance.md): the draw time per area and how it's measured.
- [API reference](api/pito_hourglass/index.html): the `cargo doc` pages.
- [Changelog](changelog.md): every release.

The code is MIT licensed, by Catalin Ilinca. The PITO MD name and its logos
are © Catalin Ilinca, all rights reserved, and are not covered by the MIT
licence. The hourglass frames and their timing come from
[basecamp/hey-cli](https://github.com/basecamp/hey-cli) under its MIT licence.
