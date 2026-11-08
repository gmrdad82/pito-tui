# Screens, Cx and activities

Each screen implements `Screen<E>`, where `E` is the app's own message type (`()` for
none). Every method but `draw` has a default.

```rust
Tui::new("demo", VERSION, accent)
    .screen(Section::new("Home"), home)
    .screen(Section::new("What to do next").short("Next"), Next::new())
    .run()
```

- Tabs: `tab`/`shift+tab` move between groups, `]`/`[` between tabs, a digit goes to a
  tab, `esc` or `q` go back. `Tui::nav_keys(..)` changes them; `NavKeys::NONE` turns them off.
  `Tui::group(Group::new("Work"))` starts a group before its screens.
- Header and footer: `facts` (row under the tabs), `hints` (`Hint::new("↑↓", "scroll")`,
  higher `.rank(n)` dropped first, `.pinned()` stays), `status_parts(room)`, `lead(room)`,
  `badge()`, `notice`, `Cx::say(text, tone)`. `fact_spans()` and `hint_list()` are the
  borrowed forms that allocate nothing on a steady frame.
- Drill in: return the opened name from `crumb`; the nav's back key calls `back`.
- Loading: `load` returns the read as a closure, `loaded` takes the answer from the app's
  enum; the shell tracks the generation and drops late answers. The hourglass shows meanwhile.
  `Cx::detach(move || E::Done(..))` runs one-off work; `cx.waker()` serves long-lived watchers;
  `deadline` plus `tick` is a timer.
- Prompts: `Cx::confirm` with `Screen::answer`; `Cx::pick(Pick)` with `Screen::picked`.
- `Tui::head(Head::new([Row::Title, ...]))` builds the header as a stack of rows.
- `Tui::app(screen)` adds an app-wide screen that feeds the title rule and hears keys first in `claim`.
- Palette: `Palette::new(accent)` derives every token; `Tui::palette` hands it over;
  `Cx::palette` changes it at runtime. Words: one `Words` for the shell's few places.

## Activities

`Tui::activities(Activities::new(Section::new("Operations"), ...))` gives a band flush on
the footer and a screen over any screen. Whichever screen hears about work calls
`cx.activity(Activity::new(id, label, started)...)` each time it changes, with the same `id`;
`cx.activities(..)` replaces the list; `cx.forget(id)` drops one. `Cx::run(id, label, Command)`
runs a child command and follows its `--progress json` lines (`Cx::run_from` for stderr);
`Cx::stop(id)` stops it; `Screen::heard` gets each line and the exit.

Read the repository guides `docs/pages/guides/screens.md`, `loading.md`, `prompts.md`,
`activities.md`, `keys.md` and `palette-and-words.md` for the full detail.
