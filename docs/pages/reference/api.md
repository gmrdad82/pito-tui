---
title: The API at a glance
description: Every builder, trait method, context call and type pito-tui offers, on one page, with the defaults a Tui starts with.
sidebar:
  order: 200
---

The whole API in one sketch, for finding a name fast. The
[full API docs](../../api/pito_tui/index.html) list every item with its exact
signature, and the [guides](../../guides/screens/) show each part in use.

```text
Tui::new(name, version, accent: Color) -> Tui<E>          // E: the app's worker answers
  .palette(Palette) .words(Words) .hints([Hint<'static>]) .min_size(w, h)
  .group(impl Into<Label>) .group_keys(&[footer::Key]) .screen(impl Into<Label>, impl Screen<E>)
  .show(Fn(Item) -> bool) .head(Head) .focus_keys(next, previous) .title(text) .title_spans([(text, Style)])
  .layout(Layout) .role(name, Style) .monochrome(bool) .hourglass_timing(delay, least) .version_fit(bool)
  .nav_keys(NavKeys) .help(Option<Help>) .quit(Mode) .quit_window(Duration)
  .quit_keys(&[footer::Key]) .quit_sticky(bool) .quit_tone(footer::Tone) .quit_hints(bool)
  .stop_keys(&[footer::Key]) .eager(bool) .keep_facts(bool)
  .modes(Modes) .activities(Activities) .overlay(header::Section, &[footer::Key], impl Screen<E>)
  .app(impl Screen<E>) .gate(impl Screen<E>, shut: bool)
  .band(impl Band) .top_band(impl Band) .lit_again(bool) .through_keys(&[footer::Key])
  .header_look(fn(Header) -> Header) .footer_look(fn(Footer, width: u16) -> Footer)
  .on_key(FnMut(KeyEvent, &mut Cx<E>) -> bool) .after_draw(FnMut(Duration))
  run() -> io::Result<()>, run_in(&mut Term)
  key(KeyEvent) -> Flow, paste(&str) -> Flow, mouse(MouseEvent) -> Flow, event(screen, E) -> Flow, handle(Wake<E>) -> Flow
  loaded(screen, generation, E) -> bool, landed(screen, generation, E) -> Option<Flow>  // None: a late or unknown load
  changed(source), go(screen) -> bool, start()
  pump(), settle() -> Flow, advance(Duration) -> Flow, draw(&mut Frame), busy(), current(), names()
  find::<T>(), find_mut::<T>(), sender(), waker(screen), set_palette(..), set_words(..), set_layout(..)
  frame(w, h) -> Buffer, shot(w, h, &[KeyEvent], settle) -> Buffer, bench(w, h, frames) -> Vec<Bench>
pub enum Flow { Stay, Quit }

pub trait Screen<E>: Any {                                 // every method but draw has a default
  fn draw(&mut self, &mut Frame, Rect, &Palette);
  fn moment(&mut self, now: Instant);                      // the loop's moment, before each draw
  fn phase(&self) -> Phase;                                // Ready | Loading(label) | Trouble(lines)
  fn load(&mut self) -> Option<Job<E>>;  fn loaded(&mut self, E, &mut Cx<E>);
  fn sources(&self) -> &[&str];  fn cancel(&mut self);  fn start(&mut self, &mut Cx<E>);
  fn entered(&mut self, &mut Cx<E>);                       // each time the nav brings it up
  fn key(&mut self, KeyEvent, &mut Cx<E>);  fn paste(&mut self, &str, &mut Cx<E>);
  fn mouse(&mut self, MouseEvent, Rect, &mut Cx<E>);  fn event(&mut self, E, &mut Cx<E>);
  fn answer(&mut self, yes: bool, &mut Cx<E>);  fn picked(&mut self, Option<usize>, &mut Cx<E>);
  fn heard(&mut self, id: u64, command::Heard, &mut Cx<E>);  // a command it ran: each line, then the exit
  fn layer(&mut self, id: u64, layer::Event, &mut Cx<E>);  // a modal or takeover it opened
  fn claim(&mut self, KeyEvent, &mut Cx<E>) -> bool;       // the app slot's app-wide keys, first
  fn back(&mut self, &mut Cx<E>);
  fn after_tabs(&self) -> Vec<(String, Style)>;            // after the tabs, with Tui::head
  fn caption(&self) -> Option<Vec<(String, Style)>>;      // Row::Caption's label
  fn hints(&self) -> Vec<Hint>;  fn facts(&self) -> Vec<(String, Style)>;
  fn status(&self) -> Option<(String, Style)>;
  fn status_parts(&self, room: u16) -> Vec<(String, Style)>;  // default: status() as one part
  fn badge(&self) -> &[Span];  // after the status and a space; Span = (Cow<'static, str>, Style)
  fn lead(&self, room: u16) -> Vec<(String, Style)>;      // beside the help word
  fn left(&self, room: u16, help: Option<&str>) -> Option<Vec<(String, Style)>>;  // the whole left side
  fn notice(&self) -> Option<Notice>;  fn legend(&self) -> Option<&str>;
  fn typing(&self) -> bool;
  fn input(&self) -> Option<InputBar>;  fn crumb(&self) -> Option<String>;
  fn selected(&self) -> usize;  fn busy(&self) -> usize;
  fn animating(&self) -> bool;  fn deadline(&self) -> Option<Instant>;
  fn tick(&mut self, &mut Cx<E>);                          // once when the moment reaches a deadline
}
pub type Job<E> = Box<dyn FnOnce() -> E + Send>;
pub trait Band {                                           // the app's own band, Tui::band
  fn height(&self, room: u16, now: Instant) -> u16;
  fn draw(&mut self, &mut Frame, Rect, &Palette, now: Instant);
  fn deadline(&self, now: Instant) -> Option<Instant>;     // default None
}

Cx<'_, E>: screen(), now(), key() -> Option<KeyEvent>, detach(FnOnce() -> E), waker() -> Waker<E>,
  say(text, footer::Tone), hush(), confirm(question), confirm_with(question, Confirm),
  copy(text) -> bool, go(screen), quit(), reload(), changed(source),
  palette(Palette), words(Words), hints(Vec<Hint<'static>>),
  activity(Activity), activities([Activity]), forget(id), band(bool),
  run(id, label, process::Command), run_from(id, label, Command, command::Stream), stop(id),
  pick(Pick), focus() -> Focus, typing() -> bool, epoch(), title(text), title_spans([(text, Style)]),
  layout(Layout), role(Option<name>), show(Fn(Item) -> bool), go_group(n), toast(Toast),
  open(Modal), alert(id, title, [Line]), modal(id) -> Option<&mut Modal>, close(id),
  takeover(Takeover), end_takeover(), gate(shut: bool)

Head::new([Row]) .numbers(Numbers) .lone(bool)            // Head::default(): Title, Groups, Sections, Facts, Crumbs
pub enum Row { Title, Groups, Caption, Sections, Facts, Crumbs, Blank }
pub enum Numbers { Across, InGroup, Off };  pub enum Focus { Content, Groups, Sections, Slot }
pub enum Item { Group(n), Tab(screen) }
Label::new(text) | Label::spans([(text, Style)])  .short(text) .short_spans([(text, Style)])   // From &str, String, Section, Group
layer::Modal::new(id, title) | Modal::alert(id, title, [Line])
  .text([Line]) .list([Choice]) .list_rows([Choice], rows) .field(Field) .filter(field, list)
  .status(text, Tone) .width(cells) .hints([Hint]) .cancel_keys(..) .close_keys(..) .focus(part)
  id(), focused(), set_status(..), set_text(part, ..), set_choices(part, ..), choice(part), value(part), input_mut(part)
layer::Choice::new(text) | Choice::spans([(String, Style)])  .skip() .confirm(warning)
layer::Field::new(label) .placeholder(..) .input(Input)
pub enum layer::Event { Chosen { part, choice }, Moved { part, choice }, Edited { part }, Submitted { part }, Closed }
layer::Takeover::new(id, title) .title_spans(..) .label(text) .cancel(&[footer::Key])
layer::Toast::new(text) .tone(Tone) .at(Spot) .lasting(Duration)
Layout::new() .top .bottom .side .pad .head_gap .gap (cells) .small(Spot)
Spot::new(Edge, Edge), Spot::TOP_LEFT .. BOTTOM_RIGHT, MIDDLE, THIRD; place(area, w, h) -> Rect
pub enum Edge { Start, Third, Middle, End };  pub enum Tone { Ink, Strong, Muted, Accent, Good, Warn, Bad }

Activity::new(id: u64, label, started: Instant)            // every field pub, and a builder each
  .state(activity::State) .progress(f64 or None) .estimate(bool) .ended(Instant) .status(text)
  .detail([Line]) .shared(log::Lines)
pub enum activity::State { Running, Waiting, Done, Failed, Stopped }  // activity::LINGER: 3 s once finished
Activities::new(header::Section)                           // the activity screen's name and words
  .keys(&[footer::Key]) .band(bool) .hints([Hint]) .detail_hints([Hint]) .log(Log) .empty(text)
  .more(Fn(usize) -> String) .elapsed(Fn(Duration) -> String) .facts(Fn(&[Activity]) -> Vec<(String, Style)>)
command::read(line) -> Option<Progress { state, stage, fraction, message, ok }>   // one --progress json line, v 1
pub enum command::Stream { Stdout, Stderr, Both }          // where Cx::run_from follows the lines; command::GRACE: 2 s
pub enum command::Heard { Line { stream, text, progress: Option<Progress> }, Exit(command::Exit) }
pub enum command::Exit { Code(i32), Signal(i32), Stopped, Error(text) }   // .state() -> activity::State

Pick::new(title, [option]) | Pick::rows(title, [list::Row])  .selected(index) .hints([Hint]) .keys(list::Keys) .cancel_keys(..)
  .columns([list::Column]) .full(bool)
Log::new(label) .placeholder(..) .hint(..) .keys(list::Keys) .search_keys(..) .hit_keys(next, previous) .copy_keys(page, all) .follow(bool)
  set_lines(log::Lines), lines(), len(), top(), hits(), hit(), query(), typing(), bar() -> Option<InputBar>,
  key(KeyEvent) -> log::Turn, paste(&str) -> log::Turn, draw(frame, area, &Palette)
pub enum log::Turn { Pass, Taken, Copy(text) };  pub type log::Lines = Arc<Vec<Line<'static>>>
clock::Units::new(second, minute, hour, day) .year(..) .under(..) .now(..)   // the app's unit words
clock::span(Duration, &Units) -> "1m 2s", span_hours(..) -> "49h 3m", age(Duration, &Units) -> "3m",
  next_span(since, now), next_age(since, now)
progress::bar(fraction, cells) -> "⣿⣿⣀⣀", percent(fraction), share(fraction, estimate) -> "≈57%", spinner(Duration), SPIN

capture::Walk::new() .sizes([(w, h)]) .screens(bool) .script(Script::new(name, [KeyEvent]) .loading() .at(Duration))
capture::capture(dir, &Walk, Fn() -> Tui<E>) -> io::Result<frames>
capture::compare(dir, &Walk, Fn() -> Tui<E>) -> io::Result<Vec<Difference>>   // Cell { scenario, size, x, y, before, after } | Added | Removed
capture::compare_dirs(before dir, after dir) -> io::Result<Vec<Difference>>   // two captures, frame by frame
capture::Recorder::new(dir) -> io::Result<Recorder>        // frames from an app's own buffers, written as capture writes them
capture::Recorder::against(dir) -> io::Result<Recorder>    // the same frames compared with a capture instead
  .screen(index, name, &Buffer) .record(script name, &Buffer) -> io::Result<()>, frames(), finish() -> io::Result<Vec<Difference>>

Palette::new(accent: Color)                                // every token a pub field and a builder
  .base .ink .strong .muted .accent .selected .good .warn .bad .rule .line .glass .shimmer
  .title .inactive .focus .small (Style)  .bar(Option<list::Bar>) .count(Option<list::Count>)
  mono(), tone(Tone) -> Style, header() -> header::Styles, footer() -> footer::Styles,
  list() -> list::Styles, view(&mut List, &[Column]) -> ListView, hourglass(elapsed, label, hint) -> Hourglass
Words::new()                                               // every word empty until the app sets it
  .help .again .yes .no .choose .leave .too_small .waiting (text)  .busy(Fn(usize) -> String)

Term::enter(Modes) -> io::Result<Term>                     // restored on drop and on a UI-thread panic
restore() -> bool                                          // any thread, once: true if it gave the terminal back
  terminal(), modes(), draw(FnOnce(&mut Frame)), mouse(bool)
Modes::new()  .alternate(true) .paste(true) .mouse(false) .focus(false)
Pace, FRAME (8.333 ms), wait_until(now, dirty, &Pace, deadlines)
pub enum Wake<E> { Input(Event), Lost(io::Error), Event(screen, E), Loaded(screen, generation, E), Activity(Activity),
  Command(screen, command::Report),                        // a command's lines and exit, from the shell's runner
  Stamped(epoch, screen, E),                               // a detach answer or a shell-made waker's event
  Signal(number) }                                         // SIGHUP, SIGTERM or SIGINT while the loop runs: it quits
Waker<E>: new(sender, screen), send(E) -> bool, screen();  listen(sender)   // the input thread

Filter::new(label) .placeholder(..) .hint(..) .keys(list::Keys) .start_keys(..) .clear_keys(..) .input(Input)
  sift(rows, keeps, line), key(KeyEvent) -> Turn, paste(&str) -> Turn, bar() -> Option<InputBar>,
  view(&columns, &Palette) -> ListView, typing(), query(), active(), shown(), current(), list(), list_mut()
pub enum Turn { Pass, Taken, Filtered, Open(index) }
matches(text, query) -> bool                               // every word of the query, any case
Folds::new([id]) .keys(one: &[footer::Key], all: &[footer::Key]) .still(bool) .lasting(Duration) .gap(rows)
  .marks(open, shut) .lit(bool)                            // "▾" and "▸", a 180 ms eased fold, no keys by default
  len(), is_empty(), id(index), index(id), is_shut(index), cursor(), point(index), set_still(bool)
  toggle(index, now), toggle_all(now), set(index, shut, now), set_all(shut, now), key(KeyEvent, now) -> bool
  state() -> FoldState, restore(&FoldState), moment(now), animating(), scroll(top), total(), top_of(index),
  title_at(column, row) -> Option<index>, placed() -> &[Placed],
  draw(frame, area, &Palette, &[Fold], FnMut(&mut Buffer, index, Rect))
Fold { title: &[Span], summary: &[Span], rows }            // the summary shows on a collapsed title rule
FoldState { sections: Vec<(id, collapsed)> };  Placed { title: Rect, body: Rect, rows, folding }

dump::size("120x34"), dump::keys("tab / foo enter ctrl+c") -> Result<Vec<KeyEvent>, word>,
dump::text(&Buffer), dump::ansi(&Buffer);  Bench { screen, frames, average, worst }
text::clean, cells, split, clip, fit, wrap, hard_wrap;  message(frame, area, text);  message_at(.., Spot)
copy(text), base64(bytes)
TOP, SIDE, PAD, GAP                                        // Layout::new()'s spacing, in cells
```

Until the app says otherwise, a `Tui` wants at least 40 × 12 cells, uses
`NavKeys::HEY` (`tab`, `[ ]`, digits, `esc` and `q` back), shows the hints
with `?` to hide them, quits on a double `ctrl+c` within 2 s and asks first
while any screen is busy (`Mode::Ask`), stops a load on `esc`, reads a screen
the first time it is shown (`eager(true)` reads them all at start), hides the
facts row while a screen is drilled in (`keep_facts(true)` keeps it), has no
activity band or screen until the app gives it `Activities` (the band then
shows unless `band(false)`) or its own `Band`, lights the quit word in the
accent over an input bar or a confirm (`lit_again`), lets no key past an open
confirm (`through_keys`), draws pito-header's header (no `head`), shows an
hourglass at once and drops it at once (`hourglass_timing`), shows the name
and version wherever pito-footer fits them (`version_fit`), has no app slot,
gate, takeover, modal, toast or top band until the app makes one, and turns
on raw mode, the alternate screen and bracketed paste only.

`Phase`, `Turn`, `Wake`, `Words`, `Palette`, `Tone`, `Layout`, `Spot`, `Edge`,
`Head`, `Row`, `Numbers`, `Focus`, `Item`, `layer::Event`, `Modes`, `Bench`, `Activity`,
`Activities`, `activity::State`, `Pick`, `log::Turn`, `clock::Units`,
`command::Progress`, `command::Stream`, `command::Heard` (and its `Line`), `command::Exit`,
`capture::Walk`, `capture::Script`, `capture::Look` and
`capture::Difference` are
`#[non_exhaustive]`: match the enums with a wildcard arm and build the structs
with `new()` and their builders, so a later release can add to them in a minor
version.
