# Changelog

All notable changes to pito-tui are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/); from 1.0.0 onward the
project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **The client itself** — a Bubble Tea terminal client for PITO: a
  conversation picker fed by `/resume.json` (recent/older, plus "start a
  new conversation" whose uuid arrives with the first send), a scrollback
  of turn blocks with per-kind renderers for every `Event::KINDS` entry
  (unknown kinds degrade to a payload dump — never a crash), one prompt
  that sends raw text so the server grammar stays the only grammar, and
  vim-style scrolling that only kicks in while the prompt is empty.
- **No default server, by design** — pito is self-hosted, so the
  client ships pointing at nothing and no install is ever suggested.
  Unconfigured, it stops gracefully with the way in: `pito-tui config
  server=<url>` (persists, bare hosts get `https://`) or `--instance
  <url>` for a single run. `pito-tui config` shows the current server;
  `pito-tui version` says what you're running. Cookies and sound caches
  are keyed per backend, so switching instances never crosses sessions
  or cues.
- **Login is a chat message** — the server grammar owns authentication:
  when unauthenticated, the TUI opens with a banner and you send
  `/login <code>` exactly like the web chatbox. The reply mints the
  session cookie into `~/.config/pito-tui/cookies.json` (0600, atomic
  writes), where it survives restarts until the 24h idle timeout — at
  which point the banner simply comes back.
- **Live updates over ActionCable** — a minimal in-repo cable client
  (no external cable libraries) subscribing `TuiChannel` on
  `pito:json:conversation:<uuid>`: `event.append` extends its turn,
  `event.replace` rewrites in place. Drops reconnect with jittered
  backoff, and every reconnect re-syncs the scrollback over HTTP with an
  ID-idempotent merge — the cable has no replay, and nothing goes
  missing.
- **Sound cues** — send/receive/notify mp3s fetched once from the
  instance into `~/.cache/pito-tui/`, played through `paplay` or `mpv`,
  demoting silently on failure (down to silence when neither can play).
  `sounds = false` or `--sounds=off` turns them off entirely.
- **CI, pito-style** — gofmt/vet/staticcheck, the full suite under the
  race detector with an enforced 80% coverage floor (`scripts/coverage.sh`,
  identical locally and in CI), an amd64+arm64 cross-compile check,
  shellcheck, govulncheck with a weekly bit-rot canary, the `[skipci]`
  guard, and the Deadpan Butler reporting to Slack — one green heartbeat
  per push, failure-only everywhere else.
- **Gated releases** — tag-only (`v*`): a ported verify-ci gate refuses
  to ship unless every workflow on the tagged commit is green (fail
  closed, 30-minute deadline), then goreleaser publishes static
  linux+darwin amd64+arm64 binaries with checksums, a `.deb`, and a
  Homebrew formula (`brew install gmrdad82/tap/pito-tui`) that serves
  macOS and Linux alike. macOS plays sound cues through the built-in
  `afplay`. The `pito-tui-bin` AUR package follows in a patch release —
  the AUR had account registration closed upstream at ship time.
- **Dependabot** — weekly grouped gomod + github-actions PRs, same
  grouping intent as pito; vulnerability alerts and automated security
  fixes enabled.
- **Structured payload rendering** — lists (`ls vids/games/channels`)
  as aligned tables with headings, accent `#refs`/`@handles`, and dim
  usage footers; `/help` as titled key/value sections; detail cards
  (`show vid/game/channel`) with aligned label/value pairs and icon
  labels recovered from the web's SVGs. Ported web components: comet
  post-command spinner, timestamp prefixes, meta lines
  (`#handle · @channel`), tips, and a `?`-toggled key help. Living
  trackers: `docs/claude/verbs.md` (verb coverage) and
  `docs/claude/tiers.md` (component port tiers).
- **Terminal images (kitty graphics)** — on kitty/ghostty/WezTerm
  (detected from the environment, dynamic, never required) detail-card
  thumbnails pin to the top-right of the screen through the session
  cookie; plain terminals keep the text-only cards.
- **Capture rig** — `scripts/capture.sh`, the terminal twin of pito's
  `rake pito:capture` (vhs-driven); scenarios in `captures/*.tape`,
  artifacts in `tmp/captures/<name>/`.
- **Analytics charts (v1)** — `analyze channel` renders real charts
  from the structured payload: per-metric gradient sparklines with
  totals, previous-window deltas and target pacing, plus the
  likes-vs-dislikes heart — the Butler's per-metric captions ride
  above each, shimmer included. The web's own body drawing yields to
  the terminal-native one.
- **Mutation-round fixes** — detail-card label/value grids survive
  mixed cells (the avatar `<img>` no longer collapses the channel card
  into a run-on paragraph); avatar cells show a ◉ marker pending true
  inline images; I18n-only server errors render a humanized hint
  instead of a JSON dump.
- **Shimmer and gradients** — on truecolor terminals the exact words the
  web shimmers (its `pito-subject-shimmer` spans) get a multi-stop
  gradient sweep while the message is fresh, then settle; the pending
  comet rides the same ramp. The gradient engine (with the web's
  green→red meter ramp and a `Bar()` primitive) is the shared base for
  the upcoming context meter, score bars, and analytics charts.
  256-color terminals get static accents instead.
