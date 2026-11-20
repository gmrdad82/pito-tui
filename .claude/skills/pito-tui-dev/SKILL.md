---
name: pito-tui-dev
description: "Develop pito-tui itself: the shared Rust TUI shell crate (header, footer, hourglass, list, keys, select, terminal guard, paced loop, headless dumps). Use for any change in this repo: src/, tests/, examples/, render/, docs/pages/, README.md, CHANGELOG.md, Cargo.toml, a pito-tui release or tag, or a Dispatch piece for pito-tui. Triggers: pito-tui repo, pito_tui, bin/gate, bin/gate --fast, the quick check, bin/docs-bundle, bin/link-skill, bin/header-shots, bin/marks, src/shell.rs, src/term.rs, src/screen.rs, tests/<module>, the *_allocation tests, .public-allow, docs-vX.Y.Z.tar.gz, a new pito-tui version. Excludes using pito-tui to build an app: that is the shipped user skill, skills/pito-tui in this repo, installed as pito-tui."
---

# pito-tui-dev

pito-tui is a Rust library (MIT, edition 2024, Rust 1.88+) that gives PITO MD terminal apps their shell: a header, footer, hourglass and list of its own, a panic-safe terminal guard, a 120 fps paced loop, key routing, an activity band and headless dumps. Apps pin it by git tag; it is not on crates.io. This page is the how-to for sessions that change this repo. The rules are in `AGENTS.md` (`CLAUDE.md` imports it): read both and `prompts/RESUME.md` before working, and do not expect this page to restate them.

`AGENTS.md`, `CLAUDE.md`, `prompts/`, `dev/`, `pito-app.toml` and `.dev/` are git-ignored and live in the main checkout only, so a fresh worktree has none of them: `git worktree list` names the main checkout, read them there.

## Layout

- `src/`: the shell (`shell.rs` the `Tui` and loop, `term.rs` the terminal guard and `restore`, `screen.rs` `Screen` and `Cx`, `layer.rs` modals, takeovers and toasts, `activity.rs`, `log.rs`, `pick.rs`, `fold.rs`, `palette.rs`, `words.rs`, `select.rs`, and `dump.rs`, `headless.rs`, `capture.rs` for headless runs) and one folder per module: `header/`, `footer/`, `hourglass/`, `list/`, `keys/`.
- `tests/`: one target per top-level file (`shell.rs`, `quit.rs`, `layers.rs`, ...) and per module folder (`header/`, `footer/`, `hourglass/`, `keys/`, `list/`, `select/`, each with a `main.rs`). `tests/<module>_allocation.rs` proves a steady frame allocates nothing. Unit tests sit inline under `#[cfg(test)]`.
- `examples/`: `demo.rs` is the fuller starting point and wires the headless flags; each module has its `<module>_demo`, `_gallery`, `_bench` or `_fit` examples. Only `list_demo` is declared a test target in `Cargo.toml`.
- `render/`: `prec` recipes, `render/terminal.toml` with `render/terminal/*.tape` for the demo and `render/<module>/render/` for each module.
- `docs/pages/<module>/` and the shared pages are this crate's section of docs.pitomd.com, `docs/images/<module>/` their media. `docs/changelogs/` keeps the changelogs of the crates folded in; new entries go in `CHANGELOG.md`.
- `skills/pito-tui/` is the shipped user skill for app builders; `.claude/skills/pito-tui-dev/` is this one.

## Topic guides

Read the matching in-repo doc before changing the area:

- [`headless.md`](../../../skills/pito-tui/headless.md) and [`docs/pages/guides/headless.md`](../../../docs/pages/guides/headless.md): `Tui::shot`, `--dump`, `--keys`, `--bench`, capture and compare
- [`docs/pages/guides/terminal.md`](../../../docs/pages/guides/terminal.md): the terminal guard and loop
- [`docs/pages/guides/screens.md`](../../../docs/pages/guides/screens.md): `Screen`, `Cx`, loads and ticks
- [`docs/pages/reference/api.md`](../../../docs/pages/reference/api.md): the public API
- [`README.md`](../../../README.md): every feature, and the crate's rustdoc front page
- [`CHANGELOG.md`](../../../CHANGELOG.md): how an entry reads

## Critical rules

- **Rules live in `AGENTS.md`.** Its Speed section outranks any brief or plan: while working run the quick check, not the full suite, no GPU or heavy tests, no re-rendered goldens or media unless the maintainer asks, fix forward.
- **Never open the TUI on a desktop session.** Draw it headless (`--dump WxH`, `Tui::shot`, `--bench`). Anything that can crash runs under `prlimit --core=1:1 -- timeout <secs>`.
- **Generic building blocks only, no words of the shell's own, drawing reads the loop's moment.** An app's gap enters already rewritten as a general primitive (`AGENTS.md`).
- **A steady frame allocates nothing.** The `tests/*_allocation.rs` fail on a new allocation in a draw path.
- **No comments in code.** Keep dependencies few: `Cargo.toml` lists them all, `rustix` and `signal-hook` only for Unix. The crate builds on Linux, macOS and Windows, so Unix-only code stays behind `cfg(unix)`.
- **This repo is public.** No private repo, internal path, person, price, key or plan in any tracked file, docs pages included (`bin/docs-bundle` refuses `/home/`, `/Users/` and `C:\Users`). A pre-push guard checks the tree and the pushed commits for anything internal. `.public-allow` clears only a private-marker line (one line per accepted finding, with its reason), never an agent-file path; a push the guard refuses is reported to the maintainer, not pushed around with `--no-verify`.
- **Stage by name, never the ignored files above.** Commits are signed and end with the Co-Authored-By trailer. Never force-push, amend, rewrite history, stash, or checkout, reset or clean the maintainer's tree.
- **Other repos get a Dispatch plan**, never a write: apps built on pito-tui are other repos.

## Command discovery

| Command | What it does |
| --- | --- |
| `bin/gate --fast` | the quick check: `cargo fmt --check`, clippy with all features and `-D warnings`, then only the tests for what changed against local `main` (touched test targets and modules, doctests when `src/`, `README.md` or `docs/pages/` changed, everything when `Cargo.toml`, `Cargo.lock` or `bin/gate` changed, none when nothing testable changed) |
| `bin/gate` | the full check: fmt, clippy, every nextest test, doctests and `cargo build --release --example demo`; only when the maintainer asks, and macOS takes `--fast` only |
| `cargo fmt`, `cargo clippy --all-targets --all-features -- -D warnings` | format and lint what you changed |
| `cargo nextest run --all-features -E 'binary(quit)'` | one test target by name (`kind(lib)` for the inline unit tests); `cargo nextest --help` |
| `cargo test --all-features --doc` | the doctests: README, each module's `README.md` and the pages `src/` includes |
| `cargo run --example demo -- --dump 72x14 --keys "tab"` | one headless frame after keys; also `--ansi`, `--loading`, `--at MS`, `--bench N`, `--capture DIR`, `--compare DIR` |
| `bin/docs-bundle [vX.Y.Z]` | stages the pages, images, changelogs and rustdoc API, checks links, frontmatter and local paths, packs `tmp/docs-vX.Y.Z.tar.gz`; the tag must match `Cargo.toml`'s version |
| `bin/link-skill [--uninstall]` | links `skills/pito-tui` into the agent skill folders under `$HOME` (`bin/link-skill.ps1` on Windows); run only when asked |
| `bin/header-shots` | rebuilds the header's gifs and pngs with `prec`, `pcod` and `pcmp` (each `--help` is its source of truth); only when the maintainer asks for new media |
| `bin/marks` | rewrites `docs/images/pitomd-wordmark.svg` from the brand masters checkout named by `PITO_ASSETS` |
| `dispatch plan send <file>` | hands another repo's work to Dispatch; `dispatch --help` |

The scripts in `bin/` have no `--help`; read them. Each other tool's `--help` is the source of truth.

## How work goes

- Small changes go straight onto `main`. A Dispatch piece runs in its own git worktree on a `dispatch/<slug>` branch: commit there, run the quick check on the final commit, then merge the branch into `main` locally from the main checkout. Push only when the maintainer says so.
- Commit subjects say what changed for the user in a plain sentence (`git log` shows the voice); a release commit reads `pito-tui X.Y.Z: <what it brings>`.
- Before a run that builds in a fresh worktree (no `target/`, so every dependency compiles), glance at `free -h`.
- One next version holds everything planned; no intermediate releases. Semver as `CHANGELOG.md` states it: only a first-number change may break an app, the middle adds, the last fixes.

## Release

Outward-facing: only on the maintainer's word.

1. `CHANGELOG.md`: a `## vX.Y.Z — YYYY-MM-DD` section with `### Added` or `### Fixed`, read like the entries below it.
2. Bump `version` in `Cargo.toml` (building updates `Cargo.lock`) and the two `--tag vX.Y.Z` install lines in `README.md`; `docs/pages/install.md` takes `{{tag}}` and needs no edit.
3. Quick check, commit `pito-tui X.Y.Z: ...`, signed tag `vX.Y.Z` with the message `pito-tui X.Y.Z`.
4. A GitHub release named `pito-tui X.Y.Z` on that tag, its notes the changelog entry, with `bin/docs-bundle` output attached as `docs-vX.Y.Z.tar.gz`. Check the asset is there. `AGENTS.md` names the announce step that follows.

## Known pitfalls

- The quick check diffs against local `main` and counts untracked files that are not ignored: run it on the branch before the merge, after which it sees nothing changed.
- A `src/` file with no matching test target and no inline `#[cfg(test)]`, or any change to `src/lib.rs`, makes the quick check run every test.
- Rust fences are compiled as doctests in `README.md`, each module's `README.md`, `docs/pages/install.md` and `docs/pages/list/`; a line starting `# ` in a fence is hidden, and `bin/docs-bundle` strips it from the pages.
- A docs page needs `title`, `description` and `sidebar.order` frontmatter, links other pages by URL (`../loading/`), images by file, and other repos only as `https://docs.pitomd.com/<repo>/`.
- A change to what the code does updates its page in `docs/pages/` in the same commit.

## Decision framework

1. **A fix or feature in a module** → `src/<module>/`, its tests in `tests/<module>/` (plus the `_allocation` test if it draws), its page, `CHANGELOG.md`, then `bin/gate --fast`.
2. **An app asks for something** → rewrite it as a general container, slot, overlay, indicator, band, prompt or key route first; if it cannot be general, it does not belong here.
3. **The terminal guard or loop** (`src/term.rs`, `src/shell.rs`) → the terminal must always be given back, on quit, error, panic and `restore()` from any thread; cover it in `tests/shell.rs`, `tests/signals.rs`, `tests/quit.rs` and read the README's "A terminal that is always given back".
4. **Docs only** → edit the page; `bin/gate --fast` runs the doctests, `bin/docs-bundle` checks the links when pages or images moved.
5. **The shipped user skill** (`skills/pito-tui/`) → it rides the next version inside the docs bundle; keep it about using pito-tui, never about developing it.
6. **Media or goldens** → leave them unless the maintainer asks; then `bin/header-shots` for the header, the module's `render/` recipe for the rest.
7. **A bug that breaks users now** → a quick fix release, not more gates.
8. **Another repo needs something** → a Dispatch plan whose `project:` line names that repo.
