# Changelog

## v1.3.0 — 2026-10-28

### Changed

- `Keys::new()` now moves half a page with ctrl+d and ctrl+u. Apps on the default key set that read ctrl+d or ctrl+u themselves no longer see them: they used to get `Step::Pass`, and now the list consumes them.
- `Keys::VIM`'s ctrl+d and ctrl+u move half a page instead of a whole page. Page Down and Page Up still move a whole page.
- The demo's footer names the half and whole page keys, and puts "ctrl+c twice quit" right after the move keys, so an 80-column terminal keeps it.

### Added

- `Keys::half_up` and `Keys::half_down`, as fields and as builders, so an app picks its own half page keys or empties them.
- `List::half_up` and `List::half_down`, which move by half the drawn page, rounded up and never less than one row.

## v1.2.1 — 2026-10-26

### Changed

- The demo example uses pito-footer v1.1.0, so its footer text that doesn't fit is cut after a whole word and ends with "…".

## v1.2.0 — 2026-10-26

### Changed

- Text that doesn't fit is cut after a whole word (or after `/`, `-` or `_` in a name) and ends with "…", never in the middle of a word or right after a number. A value with no room for a whole word shows "…" alone.

### Added

- `pito_tui::list::cut`, so apps can cut their own text the same way.

## v1.1.1 — 2026-10-23

### Changed

- The README's "Columns that make room" shows one clip of the demo as the terminal narrows from 96 to 38 columns and widens back, Size and Version dropping and returning in priority order, in place of the three stills.

## v1.1.0 — 2026-10-22

### Added

- `Key::Mod(Mods, Named)`: with the `crossterm` feature, Ctrl and Shift held with a named key (the arrows, Page Up/Down, Home, End, Enter, Esc, Backspace, Delete, and Ctrl on Tab) now reach your app, for example Ctrl+PageDown as `Key::Mod(Mods::CTRL, Named::PageDown)`. Shift+Tab still arrives as `Key::BackTab`. `Mods` and `Named` are new public types and work in `const` key maps.

### Changed

- Shift or Ctrl held with Up, Down, Page Up/Down, Home or End no longer moves the list, and with Enter no longer opens the row, under the built-in key maps: the key passes back to your app. Bind its `Key::Mod` form in your own key map to bring it back.
- The README links the Discord server for questions.

## v1.0.2 — 2026-10-16

### Changed

- The README shows the PITO MD wordmark under its Licence heading, 320 px wide.
- The demo example uses pito-footer 1.0.1.

## v1.0.1 — 2026-10-14

The same API as 1.0.0, with documentation: nothing to change in your code.

### Added

- An install page and ten guides of one task each, in `docs/`: a first list in an app, keys and clicks, columns, sections, ranges, a scrollbar and a count, styles, rows rewritten in place, 100,000 items built on demand, and running the benchmarks. Their examples run as tests.
- Every release now carries `docs-vX.Y.Z.tar.gz`, those pages with an overview, the changelog and the API reference, built by `bin/docs-bundle`.

## v1.0.0 — 2026-10-14

pito-list reaches 1.0.0 with the same API as 0.8.0: nothing to change in your code.

### Added

- A new README: every feature shown with a clip, a still or a chart, the measured numbers up front (about 0.3 ms a frame for 20,000 rows, no allocations in a warm frame, 70× faster than a ratatui `Table` built from every row) and a first-use example.
- `examples/versus.rs`, which draws the same 20,000 items with pito-list and with ratatui's `Table` and prints each one's frame time and allocations.
- `docs/guide.md`: every feature in detail, the whole API and the full example.

### Changed

- The LICENSE file holds the MIT licence alone, TRADEMARKS.md says the names and logos stay reserved, and the brand reads PITO MD.
- CI runs the whole gate on Linux, Windows and macOS on every push and nightly.

## v0.8.0 — 2026-10-08

### Added

- An opt-in scrollbar and a count of the rows on screen: `ListView::scrollbar` and `ListView::count`, with `List::shown()` and `List::position()` for an app's own footer.

### Changed

- The demo quits as every PITO TUI does, on ctrl+c pressed twice.

## v0.7.0 — 2026-10-07

### Added

- `Shared`, a ready source over an `Arc<Vec<T>>` of the app's own items that builds a row only when a draw or a key asks for it, through a closure or a `Build<T>` type; `Arc<Vec<Row>>` and `Arc<[Row]>` are sources too.
- A demo example, whose clip opens the README, which points to PITO at https://pitomd.com.
- A code of conduct, a security policy and CI on GitHub.

### Changed

- The licence's copyright line carries the legal name with its diacritics, Cătălin Ilinca.

## v0.6.1 — 2026-10-06

### Changed

- The gate is tiered: `bin/gate --fast` for every commit, the full gate before a merge, nightly and before every tag.

### Fixed

- A list drew past the right edge of its area when its pinned columns' minimums didn't fit; every write now clips to the list's own area.

## v0.6.0 — 2026-10-06

### Added

- `Styles::cursor` for a marker drawn in its own style over the selected line, `List::columns()` with each drawn column's index, position and width, and `Column::fit` for columns as wide as their widest cell on screen.

## v0.5.0 — 2026-10-06

### Added

- `Cell::set_style`, `Cell::set_part_style` and `Mark::set_style`, which restyle a cell, one part or a mark in place.

## v0.4.0 — 2026-10-05

### Added

- A selected range for Visual modes, with the marker on the cursor row only, kept colours under the selection, a styled header with a key column, and rows rewritten in place without allocating.

## v0.3.0 — 2026-10-05

### Added

- Section headings and blank rows the cursor skips, cells of several styled parts, a cell running to the end of the row, a flexible column, lazy rows through the `Source` trait, and paging by cursor or view.

### Fixed

- With an end line and one line left for rows, the selected last row no longer scrolls out behind the end line.

## v0.2.0 — 2026-10-05

### Added

- A base style that blank cells take, so a page background an app paints shows under the list.

## v0.1.0 — 2026-10-05

### Added

- Selectable list rows for ratatui apps, with a bold marker on the selected row and padded columns that drop by priority, a clipped last column, scrolling, paging and click hits.
