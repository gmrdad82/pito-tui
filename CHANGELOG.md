# Changelog

Every release of pito-tui, newest first. Versions follow [Semantic
Versioning](https://semver.org/): from 1.0.0 on, only a change in the first
number may break an app; the middle number adds, the last one fixes.

## v2.0.0 — 2026-10-30

### Added

- `pito_tui::list::Keys::half_up` and `pito_tui::list::Keys::half_down`, as fields and as builders, so an app picks its own half page keys or empties them.
- `pito_tui::list::List::half_up` and `pito_tui::list::List::half_down`, which move by half the drawn page, rounded up and never less than one row.

### Changed

- Lists move half a page on ctrl+d and ctrl+u with `pito_tui::list::Keys::new()`. An app that reads ctrl+d or ctrl+u itself while a list has focus no longer gets them: the list used to pass them on with `Step::Pass`, and now it takes them.
- `pito_tui::list::Keys::VIM`'s ctrl+d and ctrl+u move half a page instead of a whole page. Page Down and Page Up still move a whole page.
- The list demo (`cargo run --example list_demo`) names the half and whole page keys in its footer and puts "ctrl+c twice quit" right after the move keys, so an 80-column terminal keeps it.

- The header, footer, hourglass and list are part of pito-tui: `pito_tui::header`, `pito_tui::footer`, `pito_tui::hourglass` and `pito_tui::list` are modules of the crate, with their tests, examples (`cargo run --example <module>_<name>`), renders, guides and images, and the four git dependencies on pito-header, pito-footer, pito-hourglass and pito-list are gone.
- Every public path under `pito_tui::` is unchanged, so an app that imports through pito-tui builds as before.
- An app that pinned pito-header, pito-footer, pito-hourglass or pito-list itself drops those lines for pito-tui's one line and imports through `pito_tui::header`, `pito_tui::footer`, `pito_tui::hourglass` and `pito_tui::list`. It no longer needs `features = ["crossterm"]`: the header's, footer's and list's `Key::from(crossterm::event::KeyEvent)` is always there.
- The README, the module pages and docs.pitomd.com carry every section, guide and image of the four: a part each in the README, a section each in the docs, and one install page.
- Each module's earlier releases are listed in its own changelog: [pito-header](docs/changelogs/pito-header.md), [pito-footer](docs/changelogs/pito-footer.md), [pito-hourglass](docs/changelogs/pito-hourglass.md) and [pito-list](docs/changelogs/pito-list.md).

## v1.4.0 — 2026-10-26

### Added

- `pito_tui::list::cut` cuts an app's own text the way lists cut theirs (pito-list v1.2.0).
- `pito_tui::text::cut` gives apps the same word cut for any text.

### Changed

- Text that doesn't fit in a list is cut after a whole word (or after `/`, `-` or `_` in a name) and ends with "…", never in the middle of a word or right after a number (pito-list v1.2.0).
- The status line, the title, tab tails, fold titles, toasts and activity rows cut the same way, and the footer and legend do too (pito-footer v1.1.0). Text that fits is unchanged.

## v1.3.0 — 2026-10-23

### Added

- `pito_tui::list` is pito-list v1.1.1: `Key::Mod(Mods, Named)` brings Ctrl and Shift held with a named key (the arrows, Page Up/Down, Home, End, Enter, Esc, Backspace, Delete, and Ctrl on Tab) to your app, with `Mods` and `Named` as new public types.

### Changed

- Shift or Ctrl held with Up, Down, Page Up/Down, Home or End no longer moves a list, and with Enter no longer opens its row, under the built-in key maps: the key goes on to your app. Bind its `Key::Mod` form in your own key map to bring it back.

## v1.2.4 — 2026-10-23

### Changed

- pito-tui now needs ratatui 0.30.2 or newer (and so Rust 1.88 or newer), as pito-header v1.1.2 does.

### Fixed

- Header names with halfwidth sound marks (ﾞ ﾟ) take the cell on screen that is measured for them, so they draw and take clicks where they show (pito-header v1.1.2).

## v1.2.3 — 2026-10-23

### Fixed

- Hourglass labels and hints with halfwidth katakana sound marks (ﾞ ﾟ) now draw in full and centre correctly (pito-hourglass v1.0.2).

## v1.2.2 — 2026-10-23

### Fixed

- Toasts at the bottom of a short terminal no longer pile up on the top row over each other: a toast that doesn't fit whole at its spot is left out until older ones leave, at every edge.

## v1.2.1 — 2026-10-23

### Fixed

- A tab that `Cx::show` brings up while an overlay is open now starts and loads when the overlay closes; before, it was drawn but never loaded.
- A modal list cut short by a small terminal or narrowed by its filter grows back to its full rows once there is room again.
- Arming a confirm in one of a modal's lists shows that list's own warning, not another list's at the same position.

### Changed

- About 60 new tests for the shell's wake handling, deadlines and builder, and for modals, takeovers and toasts; line coverage is now 94%.

## v1.2.0 — 2026-10-22

### Added

- `Folds` makes a column of titled sections collapsible: each one is either expanded (its title rule, then its body) or collapsed to one row with a summary you give on its title rule. A fold eases open and shut (instant with `still(true)` for reduced motion), your keys toggle the section under the cursor or all of them, and `state()`/`restore()` hand you the folds as plain data to save. A steady column allocates nothing per frame.
- `Screen::fact_spans` and `Screen::hint_list` let a screen lend the footer its facts and hints, the way `Screen::badge` lends the header its pill, so they cost nothing per frame.
- About 390 new tests: key order, the quit guard, tabs and groups, overlays, the activity band and screen, commands, the log, pick, capture and dump files, and the text, layout, palette, clock and progress helpers. Line coverage is now 90%.

### Changed

- A steady frame allocates nothing: the header title, the left part, the facts and the hints reuse their buffers from frame to frame.
- `Tui::event`, `Tui::start` and `Tui::settle` return a `Flow`, and the new `Tui::landed` returns the `Flow` of a load's result; calls written as statements keep compiling.
- The activity screen stays the last screen even when an overlay is added after `Tui::activities`, as the docs promised: such an overlay, and a gate or app added after the board, now sit one index earlier.

### Fixed

- `cx.quit()` from a screen's `event`, `loaded`, `start` or `entered` hook now quits; before, it was ignored until the next key.
- A progress bar shows 100% only once the work is done.
- Capture and compare refuse frame files whose size line or rows don't match, scenario names with no letters or digits, and two scenarios that would write one folder, instead of reading wrong frames or overwriting one; `dump::size` takes digits only.

## v1.1.0 — 2026-10-21

### Added

- `Screen::badge` puts a short coloured pill (a licence tag, say) at the right end of the header rule, after the status and one space. It borrows the parts the screen keeps, so a steady badge costs nothing per frame; a badge wider than its room is left out rather than cut.
- Tabs can be numbered from 0: with `NavKeys::HEY.zero_first(true)` the first tab is on `0`, the next nine on `1` to `9`, and tabs past the tenth show no digit and are reached with `]`, `[` and the group keys. Every other app keeps today's numbering.

### Fixed

- In kitty, the mouse wheel no longer moves a list as if arrow keys were pressed: on the alternate screen the shell turns off alternate scroll and drops the wheel's arrow pairs, while held keys and real arrows work as before.

### Changed

- Pins pito-header at v1.1.0.
- The README names pcod (pito-codecs) as the tool that makes its GIFs, and has a line to ask questions on Discord.

## v1.0.3 — 2026-10-16

### Changed

- Pins pito-list at v1.0.2, the same API as v1.0.1.

## v1.0.2 — 2026-10-16

The API is the same as 1.0.1: nothing to change in your app.

### Added

- The README shows the PITO MD wordmark in its Licence section.

### Changed

- pito-tui now brings pito-header, pito-footer, pito-hourglass and pito-list v1.0.1 (from 0.2.0, 0.4.0, 0.1.3 and 0.8.0), each with the same API as before.
- CI saves its cargo cache only from main.

## v1.0.1 — 2026-10-14

### Added

- Docs for the PITO MD docs site: an overview, install and first app, eleven guides with real output from the demo, the formats pito-tui reads and writes, and the full API, attached to every GitHub Release as `docs-vX.Y.Z.tar.gz`.

## v1.0.0 — 2026-10-14

### Changed

- pito-tui is 1.0.0: the API an app writes against is the one 0.4.1 had, and from here only a 2.0 may break it.
- Works on Linux, Windows and macOS, and every change is now built and tested on all three.
- The README shows pito-tui at a glance: a frame drawn in 0.07 to 0.31 ms against the 8.3 ms of 120 fps, a 30-second start, and a GIF for every feature.
- The brand reads PITO MD in the README, the crate description and TRADEMARKS.md.

## 0.4.1 (2026-10-08)

- No command outlives a hangup or a termination either. While `Tui::run` or
  `run_in` runs, on Unix, `SIGHUP` (the terminal going away: a closed tmux
  pane or window, say), `SIGTERM` and `SIGINT` end the loop as a quit does,
  without asking: every command the shell started, forgotten ones too, gets
  `SIGTERM` to its process group and `SIGKILL` after `command::GRACE` if it
  still runs, a command queued but not yet started never starts, the
  terminal is given back, and then the process ends by that same signal, as
  it would have without the shell. Before, the app died at once and its
  commands, each in a process group of its own that the hangup never
  reaches, ran on. Before the loop and after it, those signals keep their
  default action.
- `Wake::Signal(number)` is the wake that ends the loop for such a signal;
  `Tui::handle` answers it with `Flow::Quit`.

## 0.4.0 (2026-10-08)

Every PITO TUI can now run the whole shell: the building blocks an app that
kept its own loop was missing, each a general primitive with no app's words
or shape in it.

### The header

- `Tui::head(Head)` draws the header as a stack of rows in the app's order:
  `Row::Title`, `Groups`, `Caption` (a ruled label between two rows, the
  current group's name unless the screen's `caption()` gives one),
  `Sections`, `Facts`, `Crumbs` and `Blank`. `Head::numbers(Numbers::Across
  | InGroup | Off)` numbers the tabs across the app, within their group, or
  not at all, and `Head::lone(true)` leaves a group of one tab unnumbered;
  number keys follow what is drawn. Without `head` the header is
  pito-header's, exactly as before, and `header_look` applies there.
- `Label`: a group's or a tab's name as styled spans (an underlined
  accelerator letter, say) with a short form, taken by `Tui::group` and
  `Tui::screen` (a `Section`, a `Group`, `&str` or `String` still work).
- `Tui::title`, `Cx::title` and `Cx::title_spans` set the title on the rule
  at runtime, in the new `Palette::title` style, which the spans patch.
- `Screen::after_tabs()` puts the screen's own spans after the tabs on the
  sections row, a "‹ value ›" selector, say.
- `Tui::focus_keys(next, previous)` moves keyboard focus through the groups
  row, the sections row, the after-tabs slot and the content: the focused
  row's current item takes `Palette::focus`, left and right move along it,
  up and down move between rows, `esc` or `enter` gives focus back to the
  content, and in the slot keys reach the screen with `Cx::focus()` saying
  `Focus::Slot`.
- `Tui::show` and `Cx::show(|Item| bool)` show or hide groups and tabs
  (`Item::Group(n)`, `Item::Tab(screen)`); the nav is rebuilt then, keeping
  each group's last tab and the current tab when it is still shown.
- `Tui::group_keys(keys)` gives the group added last its keys, and
  `Cx::go_group(n)` returns to a group's last-used tab.

### The app slot and layers

- `Tui::app(screen)`: one app-level screen, never drawn as content. Its
  `lead`, `left` and `status_parts` feed the title rule on every screen (a
  screen's own only when it gives none), it hears app-wide keys first in
  the new `Screen::claim(key, cx) -> bool` (over a gate, a typing screen
  and any screen; `Cx::typing()` says when the screen in front takes
  text), and what it opens answers to it.
- `Tui::gate(screen, shut)` and `Cx::gate(bool)`: a layer before the app,
  the title rule over the gate's own box, no tabs, no help, only the
  gate's own hints; every key is the gate's after the quit guard, open
  prompts and the app's claim. The capture walk shows each screen past
  the gate.
- `Cx::takeover(Takeover)` and `Cx::end_takeover()`: an app-wide
  transition, its text on the title rule and the hourglass under it, tabs
  and footer hidden, every key ignored but its cancel keys (the app hears
  `layer::Event::Closed`) and the quit guard, whose word and question take
  a bare footer line.
- `layer::Modal`, opened with `Cx::open`, a container over the live screen:
  centred, bordered and titled, owning the keys until it closes, its body
  built from parts in order (`text`, a `list` of `Choice`s, a `Field`), tab
  and shift+tab moving focus, `Modal::filter(field, list)` narrowing a list
  as the field is typed, `Choice::skip()` for rows the cursor passes over,
  `Choice::confirm(warning)` for a row that needs a second enter with the
  warning on the status line, and `Modal::status(text, Tone)`. `Cx::modal(id)`
  changes it while it is open, `Cx::close(id)` closes it, and the screen
  that opened it hears `Screen::layer(id, Event)`: `Chosen`, `Moved`, `Edited`,
  `Submitted` or `Closed`. Modals stack. `Cx::alert(id, title, lines)` and
  `Modal::alert` are a dismiss-only one: `esc`, `enter` or `q` closes it.
- `Cx::toast(Toast)`: app-wide notices that survive screen changes, at a
  `Spot` over the content (top right unless placed), in a `Tone`, stacked,
  leaving on their own after their time (3 s unless set) with one redraw.
- `Tui::top_band(impl Band)`: the same `Band` trait, under the header.
- `Tui::quit_sticky(true)` keeps the armed quit guard through other keys
  from any layer; `Tui::quit_tone(footer::Tone)` and `Tui::quit_hints(false)`
  give its word a tone and hide the hints while it is armed.

### Layout, palette and timing

- `Layout` (`Tui::layout`, `Cx::layout`): the top and bottom margins, the
  side margin, the content's padding, the gap under the header and the gap
  above the footer, and the `Spot` where the too-small words sit, drawn in
  the new `Palette::small`. `TOP`, `SIDE`, `PAD` and `GAP` stay as its
  defaults. `Spot` places a box by `Edge` (`Start`, `Third`, `Middle`,
  `End`) on each axis; `message_at` places text by one.
- `Palette` gains `strong`, `warn` (apart from `bad`), `title`, `inactive`
  (apart from `muted`), `focus` (inverse), `small`, `bar` and `count`, and a
  shell `Tone` (`Ink`, `Strong`, `Muted`, `Accent`, `Good`, `Warn`, `Bad`)
  for toasts and modal status lines (`Palette::tone`).
- `Tui::role(name, style)` and `Cx::role(Some(name))`: named colour roles,
  one per app mode; the active one colours the title and the hourglass.
- `Palette::mono()`: reverse, bold and dim stand in for colour. `Tui::run`
  applies it when `NO_COLOR` is set, and `Tui::monochrome(true)` forces it;
  headless runs stay as built.
- `Tui::hourglass_timing(delay, least)`: a load shorter than the delay shows
  nothing, and a shown hourglass stays at least `least`.
- `Tui::version_fit(true)` shows the footer's name and version only when
  they cost no extra row and the quit guard is quiet.
- `Cx::epoch()`: detach jobs and reads started before it, and waker events
  sent before it, are dropped, and every started screen reads again. A
  waker made earlier still delivers what it sends after.

### Lists (pito-list v0.8.0)

- pito-list moves to v0.8.0 and is re-exported. `Palette::bar` and
  `Palette::count` hold the app's scrollbar (`list::Bar`) and range count
  (`list::Count`, every word the app's), and `Palette::view(list, columns)`
  gives any screen's list the app's styles, bar and count. The pick, the
  activity screen, `Filter::view` and the modal lists use it, and `Log`
  draws the same bar beside its lines and the count on its bottom line.

### Breaking

- `FooterLook` is `fn(Footer, u16) -> Footer`: the look gets the footer's
  width.
- `Tui::group` and `Tui::screen` take `impl Into<Label>`; a `Group` passed
  in keeps its name and short name, not sections built into it.
- `Wake` has a `Stamped(epoch, screen, E)` variant: detach results and
  events from wakers made by `Cx::waker` and `Tui::waker` arrive as it.
  `Waker::new` still sends plain `Wake::Event`.
- `Palette` has new fields, so a palette built field by field needs them
  (it is `#[non_exhaustive]`; build it with `new` and the builders).
- `pito_tui::Tone` is the shell's tone; the footer's is still
  `pito_tui::footer::Tone` (`Cx::say` and `quit_tone` take it).
- While a modal or pick is open, the footer keeps only the app's pinned
  hints, since the others' keys don't reach the app then.

## 0.3.7 (2026-10-08)

- `capture::compare_dirs(before, after)` compares two capture folders frame
  by frame and cell by cell, with the same `Difference` list `compare`
  gives: each cell that differs, each frame only `after` holds (`Added`)
  and each one only `before` holds (`Removed`). Either folder may come from
  `capture::capture` or from a `Recorder`.
- `Recorder::against(dir)` compares each frame with the one of the same
  name in an existing capture instead of writing it, and `finish()` lists
  the differences. An app that keeps its own loop draws its states into one
  `&mut Recorder` for both `--capture` and `--compare`; "Proving the
  switch" in the README shows it.

## 0.3.6 (2026-10-07)

- No command outlives the loop that started it. When `Tui::run` or `run_in`
  returns, every command started with `Cx::run` or `Cx::run_from` that is
  still running gets `SIGTERM` to its process group, as `Cx::stop` sends;
  `run_in` waits up to `command::GRACE` (2 s) for them, sends `SIGKILL` to
  the ones still running, and only then returns. A command queued but not
  yet started never starts. Before, a quit left them running, and a
  command that ignores a closed pipe ran to its end.
- After `Cx::forget`, the shell no longer keeps the command's lines nor
  copies them on every post; the screen still hears each line in
  `Screen::heard`. An app without an activity board keeps none at all.

## 0.3.5 (2026-10-07)

- `Tui::band(impl Band)`: the app draws the band from its own data, in its
  own columns and with its own cap and "more" line. The shell asks its
  `height` for the room between the header and the footer, places it flush
  on the footer with the content's blank row above, draws it on every
  screen and under every overlay, picker and confirm, and wakes for its
  `deadline`. It replaces the activity band.
- `Tui::overlay(section, keys, screen)` opens an app's own screen over any
  screen as the activity screen opens: by its keys, the tabs unlit, only
  the back keys, its own breadcrumb row ("Operations / update pfx"), and
  its confirms and picks answered to it. Overlays come after every tab.
- `Tui::lit_again(false)`: while the quit guard is armed, its word takes an
  input bar's hint row in the bar's own hint style, and a confirm keeps its
  own hint.
- `Screen::left(room, help)` draws the header's whole left side, the help
  word (when the shell would show it) joined to the lead as the screen
  likes.
- `Screen::entered` each time the nav brings a screen up, the digit of the
  screen already shown included.
- `Cx::key()`: the key that led to a callback, so `back` tells `esc` from
  `q`.
- `Tui::through_keys(keys)`: keys that reach `on_key` and the screen while a
  confirm is open, the confirm staying open.
- `Log::follow(false)` keeps the log's place when lines arrive while it is
  at the bottom.
- `Pick::full(true)` fills the screen's area (a heading, a blank row, the
  list) instead of a box, and `Pick::columns` sets its columns.
- `clock::span_hours` never goes to days; `clock::Units` takes a year unit
  for ages of 365 days and more, words for a span under a second ("under
  1s") and for an age under a minute ("just now"). `clock::next_span` now
  steps by the minute past an hour, so it suits both spans.

## 0.3.4 (2026-10-07)

- The screen that ran a command hears it: `Screen::heard(id,
  command::Heard, cx)` gets every line with its stream and, on the stream
  the app follows, its parsed progress (`Heard::Line { stream, text,
  progress }`), then the exit once (`Heard::Exit` with `command::Exit`:
  `Code`, `Signal`, `Stopped`, or `Error` when it never started).
  `Exit::state` maps an exit to done, failed or stopped. The runner's posts
  reach the loop as `Wake::Command`.
- An app sets what the band shows for a command by posting its own activity
  for that id: from then on the band shows the app's label, status,
  progress, state and start, and the command's lines only fill the detail.
  The shell keeps the process, the stop and the busy count, and ends the
  activity by its exit if the app hasn't. The runner's posts used to
  overwrite the app's own.
- `Cx::activities` replaces the app's own activities and keeps the
  commands'; it used to wipe them.
- `Cx::run_from` keeps the start of an unfinished activity already on the
  board under that id, such as one shown waiting first.
- `Cx::forget` takes a running command's activity off the board for good;
  its next lines used to bring it back.
- A line that isn't UTF-8 is read with its bad bytes replaced; it used to
  end the reading, so a chatty command could stall on a full pipe.

## 0.3.3 (2026-10-07)

- `Cx::run_from(id, label, Command, command::Stream)` follows the
  `--progress json` lines on stdout, stderr or both, for tools that print
  them on stderr; every other line still goes into the activity's detail.
  `Cx::run` stays on stdout.
- `Cx::stop(id)` ends a running command: `SIGTERM` to its process group on
  Unix, then `SIGKILL` after `command::GRACE` (2 s) if it is still running
  (a kill at once elsewhere). The activity shows as stopped at once, keeps
  its status and detail, and later lines no longer change it.
- `activity::State::Stopped`, a fourth end state beside done and failed,
  drawn with a faint `■`. A finished activity now ignores progress lines
  that come after its end.
- On Unix a command started with `Cx::run` runs in a process group of its
  own, so a stop reaches what it started.

## 0.3.2 (2026-10-07)

- The activity screen never takes a screen's index: it comes after every
  screen the app adds, whether `Tui::activities` is called before, between
  or after the `Tui::screen` calls. Called first, it used to take index 0
  and shift every screen by one for `go`, `waker`, `event`, `names` and
  capture's frame names.

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
