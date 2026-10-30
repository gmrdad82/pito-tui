---
title: The hourglass
description: An hourglass for ratatui apps to show while data loads, with a label that shimmers in the app's accent.
sidebar:
  order: 0
---

![The example: the hourglass drains and flips while "Loading the data..." shimmers under it, above the hint "esc stops waiting"](../images/hourglass/demo.gif)

The hourglass, `pito_tui::hourglass`, is what a ratatui app shows while data
loads: HEY's 13-frame Braille hourglass, the app's copy (the label)
shimmering under it in the app's accent, and an optional hint under that. One
widget, one call a frame. It's part of [PITO MD](https://pitomd.com).

- **4.8 µs a draw** with the glass, the label and the hint, under 0.03% of a
  60 fps frame; 1.0 µs on a single row ([how it's measured](guides/performance/)).
- **Zero allocations** when it draws, held by a counting-allocator test in
  the gate.
- **Small:** one 284-line source file and a 96-byte `Copy` widget.

Works on Linux, Windows and macOS.

## Where to go

- [Install](../install/#the-hourglass-on-its-own): the one pito-tui line and
  the first hourglass.
- [Show it while data loads](guides/show-while-loading/): wire it into an app's event loop.
- [Your copy and your colours](guides/copy-and-styles/): the label, the hint and the three styles.
- [Small areas](guides/small-areas/): what it draws as the space shrinks.
- [Frames and timing](guides/frames-and-timing/): `frame_at`, `band_at`, `CYCLE` and `FRAMES`.
- [Performance](guides/performance/): the draw time per area and how it's measured.
- [API reference](../api/pito_tui/hourglass/index.html): the `cargo doc` pages.
- [Changelog](../changelog/): every release, and
  [the hourglass's earlier releases](../changelogs/pito-hourglass/).

## Part of pito-tui

The hourglass is one of pito-tui's four widgets, with
[the header](../header/), [the footer](../footer/) and [the list](../list/);
[the shell](../) shows it while a screen loads.

The code is MIT licensed, by Catalin Ilinca. The PITO MD name and its logos
are © Catalin Ilinca, all rights reserved, and are not covered by the MIT
licence. The hourglass frames and their timing come from
[basecamp/hey-cli](https://github.com/basecamp/hey-cli) under its MIT licence.
