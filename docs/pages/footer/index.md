---
title: The footer
description: The bottom of a PITO MD terminal app, drawn with ratatui 0.30.
sidebar:
  order: 0
---

![The demo example: a task list whose footer shows its key hints and version, a notice, a search input, a delete confirm, the hints hidden by ? and the ctrl+c guard](../images/footer/demo.gif)

The footer, `pito_tui::footer`, is the bottom of a
[PITO MD](https://pitomd.com) terminal app, drawn with ratatui 0.30: key
hints that fit the width, a notice line, a y/n confirm, a one-line input, a
quit guard and the app's version. The look is in the style of HEY's terminal
UI. It has no app logic and no words of its own: the app passes in every hint,
every word, every key and every style, so any language works.

- **8.4 µs** to draw a row of twelve hints with a notice at 150 columns, and
  **38.5 µs** for a frame that draws every widget the footer has: 0.23% of a
  60 fps frame.
- **Zero allocations** while drawing: a counting allocator in the tests holds it.
- **2,357 lines** of Rust.

Works on Linux, Windows and macOS.

## Where to go next

- [Install and first use](../install/#the-footer-on-its-own): the one
  pito-tui line and a first footer.
- Guides, one task a page:
  - [Show key hints that fit the width](guides/hints/)
  - [Show a notice under the hints](guides/notices/)
  - [Lay out a status row](guides/status-row/)
  - [Read a line of text, or a secret key](guides/input/)
  - [Ask a yes or no question](guides/confirm/)
  - [Guard ctrl+c against an accidental quit](guides/quit-guard/)
  - [Feed it keys from crossterm](guides/keys/)
  - [Measure the drawing speed](guides/speed/)
- [API reference](../api/pito_tui/footer/index.html): every type and method, from
  `cargo doc`.
- [Changelog](../changelog/): every release, newest first, and
  [the footer's earlier releases](../changelogs/pito-footer/).

## Part of pito-tui

The footer is one of pito-tui's four widgets. [The shell](../) brings it
together with [the header](../header/) at the top,
[the list](../list/) for the rows and
[the hourglass](../hourglass/) while data loads.
The clips are recorded with [prec](https://docs.pitomd.com/prec/) and made into
GIFs with [pcod](https://github.com/gmrdad82/pito-codecs), and the speed chart is drawn
by [pito-compare](https://docs.pitomd.com/pito-compare/).

The code is MIT licensed. The PITO MD name and its logos are © Catalin Ilinca,
all rights reserved, and are not covered by the MIT licence.
