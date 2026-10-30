---
title: The header
description: A nested top navigation for ratatui apps, with groups of numbered sections, drill-in with a breadcrumb, and rows that abbreviate to fit.
sidebar:
  order: 0
---

![The demo: stepping through groups and numbered sections, drilling into a nightly check and back, and the double ctrl+c quit hint](../images/header/demo.gif)

The header, `pito_tui::header`, is the top of a [PITO MD](https://pitomd.com)
terminal app, drawn with [ratatui](https://ratatui.rs) 0.30: groups of
numbered sections in two nested rows, drill-in with a breadcrumb, and
optional title, facts and notice rows, all abbreviating to fit the width. It
has no app logic and no words of its own: the app passes in every name, every
style and every key. The shell draws it for every app; an app with its own
loop draws it on its own.

It works on Linux, Windows and macOS.

## At a glance

| | |
|---|---|
| Full header, drawn at 150 columns | **8.7 µs** a frame (mean of 200,000) |
| Nav rows alone / a click hit-test | **2.1 µs** / **0.11 µs** |
| Allocations while drawing or hit-testing | **0** (a counting allocator in the tests holds it) |
| Size | **about 1,540 lines** of Rust, 75 tests |

Measured on 2026-10-14 on an AMD Ryzen 9 9950X with Rust 1.97.1, with
`cargo run --release --example header_bench 200000`.
[Measure draw time](guides/measure/) shows how to run it on your machine.

## Where to start

- [Install](../install/#the-header-on-its-own): the one pito-tui line and a
  first header.
- [Groups and numbered sections](guides/groups-and-sections/): build the nav.
- [Map keys to moves](guides/keys/): choose which keys move it.
- [Fit any width](guides/fit/): how the rows abbreviate.
- [Drill in with a breadcrumb](guides/drill-in/): go deeper and come back.
- [Add title, facts and notice rows](guides/rows/): the optional rows.
- [Style a section in several colours](guides/styled-sections/): spans and live labels.
- [Handle clicks](guides/clicks/): turn a mouse click into a move.
- [Measure draw time](guides/measure/): run the bench.
- [API at a glance](reference/api/), and the full [API reference](../api/pito_tui/header/index.html).
- [Changelog](../changelog/): every release, newest first.

## Part of pito-tui

The header is one of pito-tui's four widgets, with
[the footer](../footer/), [the list](../list/) and
[the hourglass](../hourglass/); [the shell](../) brings them together.

The code is MIT licensed, by Catalin Ilinca. The PITO MD name and its logos
are © Catalin Ilinca, all rights reserved, and not covered by the MIT licence.
