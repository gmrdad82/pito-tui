---
title: pito-header
description: A nested top navigation for ratatui apps, with groups of numbered sections, drill-in with a breadcrumb, and rows that abbreviate to fit.
sidebar:
  order: 0
---

![The demo: stepping through groups and numbered sections, drilling into a nightly check and back, and the double ctrl+c quit hint](images/demo.gif)

pito-header is the top of a [PITO MD](https://pitomd.com) terminal app, as a
small [ratatui](https://ratatui.rs) 0.30 crate: groups of numbered sections in
two nested rows, drill-in with a breadcrumb, and optional title, facts and
notice rows, all abbreviating to fit the width. It has no app logic and no
words of its own: the app passes in every name, every style and every key.

It works on Linux, Windows and macOS.

## At a glance

| | |
|---|---|
| Full header, drawn at 150 columns | **8.7 µs** a frame (mean of 200,000) |
| Nav rows alone / a click hit-test | **2.1 µs** / **0.11 µs** |
| Allocations while drawing or hit-testing | **0** (a counting allocator in the tests holds it) |
| Dependencies | **3**: ratatui, unicode-segmentation, unicode-width; no terminal backend unless you turn on `crossterm` |
| Size | **about 1,540 lines** of Rust, 75 tests |

Measured on 2026-10-14 on an AMD Ryzen 9 9950X with Rust 1.97.1, with
`cargo run --release --example bench 200000`. [Measure draw time](guides/measure.md)
shows how to run it on your machine.

## Where to start

- [Install](install.md): add the crate and draw a first header.
- [Groups and numbered sections](guides/groups-and-sections.md): build the nav.
- [Map keys to moves](guides/keys.md): choose which keys move it.
- [Fit any width](guides/fit.md): how the rows abbreviate.
- [Drill in with a breadcrumb](guides/drill-in.md): go deeper and come back.
- [Add title, facts and notice rows](guides/rows.md): the optional rows.
- [Style a section in several colours](guides/styled-sections.md): spans and live labels.
- [Handle clicks](guides/clicks.md): turn a mouse click into a move.
- [Measure draw time](guides/measure.md): run the bench.
- [API at a glance](reference/api.md), and the full [API reference](api/pito_header/index.html).

## Part of PITO MD

pito-header is one of the ratatui crates behind PITO MD's terminal apps, with
[pito-footer](https://docs.pitomd.com/pito-footer/),
[pito-list](https://docs.pitomd.com/pito-list/),
[pito-hourglass](https://docs.pitomd.com/pito-hourglass/) and
[pito-tui](https://docs.pitomd.com/pito-tui/), the shell that brings them
together.

The code is MIT licensed, by Catalin Ilinca. The PITO MD name and its logos
are © Catalin Ilinca, all rights reserved, and not covered by the MIT licence.
