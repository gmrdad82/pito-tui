---
title: pito-footer
description: The bottom of a PITO MD terminal app, as a small ratatui 0.30 crate.
sidebar:
  order: 0
---

![The demo example: a task list whose footer shows its key hints and version, a notice, a search input, a delete confirm, the hints hidden by ? and the ctrl+c guard](images/demo.gif)

pito-footer is the bottom of a [PITO MD](https://pitomd.com) terminal app, as a
small ratatui 0.30 crate: key hints that fit the width, a notice line, a y/n
confirm, a one-line input, a quit guard and the app's version. The look is in
the style of HEY's terminal UI. It has no app logic and no words of its own:
the app passes in every hint, every word, every key and every style, so any
language works.

- **8.4 µs** to draw a row of twelve hints with a notice at 150 columns, and
  **38.5 µs** for a frame that draws every widget the crate has: 0.23% of a
  60 fps frame.
- **Zero allocations** while drawing: a counting allocator in the tests holds it.
- **3 dependencies:** ratatui without its default features, unicode-width and
  unicode-segmentation (crossterm 0.29 only with the `crossterm` feature).
- **2,078 lines** of Rust, rebuilt in **1.5 s** after a change.

Works on Linux, Windows and macOS.

## Where to go next

- [Install and first use](install.md): the Cargo line and a first footer.
- Guides, one task a page:
  - [Show key hints that fit the width](guides/hints.md)
  - [Show a notice under the hints](guides/notices.md)
  - [Lay out a status row](guides/status-row.md)
  - [Read a line of text, or a secret key](guides/input.md)
  - [Ask a yes or no question](guides/confirm.md)
  - [Guard ctrl+c against an accidental quit](guides/quit-guard.md)
  - [Feed it keys from crossterm](guides/keys.md)
  - [Measure the drawing speed](guides/speed.md)
- [API reference](api/pito_footer/index.html): every type and method, from
  `cargo doc`.
- [Changelog](changelog.md): every release, newest first.

## Part of PITO MD

pito-footer is one of the terminal crates of PITO MD.
[pito-tui](https://docs.pitomd.com/pito-tui/) is the app shell that brings it
together with [pito-header](https://docs.pitomd.com/pito-header/) at the top,
[pito-list](https://docs.pitomd.com/pito-list/) for the rows and
[pito-hourglass](https://docs.pitomd.com/pito-hourglass/) while data loads.
The clips are recorded with [prec](https://docs.pitomd.com/prec/) and made into
GIFs with [pcod](https://github.com/gmrdad82/pito-codecs), and the speed chart is drawn
by [pito-compare](https://docs.pitomd.com/pito-compare/).

The code is MIT licensed. The PITO MD name and its logos are © Catalin Ilinca,
all rights reserved, and are not covered by the MIT licence.
