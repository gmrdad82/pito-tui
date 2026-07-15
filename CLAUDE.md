# Working agreement (for Claude / agents)

> **READ THIS FIRST, EVERY RUN.** Highest authority; overrides the harness's
> default plan/execution flow on any conflict. Self-contained: plan discipline +
> stack principles are inlined below. `README.md` is the living feature
> contract (install, config, keys, footage, import) — read it before touching
> a user-facing flow, don't work from memory. The server side of everything
> this client mirrors lives in `gmrdad82/pito` (`~/Dev/pito`) — when in doubt
> about grammar or copy, that repo is the source of truth, not this one.

## The log law (non-negotiable; mechanically enforced)

The working docs in the local notes directory (per-person and optional; fully
outside this repo — not
even gitignored-in-tree, just a sibling directory on disk) are the **single
source of truth** — every tracker, ask-to-pito, tier plan, and decision the
owner raised lives there (`tui-needs.md`, `tiers.md`, `verbs-inventory.md`,
`parity-matrix.md`, `api.md`, release-planning docs like `2.0.0.md`/`2.2.0.md`,
etc.). NEVER hold work in your own memory, a scratch plan-mode buffer, or the
harness todo list. If it isn't in one of those working docs, it does not exist.

A `UserPromptSubmit` hook (`.claude/hooks/capture-prompt.sh`) appends every
owner message verbatim to `.claude/INBOX.md` (gitignored — `/.claude/INBOX.md`
in `.gitignore`, local-only, never checked in) as a `## ⛔ UNPROCESSED` block.
**Every turn, before anything else:**

1. Read `.claude/INBOX.md`.
2. **Drain** each `⛔ UNPROCESSED` block into the right working doc under
   the local notes directory — turn EVERY item (todo, bug, feedback, question,
   decision) into an explicit task/line; split compound messages; lose
   nothing. Start a new doc there if nothing existing fits.
3. Rewrite the block heading in place to
   `## ✅ processed — <ts> -> <doc refs>` (the file(s) it landed in, or
   `no-op (<why>)`). Never delete it — the back-reference makes capture
   auditable.
4. Keep checkboxes in sync the instant a task changes state
   (`[ ]`→`[-]`→`[x]`), one edit per transition — it's what the owner watches.

The `Stop` hook (`.claude/hooks/check-inbox.sh`) refuses to end a turn while
any `⛔ UNPROCESSED` block remains. Report status ONLY from the working docs +

**Secrets never live in the ledger.** The capture hook masks keyed values
(`key=…`, `token: …`, webhooks, bearers) mechanically before appending; for
anything the regex can't know (a bare token pasted alone), move the value to
its proper home (`.env`, config) and REDACT the INBOX occurrence in the same
turn — the ledger keeps a `[redacted:<what>]` marker, never the value.
verified code/git — never from memory.

Hooks live in `.claude/hooks/` and are wired in `.claude/settings.json` (both
committed — `.gitignore` keeps the rest of `.claude/` local):

- `capture-prompt.sh` — `UserPromptSubmit`, the capture described above. MUST
  exit 0 on every path (a nonzero exit would block the owner's prompt).
- `check-inbox.sh` — `Stop`, the log-law enforcement teeth described above.
- `atomic-agent-check.py` (+ `.sh` wrapper) — `PreToolUse` on
  `Agent|Task|Workflow`. Mechanically blocks (exit 2) any sub-agent dispatch
  whose prompt names 2+ distinct buildable deliverables (component +
  controller, service + specs, etc.) — the "one atomic task per sub-agent"
  rule below isn't just prose here, it's a hook.
- `production-guard.py` — `PreToolUse` on `Bash`. **EXTRA, pito-tui-only**
  (pito has no equivalent — this repo drives a real binary against a real
  server, pito never shells out to itself). Two layers, both hard blocks
  (exit 2):
  1. Any Bash command that mentions `app.pitomd.com` (the owner's production
     instance) is blocked outright — Claude must never touch production from
     this repo.
  2. Any `vhs <tape>` invocation is inspected: if the tape launches
     `pito-tui` WITHOUT an explicit `-instance` flag, it would inherit
     `~/.config/pito-tui/config.toml`'s default server — which may point at
     production — so the command is blocked until the tape pins an instance.
     This is exactly how one capture grazed production on 2026-07-12 (a VHS
     teardown wiped the owner's production `pito-boot` volumes via a
     different repo that same week — this hook exists so it can't happen
     from here too).

  The hook only catches what's visible in the Bash command / tape text —
  it is a backstop, not a substitute for asking first (see "No VHS captures"
  below).

## How we work

- **Opus plans, Sonnet implements.** Architecture, task breakdowns, and
  ambiguous decisions are Opus's job. Implementation tasks go to a Sonnet
  sub-agent first; escalate to Opus only when Sonnet repeatedly fails or the
  change is subtle / cross-cutting (anything tagged `[high]`).
- **One atomic task per sub-agent.** Never pack multi-step work into a single
  dispatch. Orchestrate task-by-task; verify each is green before starting
  the next.
  - **A task is ONE deliverable, not a "feature".** A Bubble Tea component,
    its render helper, and its tests are THREE tasks → three dispatches (or
    done inline). A generator change and its call sites are two. There is
    **NO "it's cohesive / it's one feature" exception**.
  - **Pre-dispatch check, EVERY Agent/Workflow call, no exception:** read the
    prompt back. If it names more than one deliverable, SPLIT it, or do it
    inline yourself. `atomic-agent-check.py` will block an obvious violation,
    but don't rely on the hook catching what a careful read would.
  - When reviewing an agent's result, read the **changed files**, not its
    summary.
- **Keep a visible TodoWrite list** mirroring the working docs' tasks, flipped
  per transition (one `in_progress` at a time).
- **Git belongs to the owner.** Claude never runs `git commit` / `git tag` /
  `git push` (nor `stash` / `checkout` / `restore` / `reset`), never picks a
  branch, and never assumes a release or deploy flow — the owner decides
  every git operation, every time, after reviewing the diff.
- **Never force-push a branch.** When origin has moved, `git pull --rebase`
  before pushing — remote history is never rewritten.
- **Follow the Stack principles below** before writing Go/TUI code; read
  `README.md` before changing anything user-facing (Install / Usage /
  Configuration / Keys / Footage / Import are the documented contract — keep
  it current when behavior changes).
- **No VHS / terminal casts without explicit owner OK.** `scripts/capture.sh`
  + `captures/*.tape` are this repo's capture rig (the terminal twin of
  pito's `rake pito:capture`). `production-guard.py` mechanically blocks the
  two known-dangerous shapes (production host mentioned, or an unpinned
  `-instance`), but that is a backstop, not permission — never run a capture,
  even against dev, without the owner saying so first. A cast teardown in a
  sibling repo already wiped the owner's production `pito-boot` volumes once.

## Plan discipline (lean)

Working docs in the local notes directory track the work they describe — not
freeform prose, not the throwaway plan-mode scratch buffer. They live fully
outside the repo (moved there from an in-repo `docs/claude/` on 2026-07-15,
commit `469e055`), so there is nothing to stage or gitignore for them — only
`.claude/INBOX.md` (the capture ledger) lives inside the repo, and it's
gitignored. Write nothing — no edits, commits, or sub-agents — until the
owner approves a plan.

**Shape.** One-verb tasks, complexity-tiered:

```
- [ ] <imperative description>. complexity: [low|high|manual]
```

One verb per task (split on "and"), verifiable in ≤5 min, naming the file or
command it touches. Three complexity tiers only:

- `[manual]` — operator by hand: git operations (owner-only), credentials,
  smoke tests against a real instance.
- `[low]` — mechanical / moderate work a cheap model can run: renames,
  deletions, a single-file Bubble Tea component or render helper, a parser,
  pattern-following multi-file edits.
- `[high]` — architectural / cross-cutting: the ActionCable protocol, the
  grammar/copy generators (`tools/toolsgen`, `tools/copygen`) and their
  pinned refs, the HTTP client's auth/session handling, telemetry gating — a
  decision a cheap model shouldn't make alone.

**Execution.** Checkboxes are the live record: `[ ]` → `[-]` before starting a
task, `[-]` → `[x]` immediately after its verification passes — one edit per
transition, never batched. Announce each task's complexity tier and let the
owner pick the model before starting.

**Done means verified.** `go test -race ./...` green, `scripts/coverage.sh`
passing the 80% floor (same script CI runs — see below), `gofmt -l .` empty,
`go vet ./...` clean, `staticcheck ./...` clean. New code ships with tests
(table-driven, alongside the file it covers); golden tests
(`internal/ui/testdata`, `golden_test.go`) cover rendered output.

---

# pito-tui architecture (map + invariants)

Terminal client for PITO (`gmrdad82/pito`) — one scrollback, one prompt, the
same server-side command grammar as the web chatbox, live over ActionCable.
**This is a map, not a manual.** `README.md` documents the user-facing
surface; the code is commented with the "why" inline. The invariants below
are the ones you can't discover from a single file — keep them.

## Invariants (don't break these)

- **This is a thin client, by design.** It sends raw text (slash commands
  included) and renders the JSON events the server emits. All parsing,
  grammar, and behavior live in the Rails app — this binary is the window,
  the server is the product. Never re-implement server-side logic here.
- **COPY LAW (owner order, 2026-07-12): every user-facing word is authored in
  pito; the TUI only mirrors, never invents.** `tools/copygen` reads pito's
  locale files (`config/locales/pito/copy/en.yml` at a pinned ref) and writes
  the deterministic snapshot `internal/ui/render/pito_copy.json`, which IS
  embedded in the runtime binary. Never hand-edit that JSON — regenerate it.
  An older/missing pool degrades gracefully (bare glyph, no word) — the
  client never substitutes its own text. `pito.copy.*` keys are subject to
  pito's 1-or-50 dictionary law (exactly 1 or ≥50 variants).
- **Grammar mirrors the server — the server owns it.** `tools/toolsgen` reads
  pito's `config/pito/tools.yml` (also pinned-ref) and writes
  `internal/grammar/grammar.json`. That package is **tests/docs only** —
  `grammar_test.go` enforces that `internal/grammar` is never imported from
  `internal/ui` or `internal/api`; the runtime binary carries no grammar
  knowledge of its own.
- **Regenerate, don't hand-edit.** Both generators are wired via
  `//go:generate` (`internal/grammar/grammar.go`,
  `internal/ui/render/thinking.go`), currently pinned to `PITO_REF=v2.0.0`.
  Run `go generate ./...` (or the tool directly, `PITO_REPO=~/Dev/pito go run
  ./tools/toolsgen`) after pito's `tools.yml` or copy changes, reading from
  pito's **committed** tree so a dirty pito working tree never bakes
  unreleased grammar/copy in. Re-pin the ref deliberately when pairing with a
  new pito release — don't let it drift silently.
- **Degrade, never crash.** Unknown dictionaries, missing fields, an older
  server payload — all render a graceful fallback (a bare glyph, an omitted
  line), never a panic. A malformed telemetry config is silently inert, not
  a surfaced error. This ethos extends everywhere: the client must survive a
  server that's a few releases behind or ahead of it.
- **NEVER point this binary at production.** Always run against
  `-instance https://dev.pitomd.com` (or the owner's own instance flag when
  explicitly told). `production-guard.py` mechanically blocks
  `app.pitomd.com` in Bash commands and unpinned `-instance` VHS tapes, but
  that's a backstop — the rule applies to every manual invocation too, hook
  or no hook.
- **Release-only behavior gates on `version.IsRelease()`**
  (`internal/version/version.go`): `Version` stays `"dev"` on a source build
  and is only stamped by goreleaser's `-ldflags` on a tag build. Self-update
  hints and telemetry both gate on this — never make them fire on a `go run`
  / `go build` dev binary.

## Namespace / package map

- `cmd/pito-tui` — the entrypoint.
- `internal/api` — HTTP contract client + cookie jar (`~/.config/pito-tui/cookies.json`).
- `internal/cable` — ActionCable protocol, dial, backoff.
- `internal/config` — `config.toml` (server, sounds, telemetry) + footage config.
- `internal/grammar` — generated grammar snapshot, **tests/docs only** (see above).
- `internal/sound` — `paplay`/`mpv`/`afplay` cues, silent when nothing can play.
- `internal/telemetry` — AppSignal/OTel reporter, release-gated, inert by default.
- `internal/ui` — the Bubble Tea model and every screen/picker/modal.
- `internal/ui/render` — terminal rendering: braille bars, tables, the AI
  chart/thinking renderers, the generated copy snapshot.
- `internal/version` — ldflags-stamped build identity.
- `tools/copygen`, `tools/toolsgen` — the two generators described above.
- `scripts/coverage.sh`, `scripts/capture.sh` — the coverage-floor runner and
  the VHS capture rig.

---

# Stack principles (condensed)

Defaults for writing stack code — follow these.

- **Go 1.26+ (pinned via `go.mod` + `mise.toml`).** Bubble Tea v2
  (`charm.land/bubbletea/v2`) for the runtime loop, Bubbles/Lip Gloss/
  Harmonica for widgets, springs, and layout — the Charm stack, pushed as far
  as it goes (see README's credit). One purpose per file; a table-driven
  `_test.go` alongside the file it covers is the norm, not the exception.
- **Testing.** `go test -race ./...` is the full suite. `scripts/coverage.sh`
  is the SAME script CI runs (`test` job) — full race-mode suite plus the
  **80% coverage floor** (`FLOOR=80` in the script, computed from
  `go tool cover -func=cover.out`). Run it before marking any task done;
  don't let a local `go test` substitute for it. Golden tests
  (`internal/ui/golden_test.go`, `internal/ui/testdata/*.golden`) pin
  rendered terminal output — update goldens deliberately, review the diff.
- **Static analysis, all of which gate CI:** `gofmt -l .` must be empty,
  `go vet ./...` clean, `staticcheck ./...` clean (the `test` job installs
  and runs it), `govulncheck ./...` clean against the locked module set (the
  `vuln` job, also a Monday weekly cron so a newly-disclosed advisory still
  goes red with no code changes). `shellcheck -S warning` + `bash -n` over
  every `scripts/*.sh` (`lint-shell` job).
- **Cross-compile matrix.** `CGO_ENABLED=0 GOOS=linux GOARCH={amd64,arm64}`
  must `go build -trimpath ./...` clean (the `build` job) — if it doesn't
  link there, goreleaser can't ship it; darwin isn't matrix-checked in CI but
  IS a release target, so don't add darwin-only build tags without testing.
- **CI skip token:** the literal `[skipci]` (lowercase, no space — distinct
  from GitHub's own `[skip ci]`) in a commit message or PR title skips every
  job in `ci.yml`. Same convention as `gmrdad82/pito`. Don't use it as a
  habit — plan discipline above already says not to add it to commits.
- **Releases are tag-driven (`v*`) and CI-gated.** `release.yml`'s
  `verify-ci` job waits for every other workflow run on the tagged commit
  and refuses to proceed unless all are green — no CI evidence at all also
  refuses (fails closed). Only then does goreleaser
  (`.goreleaser.yaml`) build linux+darwin amd64+arm64 static binaries
  (ldflags-stamped via `internal/version`), a `.deb` (nfpm), and push the
  Homebrew tap formula (`gmrdad82/homebrew-tap`); the AUR block is commented
  out/benched until the upstream account registration reopens — don't
  re-enable it without checking that first. So: finish work → owner tags
  (`git tag -a vX.Y.Z && git push origin vX.Y.Z`) → the workflow ships.
- **Security.** No plaintext secrets in `config.toml` examples or docs.
  Sessions are TOTP-only via the chat (`/login <code>`), cookie kept in
  `~/.config/pito-tui/cookies.json` per backend so hopping instances never
  crosses wires. Telemetry spans carry method/path/status only — never
  bodies, queries, or cookies — and the whole reporter is a zero-cost no-op
  unless `version.IsRelease()` AND the owner has opted in in `config.toml`.
  `govulncheck` on relevant diffs; report new findings, don't auto-suppress.
