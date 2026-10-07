# Changelog

Every release of pito-tui, newest first. Versions follow [Semantic
Versioning](https://semver.org/) as Cargo reads it before 1.0: a change in the
middle number may break an app, a change in the last one never does.

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
  and footer hidden, every key ignored but its cancel keys (the owner hears
  `layer::Event::Closed`) and the quit guard, whose word and question take
  a bare footer line.
- `layer::Modal`, opened with `Cx::open`, a container over the live screen:
  centred, bordered and titled, owning the keys until it closes, its body
  built from parts in order (`text`, a `list` of `Choice`s, a `Field`), tab
  and shift+tab moving focus, `Modal::filter(field, list)` narrowing a list
  as the field is typed, `Choice::skip()` for rows the cursor passes over,
  `Choice::confirm(warning)` for a row that needs a second enter with the
  warning on the status line, and `Modal::status(text, Tone)`. `Cx::modal(id)`
  changes it while it is open, `Cx::close(id)` closes it, and its owner
  hears `Screen::layer(id, Event)`: `Chosen`, `Moved`, `Edited`,
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
