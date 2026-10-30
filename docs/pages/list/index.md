---
title: The list
description: "Selectable list rows for ratatui apps: columns that make room, sections, ranges, a scrollbar and a count, about 0.3 ms a frame and no allocation in a warm frame."
sidebar:
  order: 0
---

![The demo example: jobs in sections, sizes that grow in place, a selected range, and the view following the selection to the end with a scrollbar and a count of the rows on screen](../images/list/demo.gif)

The list, `pito_tui::list`, is the list behind the
[PITO MD](https://pitomd.com) terminal apps: selectable list rows for
[ratatui](https://ratatui.rs) apps, in the style of HEY's terminal UI. The
selected row gets a bold "▌ " marker, the columns pad to line up and drop by
priority when the terminal narrows, and the list scrolls, pages, selects
ranges and answers clicks. It has no words of its own: every word, style and
key comes from your app, so any language works.

It is fast and stays fast as the list grows: **about 0.3 ms a frame** for a
20,000-row list at 150×40, the same for 100,000 items built on demand, and
**zero heap allocations** in a warm frame. Drawing the same 20,000 items, it
is **70× faster** than a ratatui `Table` built from every row.

Works on Linux, Windows and macOS.

## At a glance

| | |
|---|---|
| Frame time, 20,000 owned rows, 150×40 | 289 µs mean |
| Frame time, 100,000 items built on demand | 292 µs mean (lazy, in groups), 354 µs (shared vector, scrollbar and count) |
| Heap allocations in a warm frame, owned rows | 0 (held by a counting-allocator test) |
| Against ratatui 0.30 `Table`, 20,000 items | 309 µs vs 21,692 µs, 0 vs 260,062 allocations a frame |
| Source | 2,165 lines of Rust in `src/list/` |

Measured once on 2026-10-14 on an AMD Ryzen 9 9950X under Linux, Rust 1.97.1,
release profile, with `cargo run --release --example list_bench` and
`cargo run --release --example list_versus`. Windows and macOS weren't timed.

## Where to go next

- [Install](../install/#the-list-on-its-own) adds pito-tui to your app and
  draws a first list.
- [Draw a list in your app](guides/first-list/) is the first guide; the
  others each take one task, from columns and sections to 100,000 items:
  - [Handle keys and clicks](guides/keys-and-clicks/)
  - [Make columns fit any width](guides/columns/)
  - [Group rows under headings](guides/sections/)
  - [Select a range](guides/ranges/)
  - [Show a scrollbar and a count](guides/scrolling/)
  - [Theme the list](guides/styles/)
  - [Update rows in place](guides/live-rows/)
  - [Show 100,000 items](guides/big-lists/)
  - [Measure it on your machine](guides/measure/)
- [Every feature and the API](reference/api/) has the whole API on one
  page, and the [generated API reference](../api/pito_tui/list/index.html) every item.
- The [changelog](../changelog/) lists what each release changed, and
  [the list's earlier releases](../changelogs/pito-list/) are listed on their own.

## Part of pito-tui

The list is one of pito-tui's four widgets, with [the header](../header/),
[the footer](../footer/) and [the hourglass](../hourglass/);
[the shell](../) draws every list it shows with it.
