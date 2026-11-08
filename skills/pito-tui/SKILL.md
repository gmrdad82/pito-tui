---
name: pito-tui
description: >
  Use when building or changing a terminal app on pito-tui, the shared Rust TUI shell.
  Triggers: pito-tui, pito_tui, Tui, Screen, Cx, Section, Group, header, footer, hourglass,
  list, keys, Palette, Words, Activities, Log, Pick, Filter, headless dumps, --dump, --keys,
  --bench, --capture, --compare, Tui::shot, moving an app onto the shell.
---

# pito-tui Skill

pito-tui is a Rust library (MIT) that gives a terminal app its shell: the header,
footer, hourglass and list, the terminal guard and panic restore, a paced 120 fps
loop, key routing with the double ctrl+c quit guard, generation-tracked loads, an
activity band and screen, a palette from one accent, and headless dumps and benches.
An app supplies screens, words and data; the shell draws and runs them.

## Hard rules

- Never launch a TUI on the desktop. Draw it headless: `--dump WxH --keys "..."`,
  `Tui::shot`, `Tui::bench`. Read [`headless.md`](headless.md).
- Drawing reads the loop's moment, never the wall clock or input. Time comes from
  the run (`Screen::moment`, `Tui::advance` headless), so the same app draws the same frame.
- The shell has no words of its own. Every string comes through `Words`,
  `Activities` and each screen's hints, facts, notices and labels. An empty word is not drawn.
- Generic building blocks only: nothing app-specific goes into pito-tui.
- Pin pito-tui by git tag. Reach ratatui and crossterm through `pito_tui::ratatui`
  and `pito_tui::crossterm`, never as their own dependencies.
- Keep dependencies few; no comments in code.

## Topic Guides

Read the matching guide before starting. Guides live next to this file:

- [`headless.md`](headless.md) - `Tui::shot`, `dump::keys`, `dump::text`, `--dump`, `--keys`, `Tui::bench`, capture, the key-script syntax
- [`adopting.md`](adopting.md) - moving an app that has its own loop onto pito-tui, and linking this skill
- [`screens.md`](screens.md) - `Screen`, `Cx`, tabs, groups, the header and footer, activities

The full guides are in the repository under `docs/pages/guides/` and the formats in
`docs/pages/reference/formats.md`.

## If you want X, read this guide

| If you want | Use |
|---|---|
| The shell an app builds from a name, a version and an accent, with screens in groups | `Tui`, [`screens.md`](screens.md) |
| A pluggable screen: draw, keys, loads, ticks, hints, facts, notices, breadcrumbs, confirm, copy | `Screen` and `Cx`, [`screens.md`](screens.md) |
| Running work shown as a band flush on the footer and a screen with each one's detail | `Activities` and `Activity`, [`screens.md`](screens.md) |
| Every colour and every word from the app, both changeable while it runs | `Palette` and `Words`, [`screens.md`](screens.md) |
| A loop of the app's own | `Term`, `Pace`, `Wake`, `Waker`, `restore`, [`adopting.md`](adopting.md) |
| A filter over a list, width-aware text, `--dump`/`--keys`/`--ansi`/`--bench` | `Filter`, `text`, `dump`, [`headless.md`](headless.md) |
| A long-log pane with find and copy, a pick-one prompt, age and duration labels, Braille bars and spinners | `Log`, `Pick`, `clock`, `progress` |
| A child command's `--progress json` lines fed into the activity band | `Cx::run` and `command`, [`formats`](formats.md) |
| Every screen and key script saved cell by cell, compared after a change | `capture`, [`headless.md`](headless.md) |
| Move an existing app onto the shell | [`adopting.md`](adopting.md) |
