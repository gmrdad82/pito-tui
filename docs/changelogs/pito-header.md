# Changelog

## v1.1.2 — 2026-10-23

### Changed

- pito-header now needs ratatui 0.30.2 or newer (and so Rust 1.88 or newer), so halfwidth sound marks (゙ ゚) take the same cell on screen that pito-header measures for them, and clicks on such names land on the right section.

## v1.1.1 — 2026-10-22

### Fixed

- Section and crumb names whose letters join, such as Arabic lam-alef, are measured the way ratatui draws them. Before, the breadcrumb could keep a trail that didn't fit and show a stray ellipsis, and a click on the nav bar could land on the wrong section. Plain ASCII names draw and measure as before, just as fast.

### Changed

- The tests now reach every reachable line of the crate: 99.8% of lines and every function, up from 95.2% and 94.3%.

## v1.1.0 — 2026-10-21

### Added

- `NavKeys::zero_first`: with it on, `0` jumps to the first section and `1` to `9` to the second through the tenth, and the section labels show those digits. A section past the tenth has no digit on its label and is reached with `]`, `[` and the group keys. Off by default, so `0` stays the tenth in every app that doesn't turn it on.

### Changed

- The README's licence section shows the PITO MD wordmark.
- The README's GIFs are made with pcod (pito-codecs), which replaced pconv.
- Builds, tests and releases run on the maintainer's machines; the README no longer carries a CI badge.
- The README has a line on asking questions on Discord, and its install line names v1.1.0.

## v1.0.1 — 2026-10-14

The crate's code and API are the same as 1.0.0.

### Added

- Docs pages for the PITO MD docs site: install and a quick start, a guide for each task (groups and numbered sections, keys, fitting the width, drill-in, the optional rows, styled sections, clicks and the bench) and the API reference. Every GitHub Release from this one on carries them as `docs-vX.Y.Z.tar.gz`.

### Fixed

- The README no longer says `Styles` is `#[non_exhaustive]`; its fields are plain.

## v1.0.0 — 2026-10-14

pito-header is 1.0. The crate's code and API are the same as 0.2.0, so moving up needs no change in your app.

### Added

- Two examples to try: `cargo run --example fit --features crossterm` narrows and widens the header so you can watch it abbreviate, and `gallery` shows every optional row, sections in several styles and the breadcrumb at three widths.
- The bench measures five cases: the nav rows alone, the full header, drilled in, abbreviated at 40 columns, and a click hit-test. The full header draws in 8.7 µs at 150 columns on a Ryzen 9 9950X.

### Changed

- The README shows every feature with a GIF or a still, leads with measured numbers and has a 30-second install and first use.
- The demo quits on ctrl+c pressed twice, as every PITO MD terminal app does.
- CI tests every change on Linux, Windows and macOS.
- The brand reads PITO MD, and the names and logos are covered in TRADEMARKS.md, with LICENSE the plain MIT text.

## v0.2.0 — 2026-10-05

### Added

- A section's label changes in place: `Nav::section_mut` with `set_name`, `set_spans`, `set_short` and `set_short_spans`.
- Owned strings draw as facts and slot parts: `facts_pairs`, `left_pairs` and `right_pairs`.

### Changed

- `Key` carries Alt, so Alt+q is not Back, and Ctrl+Alt plus a character passes through as that character, so AltGr input still types.
- `Action`, `Step`, `Key`, `Drill` and `NavKeys` are `#[non_exhaustive]`, and `NavKeys` is built from `NavKeys::HEY`, `NavKeys::NONE` or `NavKeys::default()` with its const methods.
- The facts row skips empty facts and elides only when the room is short.

### Fixed

- Every clipping edge of the title slots, facts, breadcrumb and hit test, and control characters are measured as they are drawn.
- `cycle(0)` stays put.

## v0.1.5 — 2026-10-05

### Added

- `Section::short_spans` gives a section's abbreviation several styled spans, drawn in the short and lone forms as `Section::spans` draws the full one; the last of `short` and `short_spans` wins.

## v0.1.4 — 2026-10-05

### Added

- `Section::spans` gives a section several styled spans that draw in their own styles over the selected section's emphasis, abbreviate and clip like a plain label, and are hit-tested as one cell.

### Changed

- `Group` and `Section` share one name type and the breadcrumb's three trails share one rule for which crumbs they show, with no change to the API or the drawing.

## v0.1.3 — 2026-10-05

### Fixed

- The tenth section's label draws its key, 0, instead of 10, so it reads "0 Requests", and clicks on it still land on section ten.

## v0.1.2 — 2026-10-04

### Added

- `Nav::selected` reads the section any group remembers.
- `NavKeys::swallow_digits` takes digits past the last section as a no-op.
- The title's slots take several styled parts.

### Fixed

- Text on a rule keeps exactly its own style instead of inheriting the rule's modifiers.

## v0.1.1 — 2026-10-04

### Added

- `Drill::Rows` keeps the old rows when drilling in, and `Breadcrumb::centred` serves apps that draw their own.

### Changed

- Drilling in swaps the sections row for a centred breadcrumb rule in the accent and steps the facts and breadcrumb rows aside.

## v0.1.0 — 2026-10-04

### Added

- A ratatui header with groups of numbered sections in two nested rows that abbreviate to fit.
- Drill-in with a breadcrumb that keeps the list's selection.
- Optional title, facts and notice rows, click hits, and configurable keys that map to actions without taking input.
