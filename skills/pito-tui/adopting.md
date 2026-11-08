# Adopting pito-tui in an app

## Link this skill

A git dependency lands in cargo's checkouts, where no agent looks. From a checkout of
pito-tui run `bin/link-skill`; it links `skills/pito-tui` into `~/.agents/skills/pito-tui`,
`~/.claude/skills/pito-tui`, `~/.codex/skills/pito-tui` and `~/.pi/agent/skills/pito-tui`.
The links point at that checkout, so re-run it from a newer one after bumping the tag.

## Pin

```
cargo add pito-tui --git https://github.com/gmrdad82/pito-tui --tag <tag>
```

Import the widgets through `pito_tui::header`, `footer`, `hourglass` and `list`, and
ratatui and crossterm through `pito_tui::ratatui` and `pito_tui::crossterm`.

## Moving an app that has its own loop

The app keeps its screens' content, its words, its keys beyond the shell's, its workers,
its command line and its crash hook (which calls `pito_tui::restore()` before ending the process).
The shell takes over terminal setup and restore, the input thread, the frame pacer, key
routing, header and footer assembly, loading, the quit guard, confirms and pickers.

1. Pin the one dependency; import through `pito_tui::`.
2. Build `Palette::new(accent)` and set the tokens the old tone module had.
3. Move every shell-facing string into `Words`; app-wide hints into `Tui::hints`.
4. One screen at a time: a type implementing `Screen<E>`, its draw becomes `draw`, its loader
   `load` and `loaded`, its detail view `crumb` plus `back`.
5. `Tui::group(..)` before the screens of a group.
6. Workers: `Cx::detach`, or `cx.waker()` for a watcher.
7. Running work: `Tui::activities` with `cx.activity(..)`.
8. Replace the app's `run` with `Tui::run()`; delete the pacer, terminal setup, restoring panic hooks, input thread, key routing.
9. Wire the headless flags (see [`headless.md`](headless.md)).
10. Before the move, record every state at the sizes the app cares about with
    `capture::Recorder`; after it, `capture::compare`; an empty list proves the switch. Then delete the captures.

Full table of what maps to what: `docs/pages/guides/adopting.md` in the repository.
