# pito-tui

[![CI](https://github.com/gmrdad82/pito-tui/actions/workflows/ci.yml/badge.svg)](https://github.com/gmrdad82/pito-tui/actions/workflows/ci.yml)

The app shell for pito terminal apps, as one ratatui 0.30 crate. An app gives
it a name, a version and an accent colour, and plugs in its screens; the shell
does everything around them: the terminal, the loop, the keys, the header, the
footer, the hourglass while a screen loads, and headless dumps for tests and
reviews. It brings [pito-header], [pito-footer], [pito-hourglass] and
[pito-list] with it and re-exports them, so an app pins one crate.

It has no words of its own: every label, hint and message on screen comes from
the app, in any language.

[pito-header]: https://github.com/gmrdad82/pito-header
[pito-footer]: https://github.com/gmrdad82/pito-footer
[pito-hourglass]: https://github.com/gmrdad82/pito-hourglass
[pito-list]: https://github.com/gmrdad82/pito-list

## Install

```toml
pito-tui = { git = "https://github.com/gmrdad82/pito-tui", tag = "v0.1.0" }
```

That one line brings ratatui 0.30, crossterm 0.29, pito-header v0.2.0,
pito-footer v0.4.0, pito-hourglass v0.1.3 and pito-list v0.6.1, reachable as
`pito_tui::ratatui`, `pito_tui::crossterm`, `pito_tui::header`,
`pito_tui::footer`, `pito_tui::hourglass` and `pito_tui::list`.

Run the demo to see it (`ctrl+c` twice quits):

```sh
cargo run --example demo
cargo run --example demo -- --dump 100x24 --keys "tab down enter"
```

## What it does

- **Screens, plugged in.** Each screen is a type that implements `Screen`:
  it draws its area, takes its keys, and says what it shows in the header
  (facts, a status, a breadcrumb) and the footer (hints, an input bar). The
  shell lays them out as numbered tabs in groups (pito-header), switches
  between them with the app's nav keys, and starts a screen the first time
  it is shown.
- **Loads that can't land late.** A screen hands the shell its read
  (`load`), the shell runs it on a worker and delivers the answer
  (`loaded`) only if it is still the newest one: each read carries a
  generation, so an answer that comes back after the user stopped waiting,
  or after a newer read, is dropped. While a screen loads, the shell shows
  the hourglass (pito-hourglass) with the screen's own words; a stop key
  (`esc` unless the app picks others) stops waiting. `Cx::changed(source)`
  marks every screen that reads that source stale, and they read again.
- **A 120 fps paced loop that never blocks the UI thread.** An input thread
  and the app's workers wake one channel; the loop drains every wake in a
  batch, redraws only when something changed, no more often than every
  8.3 ms, and otherwise sleeps until the next wake or deadline (the quit
  notice running out, a screen's own deadline). Every frame is a
  synchronized update, so the terminal never shows half of one. Idle, it
  draws nothing.
- **Keys in a fixed order:** a key release is ignored; then the quit guard
  (a double `ctrl+c`, asking first while work runs, with the app's words);
  then an open confirm; then a screen that is typing into an input; then
  the help toggle (`?`); then the nav keys; then the stop key while a
  screen loads; then the app's own keys (`on_key`); then the screen.
- **A terminal that is always given back.** `Term` turns on raw mode, the
  alternate screen and bracketed paste (mouse capture and focus events only
  when the app asks), remembers each one it turned on, and turns exactly
  those off again on quit, on an error, and on a panic on the UI thread,
  before the panic message prints. A panic on a worker thread leaves the
  running app alone. The app's own panic hook still runs after it.
- **A palette from one colour, or a whole one.** `Palette::new(accent)`
  derives every style the header, footer, list and hourglass draw with;
  `Palette` also takes each token by hand (base, ink, muted, accent,
  selected, good, bad, rule, line, glass, shimmer). `Cx::palette` swaps it
  while the app runs, and the hourglass follows.
- **The footer, assembled:** the screen's hints and the app's, the notice
  line (the quit guard's, or what a screen said with `Cx::say`), the
  confirm bar, the input bar, and the app's name and version at the right
  end of the row of keys (pito-footer's version slot). `?` hides the hints;
  the header then shows the app's help word.
- **The header, assembled:** the app's name on the title rule, the groups
  and screens as tabs, the screen's facts and status, and its breadcrumb
  kept in step: when a screen opens something (`crumb`), the sections row
  becomes the breadcrumb, and the nav's back key (`esc`, `q`) calls the
  screen's `back`.
- **A filter over a list.** `Filter` holds a pito-list `List`, a pito-footer
  `Input` and the rows that match; `/` starts typing, `enter` keeps the
  filter, `esc` clears it, and the keys are the app's to change.
- **The rest of the glue:** a too-small guard with the app's message, OSC 52
  copy (`Cx::copy`, up to 48 KiB), a y/n confirm whose answer comes back to
  the screen that asked, an optional callback after every draw with the time
  it took, and width-aware text helpers (`text::clip`, `fit`, `wrap`).
- **Headless.** `Tui::shot` draws the app at any size after a list of keys,
  `dump::text` and `dump::ansi` print the result, `dump::keys` reads a
  `--keys` string and `Tui::bench` times every screen. The app wires them
  to its own flags.

## Use

```rust,no_run
use pito_tui::crossterm::event::KeyEvent;
use pito_tui::footer::Hint;
use pito_tui::header::Section;
use pito_tui::ratatui::{Frame, layout::Rect, style::Color, text::Line};
use pito_tui::{Cx, Palette, Screen, Tui, Words, message};

struct Hello {
    presses: usize,
}

impl Screen<()> for Hello {
    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        let text = format!("{} keys pressed", self.presses);
        message(frame, area, Line::styled(text, palette.ink));
    }

    fn key(&mut self, _key: KeyEvent, _cx: &mut Cx<'_, ()>) {
        self.presses += 1;
    }

    fn hints(&self) -> Vec<Hint<'_>> {
        vec![Hint::new("any key", "count")]
    }
}

fn main() -> std::io::Result<()> {
    let words = Words::new()
        .help("? help")
        .again("ctrl+c again to quit")
        .yes("Yes")
        .no("No")
        .too_small("Make the window bigger");
    Tui::new("hello", env!("CARGO_PKG_VERSION"), Color::Rgb(0x5b, 0x8c, 0xff))
        .words(words)
        .hints([Hint::new("ctrl+c", "twice quit").pinned()])
        .screen(Section::new("Hello"), Hello { presses: 0 })
        .run()
}
```

`examples/demo.rs` is the fuller starting point: two screens, a read with
the hourglass, a filtered list, a drill-in with a breadcrumb and copy, an
app-wide key that swaps the palette, and the headless flags.

## The API

```text
Tui::new(name, version, accent: Color) -> Tui<E>          // E: the app's worker answers
  .palette(Palette) .words(Words) .hints([Hint<'static>]) .min_size(w, h)
  .group(header::Group) .screen(header::Section, impl Screen<E>)   // screens join the last group
  .nav_keys(NavKeys) .help(Option<Help>) .quit(Mode) .quit_window(Duration)
  .quit_keys(&[footer::Key]) .stop_keys(&[footer::Key]) .eager(bool) .modes(Modes)
  .header_look(fn(Header) -> Header) .footer_look(fn(Footer) -> Footer)
  .on_key(FnMut(KeyEvent, &mut Cx<E>) -> bool) .after_draw(FnMut(Duration))
  run() -> io::Result<()>, run_in(&mut Term)
  key(KeyEvent) -> Flow, paste(&str), mouse(MouseEvent), event(screen, E), handle(Wake<E>) -> Flow
  loaded(screen, generation, E) -> bool, changed(source), go(screen) -> bool, start()
  pump(), settle(), draw(&mut Frame), busy(), current(), names()
  find::<T>(), find_mut::<T>(), sender(), waker(screen), set_palette(..), set_words(..)
  frame(w, h) -> Buffer, shot(w, h, &[KeyEvent], settle) -> Buffer, bench(w, h, frames) -> Vec<Bench>
pub enum Flow { Stay, Quit }

pub trait Screen<E>: Any {                                 // every method but draw has a default
  fn draw(&mut self, &mut Frame, Rect, &Palette);
  fn phase(&self) -> Phase;                                // Ready | Loading(label) | Trouble(lines)
  fn load(&mut self) -> Option<Job<E>>;  fn loaded(&mut self, E, &mut Cx<E>);
  fn sources(&self) -> &[&str];  fn cancel(&mut self);  fn start(&mut self, &mut Cx<E>);
  fn key(&mut self, KeyEvent, &mut Cx<E>);  fn paste(&mut self, &str, &mut Cx<E>);
  fn mouse(&mut self, MouseEvent, Rect, &mut Cx<E>);  fn event(&mut self, E, &mut Cx<E>);
  fn answer(&mut self, yes: bool, &mut Cx<E>);  fn back(&mut self, &mut Cx<E>);
  fn hints(&self) -> Vec<Hint>;  fn facts(&self) -> Vec<(String, Style)>;
  fn status(&self) -> Option<(String, Style)>;  fn typing(&self) -> bool;
  fn input(&self) -> Option<InputBar>;  fn crumb(&self) -> Option<String>;
  fn selected(&self) -> usize;  fn busy(&self) -> usize;
  fn animating(&self) -> bool;  fn deadline(&self) -> Option<Instant>;
}
pub type Job<E> = Box<dyn FnOnce() -> E + Send>;

Cx<'_, E>: screen(), now(), detach(FnOnce() -> E), waker() -> Waker<E>,
  say(text, footer::Tone), hush(), confirm(question), confirm_with(question, Confirm),
  copy(text) -> bool, go(screen), quit(), reload(), changed(source),
  palette(Palette), words(Words), hints(Vec<Hint<'static>>)

Palette::new(accent: Color)                                // every token a pub field and a builder
  .base .ink .muted .accent .selected .good .bad .rule .line .glass .shimmer (Style)
  header() -> header::Styles, footer() -> footer::Styles, list() -> list::Styles,
  hourglass(elapsed, label, hint) -> Hourglass
Words::new()                                               // every word empty until the app sets it
  .help .again .yes .no .choose .too_small .waiting (text)  .busy(Fn(usize) -> String)

Term::enter(Modes) -> io::Result<Term>                     // restored on drop and on a UI-thread panic
  terminal(), modes(), draw(FnOnce(&mut Frame)), mouse(bool)
Modes::new()  .alternate(true) .paste(true) .mouse(false) .focus(false)
Pace, FRAME (8.333 ms), wait_until(now, dirty, &Pace, deadlines)
pub enum Wake<E> { Input(Event), Lost(io::Error), Event(screen, E), Loaded(screen, generation, E) }
Waker<E>: new(sender, screen), send(E) -> bool, screen();  listen(sender)   // the input thread

Filter::new(label) .placeholder(..) .hint(..) .keys(list::Keys) .start_keys(..) .clear_keys(..) .input(Input)
  sift(rows, keeps, line), key(KeyEvent) -> Turn, paste(&str) -> Turn, bar() -> Option<InputBar>,
  view(&columns, &Palette) -> ListView, typing(), query(), active(), shown(), current(), list(), list_mut()
pub enum Turn { Pass, Taken, Filtered, Open(index) }
matches(text, query) -> bool                               // every word of the query, any case

dump::size("120x34"), dump::keys("tab / foo enter ctrl+c") -> Result<Vec<KeyEvent>, word>,
dump::text(&Buffer), dump::ansi(&Buffer);  Bench { screen, frames, average, worst }
text::clean, cells, split, clip, fit, wrap;  message(frame, area, text);  copy(text), base64(bytes)
TOP, SIDE, PAD, GAP                                        // the shell's spacing, in cells
```

Until the app says otherwise, a `Tui` wants at least 40 × 12 cells, uses
`NavKeys::HEY` (`tab`, `[ ]`, digits, `esc` and `q` back), shows the hints
with `?` to hide them, quits on a double `ctrl+c` within 2 s and asks first
while any screen is busy (`Mode::Ask`), stops a load on `esc`, reads a screen
the first time it is shown (`eager(true)` reads them all at start), and turns
on raw mode, the alternate screen and bracketed paste only.

`Phase`, `Turn`, `Wake`, `Words`, `Palette`, `Modes` and `Bench` are
`#[non_exhaustive]`: match the enums with a wildcard arm and build the structs
with `new()` and their builders, so a later release can add to them in a minor
version.

## Adopting pito-tui

This section is for an app that already draws with pito-header, pito-footer,
pito-hourglass and pito-list directly, and keeps its own loop. Moving to the
shell deletes that glue; the app's screens and data stay as they are.

### What the app keeps

- Its screens' content: what each one draws, its data, its rows and
  columns, its detail views. pito-list and pito-footer's `Input` stay the
  app's to use inside a screen, through `pito_tui::list` and
  `pito_tui::footer`.
- Its words, every one, in any language: `Words` for the shell's few
  places (the help word, the quit notice and question, Yes and No, the
  confirm's hint line, the too-small message, the hint under the
  hourglass), the global hints, and each screen's hints, facts, labels and
  messages. An empty word is simply not drawn.
- Its keys beyond the shell's: anything a screen's `key` takes, and
  app-wide actions through `on_key`. The shell's own keys are the app's to
  change too: `nav_keys` (pito-header's `NavKeys`, `NavKeys::NONE`
  included), `quit_keys`, `stop_keys`, `help(None)`, and the filter's
  `start_keys` and `clear_keys`. Mouse capture stays off unless the app
  turns it on with `Modes`.
- Its workers and their messages: the app's own enum is the `E` in
  `Tui<E>`, and its threads post through a `Waker<E>`.
- Its command line: the app parses its own flags and calls the shell's
  headless helpers.
- Its crash reporting: a panic hook the app installs before `run` still
  runs, after the shell has given the terminal back.

### What the shell takes over

| The app's own code today | With pito-tui |
|---|---|
| `ratatui::init` and `ratatui::restore`, bracketed-paste and mouse toggles, panic hooks that restore the terminal | `Tui::run` (or `Term::enter` for a loop of its own) |
| An input thread feeding a channel, an enum with an input arm, `recv_timeout` | `listen` and `Wake<E>`, inside `run` |
| A frame pacer, a dirty flag, deadlines, draining a batch of wakes | `run`: `Pace` at 120 fps, synchronized updates, `deadline` and `animating` per screen |
| Code that hands the app's asks to threads and routes their answers back | `Screen::load` (tracked by generation) and `Cx::detach` (one-off work) |
| Key routing: release, quit guard, confirm, input, help, nav | `Tui::key`, in that order |
| Quit guard wiring: busy count, wording, tick, deadline, notice, bar | `Screen::busy`, `Words::again` and `Words::busy`, `quit`, `quit_window` |
| A confirm gathered from several places | `Cx::confirm` and `Screen::answer` |
| Header assembly: title, help word, facts, a breadcrumb kept in step | `Screen::facts`, `status`, `crumb`, `selected` and `back` |
| Footer assembly: hints, notice, confirm, input, version, height | `Screen::hints`, `input`, `Cx::say`; the version slot is wired |
| A tone module mapped to four crates' `Styles` | `Palette`, and `Cx::palette` at runtime |
| A loading wrapper around the hourglass | `Phase::Loading(label)` |
| The too-small guard, a centred message, OSC 52, text helpers | `min_size` and `Words::too_small`, `message`, `Cx::copy`, `text` |
| Dump size and key parsing, text and ANSI output, a bench loop | `dump::*`, `Tui::shot`, `Tui::bench` |

### Moving over, step by step

1. **One pin.** Replace the four `pito-*` lines in `Cargo.toml` with the
   pito-tui line above, and import through `pito_tui::header`,
   `pito_tui::footer`, `pito_tui::hourglass` and `pito_tui::list`. Keep
   ratatui and crossterm through `pito_tui::ratatui` and
   `pito_tui::crossterm`, or pin the same versions (0.30 and 0.29).
2. **The palette.** Build `Palette::new(accent)`, then set the tokens the
   app's tone module had (`good`, `bad`, `muted`, `base` for a painted
   background). Hand it to `Tui::palette`.
3. **The words.** Move every shell-facing string into one `Words` and the
   app-wide hints (`? help`, `ctrl+c twice quit`) into `Tui::hints`.
4. **One screen at a time.** Make each section or page a type that
   implements `Screen<E>`: its draw function becomes `draw`, its hints and
   facts become `hints` and `facts`, its loader becomes `load` (return the
   work as a closure) and `loaded` (take the answer from the app's enum, no
   downcast), its "stop waiting" becomes `cancel`, its detail view becomes
   `crumb` plus `back`, and its search box becomes a `Filter` or its own
   `Input` returned from `input` while `typing` is true. A password field is
   an `Input::new().masked(true)`; a bracketed paste arrives in `paste`.
5. **Groups.** `Tui::group(Group::new("Work"))` before the screens that
   belong to it; screens before any group join one unnamed group, and a
   single group draws no groups row.
6. **Workers.** One-off work goes through `Cx::detach(move || E::Done(..))`
   and comes back to the same screen's `event`. A long-lived watcher takes
   `cx.waker()` (or `tui.waker(index)` before `run`) and posts with
   `send`. A watcher that sees a source change sends an event whose screen
   calls `cx.changed("source")`.
7. **The loop goes.** Replace the app's `run` with `Tui::run()`; delete
   the pacer, the terminal setup and restore, the restoring panic hooks,
   the input thread and the key routing. The app's crash hook stays where
   it is, installed before `run`.
8. **Headless flags.** Wire `--dump WxH` to `dump::size` and `Tui::shot`,
   `--keys` to `dump::keys`, `--ansi` to `dump::ansi`, `--loading` to
   `shot(.., settle: false)` and `--bench N` to `Tui::bench`; `main` in
   `examples/demo.rs` does exactly this. A dump with fixtures builds the
   screens from fixture data before `shot`.
9. **Check.** Dump every screen at the sizes the app cares about and
   compare them with the old dumps; the header, footer and spacing should
   match, since the shell draws them with the same crates and the same
   layout (a one-cell side margin for the header and footer, two cells for
   the content, one row above the footer).

### An app that already has a section trait

Most of it maps one to one: a section's name becomes the `header::Section`
passed to `Tui::screen`; its data sources, phase, cancel and retry become
`sources`, `phase`, `cancel` and `Cx::reload`; a loader that returns a job
and a delivery to downcast becomes `load` returning a closure and `loaded`
taking the app's own enum, with the generation kept by the shell; its notice
becomes `Cx::say`; its pending confirm becomes `Cx::confirm` with `answer`;
its asks become `Cx::detach`. A job that needs a connection or a client opens
or clones its own inside the closure, since it runs on a worker.

An overlay that sits over every section becomes a screen of its own (in its
own group if it should read as one), or stays the app's drawing inside a
screen with `crumb` naming where the user is.

## Development

`bin/gate` runs `cargo fmt --check`, `cargo clippy --all-targets
--all-features -- -D warnings`, the tests (`cargo nextest run`, then this
README's example as a doctest) and the release build of the demo.
`bin/gate --fast` leaves the release build out, and CI runs it on every push
and pull request to main. The tests are a safety net for what would hurt if it
broke: the frame pacing, the terminal restore, the key order, the load
generations and the dump formats. Each release is listed in
[CHANGELOG.md](CHANGELOG.md).

## Contributing

Issues and pull requests are welcome. Please read the
[code of conduct](CODE_OF_CONDUCT.md) first. A change keeps `bin/gate` green
with no warnings and leaves every word, style and key to the app. Report a
security issue privately, as [SECURITY.md](SECURITY.md) says, not in a public
issue.

## Licence

The code is MIT licensed: see [LICENSE](LICENSE), by Catalin Ilinca. The PITO
name and its logos, and the names and logos of every pito app and game, are ©
Catalin Ilinca, all rights reserved, and are not covered by the MIT licence.
The look of the crates it brings is in the style of HEY's terminal UI; see
[NOTICE.md](NOTICE.md).
