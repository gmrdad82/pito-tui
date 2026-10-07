# Changelog

Every release of pito-tui, newest first. Versions follow [Semantic
Versioning](https://semver.org/) as Cargo reads it before 1.0: a change in the
middle number may break an app, a change in the last one never does.

## 0.3.1 (2026-10-07)

- `capture::Recorder` writes frames from an app's own buffers exactly as
  `capture::capture` does: `Recorder::new(dir)`, then `screen(index, name,
  &buffer)` for a screen as a walk names it and `record(name, &buffer)` for
  a state a `capture::Script` of that name reaches, each at the buffer's
  size. An app still on its own loop records its headless dumps with the
  old build, moves onto `Tui::run`, and `capture::compare` checks the new
  build against them. "Adopting pito-tui" has the steps, under "Proving the
  switch".

## 0.3.0 (2026-10-07)

- Frames before and after: `capture::capture` walks every screen and the
  app's key scripts (`capture::Walk`, `capture::Script`, loading or moved to
  a given moment) at a list of sizes and saves every cell to a directory;
  `capture::compare` reports each cell that differs, by screen, size and
  position, before and after, and each frame added or gone. The demo wires
  `--capture DIR` and `--compare DIR`. A safety net for big changes, not a
  test to keep.
- `Cx::run(id, label, Command)` runs a child command and feeds the
  `--progress json` lines it prints (version 1) into the activity band and
  screen live, its output and stderr into the activity's detail, ending on
  the `end` line or the exit status; a running command counts as busy.
  `command::read` parses one line. `Wake::Activity` carries its updates.
- `Cx::pick(Pick)` and `Screen::picked`: a pick-one list over the screen,
  arrows and `enter` to choose, `esc` to cancel.
- `Log`: a pane for a long log or trace that builds only the rows on screen,
  with paging, both ends, find with `n` and `N`, and copy (`y` the page,
  `Y` everything). The activity screen's detail is a `Log`, and
  `Activities::log` gives it the app's words.
- `clock`: `span` and `age` in the app's unit words, and `next_span` and
  `next_age` for the deadline that keeps a label fresh. `progress`: the
  Braille `bar`, `percent`, `share` (with "≈" for an estimate) and the
  `spinner`. `Activity::estimate` draws a bar faint.
- Breaking: `Activity::detail` is a shared `log::Lines`
  (`Arc<Vec<Line<'static>>>`); `Activity::detail(lines)` still builds it,
  and `Activity::shared(lines)` hands over one the app already holds.
- pito-list moves to v0.7.0, which adds `Shared`, a source over the app's
  own items that builds only the rows on screen. pito-tui now depends on
  `serde_json` for the progress lines.
- The demo adds a Log tab (20,000 lines), `c` (the demo running itself as a
  child that prints progress lines), `p` (an accent picked from a list) and
  a time label on Home kept fresh by a deadline.

## 0.2.0 (2026-10-07)

- Breaking: `Screen::status_parts` takes the room the status has on the
  title rule, `status_parts(&self, room: u16)`, so a screen can drop a part
  that won't fit rather than see it cut. A screen that sets only `status`
  is unchanged.
- Activities: the app hands the shell what it has running
  (`Cx::activity`, `Cx::activities`, `Cx::forget`), each with a label, a
  state (running, waiting, done, failed), a progress fraction or a spinner,
  when it started and ended, a status line and the detail lines it opens
  into. The shell draws them as a band flush on the footer, the newest
  first, up to half the room, a finished one fading for 3 s before it
  goes (`Cx::band` turns the band off and on). A key the app picks opens
  the activity screen over any screen, outside the tabs: the tabs unlit,
  only the back keys, its own breadcrumb row, every activity, and each
  one's detail lines. `Tui::activities(Activities)` names it and gives it
  every word it shows. The demo starts fake jobs with `j` and lists them
  with `o`.
- `Screen::tick`: when the loop's moment reaches a screen's `deadline`, the
  shell calls its `tick`, once for each deadline, whichever screen is
  showing, in the loop, in `settle` and in `Tui::advance`.
- `Tui::advance(Duration)` moves a headless run's clock: the hourglass draws
  as if it had run that long and deadlines tick. `Screen::moment` hands a
  screen the loop's moment before each frame, so drawing never reads the
  wall clock; `Tui::bench` moves it a frame at a time.
- `Screen::notice` and `Screen::legend`: the footer's notice line shows the
  quit guard, then what `Cx::say` said, then the screen's own notice (in
  any tone, for as long as the screen keeps it), then its legend while the
  hints show.
- `Screen::lead(room)` puts the screen's styled parts beside the help word
  on the title rule, told the room left after it.
- `Words::leave`, the quit question's own hint line; it falls back to
  `Words::choose` when empty.
- `text::hard_wrap` breaks lines at the width, whatever the spaces.
- The demo takes `--at MS`, which draws the dump again after moving the
  clock that far.

## 0.1.2 (2026-10-07)

- `Screen::status_parts` gives the header's status as several styled parts,
  so a part such as "1 failed" keeps its own colour beside the rest. It
  defaults to `Screen::status` as one part, so a screen that sets `status`
  draws as before.
- `Tui::keep_facts(true)` keeps a screen's facts row while it is drilled in,
  on the row it had before; by default the row still goes with the tabs.
- The README shows the demo running, links to pitomd.com and writes the
  brand as PITO.

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
