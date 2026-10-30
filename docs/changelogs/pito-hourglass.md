# Changelog

## v1.0.2 — 2026-10-23

### Fixed

- Labels and hints with halfwidth katakana sound marks (ﾞ ﾟ) now draw in full and centre correctly; glyph widths are measured the way ratatui draws them.

### Changed

- Needs ratatui 0.30.1 or later, and depends on one crate fewer (unicode-width is gone).
- The README shows the PITO MD wordmark under its Licence heading and a line to ask questions on Discord.

## v1.0.1 — 2026-10-14

### Added

- Documentation for docs.pitomd.com: an overview, install for Linux, Windows and macOS, five guides with runnable code, the API reference and the changelog, attached to every release as docs-vX.Y.Z.tar.gz.

### Changed

- The README pins v1.0.1. The API and the picture are the same as 1.0.0.

## v1.0.0 — 2026-10-14

### Added

- The README shows every feature with a picture and leads with measured numbers: 4.8 µs a draw, zero allocations, and no new crates for a ratatui app.
- Examples to run and read: `demo` (the clip at the top of the README), `gallery` (four apps' copy and accents), `fit` (what it draws as the area shrinks), `frames` (the 13 frames and when each starts) and `bench` (the draw time per area).

### Changed

- The crate is 1.0.0, with the same API and the same picture as 0.1.3.
- CI tests every change on Linux, Windows and macOS.
- LICENSE is the MIT text alone, under Cătălin Ilinca's name, and TRADEMARKS.md keeps the PITO MD name and logos.

## v0.1.3 — 2026-10-05

### Changed

- Drawing a label or a hint no longer allocates on any frame, with the same picture, and a counting-allocator test holds that drawing allocates nothing.
- Widths and positions add with saturation, so a very wide label can't overflow them.
- The crate's description, tests and README speak to any ratatui app.
- `rust-version` is dropped, since no gate tests a minimum.

## v0.1.2 — 2026-10-04

### Changed

- `CYCLE` is one full turn of the glass, drain plus flip, derived from the frame timing.

## v0.1.1 — 2026-10-04

### Changed

- The crate is public, with Gamer Dad as its only author, and its repository points at its public home.
- The README shows the https source to pin.

## v0.1.0 — 2026-10-03

### Added

- The Braille hourglass spinner as a ratatui widget, its copy shimmering in the app's accent.
