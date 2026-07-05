# Changelog

All notable changes to pito-tui are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/); from 1.0.0 onward the
project follows [Semantic Versioning](https://semver.org/).

## [1.0.0] — 2026-07-05

First release — fresh meat.

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
- **Capture rig** — `scripts/capture.sh`, the terminal twin of pito's
  `rake pito:capture` (vhs-driven); scenarios in `captures/*.tape`,
  artifacts in `tmp/captures/<name>/`.
- **No images, by decision** — the terminal renders none: payload
  images are ignored wholesale, avatar cells and their columns vanish
  from tables and card grids. All kitty graphics code (pinned
  placements, Ken Burns, unicode placeholders) is gone.
- **Replies that edit messages in place, visibly** — mutation replies
  (`#handle sort/with/without …`) now echo locally (the server creates
  no turn for them) and trigger a re-sync fetch, so the original list
  re-sorts before your eyes — headline included — whether or not a
  cable replace also arrives.
- **Lists, rebuilt on lipgloss/table** — one shared viewer for
  `ls channels/vids/games` and every reply that re-emits a list:
  horizontal rules only (top, header, bottom — no vertical borders,
  the rows breathe against the message bar), zebra rows, alignment
  driven by the server's own hints (strings left, numbers and dates
  right), per-cell ellipsis truncation on narrow terminals, headline
  above, a breath of air, usage copy below, reply handle intact.
  Zebra rows wear candy plum, not battleship gray.
- **Charts and shimmer, hardened** — charts/sparklines/hearts render
  through the same profile-managed styling as text (fixing the
  white-charts bug raw escape codes caused) and ride the shimmer
  sweep, which now runs indefinitely like the web at a terminal-tuned
  tempo (~3.2s, 12.5fps).
- **Suggestions palette** — the web's ctrl+k, inlined: every
  keystroke asks the server-side ontology (`POST /suggestions`) and a
  menu rises above the prompt — labels, descriptions, accent-bar
  selection, ↑/↓ or ctrl-n/p to move, tab to complete the current
  token, esc to dismiss. No verb list ships in the binary; the grammar
  stays server-owned, so the palette can never drift from verbs.yml.
- **Analytics charts (v1)** — `analyze channel` renders real charts
  from the structured payload: per-metric gradient sparklines with
  totals, previous-window deltas and target pacing, plus the
  likes-vs-dislikes heart — the Butler's per-metric captions ride
  above each, shimmer included. The web's own body drawing yields to
  the terminal-native one. All three levels (channel, vids, games)
  live-verified; the shimmer sweep now matches the web's 5s tempo.
- **Mutation-round fixes** — detail-card label/value grids survive
  mixed cells (the avatar `<img>` no longer collapses the channel card
  into a run-on paragraph); I18n-only server errors render a humanized
  hint instead of a JSON dump.
- **The list reply contract, live-specced** — a live test
  (`go test -tags live -run TestListReplyContract`) walks every
  `with`/`without` column kwarg per noun (straight from pito's
  `list_columns.rb`: likes for channels; platform, genre, developer,
  publisher, channels, footage, price, views, likes for games; channel,
  visibility, game, duration, views, likes, category for vids), both
  directions of every base sort key (handle/title for channels,
  id/title for games and vids), and the full requires_with sort model:
  every column that joins the sort vocabulary while visible is sorted
  descending then ascending and must lead with different values —
  platform (icons don't order) and category (not sortable) are the
  deliberate exceptions, and channels' default subs/views/vids counters
  sort without any `with` first. Found along the way: a headed column
  whose cells are all empty is data, not image residue — it renders now
  (`with platform` before any platform is set); the server sizes tables
  in pixels, so the TUI reports its width in them too, and wide layouts
  finally arrive. Found on the server: channel sort replies dropped both
  `selected_columns` and the current column set (counter sorts no-op,
  `with likes` vanished on sort) — reported for the Rails side and
  asserted strictly here, so the spec confirms the fix the moment it
  deploys.
- **Wide tables truncate, never wrap** — a table pushed past the
  terminal by extra columns now shrinks cell-by-cell with `…` (the
  chosen design) instead of wrapping row remainders onto stray
  zebra-painted stub lines; the width budget accounts for the message
  bar's own frame, verified at every width from 40 to 220 columns.
- **Shimmer and gradients** — on truecolor terminals the exact words the
  web shimmers (its `pito-subject-shimmer` spans) get a multi-stop
  gradient sweep while the message is fresh, then settle; the pending
  comet rides the same ramp. The gradient engine (with the web's
  green→red meter ramp and a `Bar()` primitive) is the shared base for
  the upcoming context meter, score bars, and analytics charts.
  256-color terminals get static accents instead.
