# Changelog

Every release of pito-tui, newest first. Versions follow [Semantic
Versioning](https://semver.org/) as Cargo reads it before 1.0: a change in the
middle number may break an app, a change in the last one never does.

## 0.1.1 (2026-10-07)

- `pito_tui::restore()` gives the terminal back from any thread, once, for
  an app whose panic hook ends the process: a panic on a worker thread no
  longer leaves the terminal raw. The first call turns off what `Term`
  turned on, after any frame being drawn, and later calls do nothing; the
  shell draws nothing after it.
- While the quit guard is armed and an input bar or a confirm is open, the
  quit guard's word (`Words::again`) takes the bar's hint row in the accent,
  since the footer draws no notice line under a bar; the bar's own hint
  comes back when the guard lapses.

## 0.1.0 (2026-10-07)

- The first release: `Tui`, the app shell, built from a name, a version and an
  accent colour, with screens in groups, pinning and re-exporting pito-header
  v0.2.0, pito-footer v0.4.0, pito-hourglass v0.1.3 and pito-list v0.6.1.
- `Screen`, a pluggable screen: draw, keys, paste, mouse, hints, facts, a
  status, a breadcrumb with back, an input bar, a busy count, and reads that
  the shell runs on workers and delivers only while they are the newest
  (`load`, `loaded`, `sources`, `cancel`), with the hourglass while one runs.
- `Cx`, what a screen asks of the shell: one-off work, a waker for its own
  threads, a notice, a confirm, an OSC 52 copy, a jump, a reload, a changed
  source, a new palette, new words, new hints, or quitting.
- A 120 fps paced loop that redraws only what changed, in synchronized
  updates, and sleeps until the next wake or deadline.
- Keys in a fixed order: release, the double `ctrl+c` quit guard (asking
  while work runs), the confirm, a typing screen, help, nav, the stop key
  while loading, the app's own keys, the screen.
- `Term`, a terminal guard that turns off exactly the modes it turned on, on
  quit, on an error and on a UI-thread panic, before the app's own hook runs.
- `Palette` (from one accent, or token by token, swappable at runtime) and
  `Words` (every visible word is the app's).
- `Filter`, a filter input over a pito-list; `text` helpers by cell; and
  headless `Tui::shot`, `Tui::bench`, `dump::size`, `dump::keys`,
  `dump::text` and `dump::ansi`.
- `examples/demo.rs`: two screens, a read with the hourglass, a filtered
  list with a drill-in, an app-wide palette key and the headless flags.
