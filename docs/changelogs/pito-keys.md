# Changelog

## v0.1.2 — 2026-10-30

### Changed

- pito-keys is now public, under the MIT licence.
- The README covers installing it by tag, building, testing, contributing and the licence, and opens with a demo GIF of the example showing the sheet.
- No API change.

## v0.1.1 — 2026-10-15

### Changed

- The brand now reads PITO MD.
- The ticker is about 4 times faster and a searched sheet about twice as fast: the keymap ranks each mode's hints and writes each bind's search text once, when it is declared.
- The sheet's search keeps its first 64 characters and 8 words, so a pasted page no longer stalls the frame (20 ms down to 6 µs).

## v0.1.0

### Added

- One keymap per app: groups, binds with their keys, what they do, the modes they answer in and an optional context such as "while the cut plays".
- The action a key does where the app is, the first live bind winning, and a check for binds that clash.
- The `?` help sheet's model: the current mode's keys first, the rest after, grouped and searchable with `/`.
- The legend's hints and a ticker line per mode, current first, in the same words as the sheet.
- The sheet's conventions: `?` opens and closes it, Esc closes it, it passes every other key on, and a Help button toggles it.
- Key conversion from crossterm and egui events, behind features.
- An example that drives one keymap from a ratatui view and an egui view.
