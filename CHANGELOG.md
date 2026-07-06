# Changelog

All notable changes to pito-tui are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/); from 1.0.0 onward the
project follows [Semantic Versioning](https://semver.org/).

## [1.1.0] — 2026-07-06

Every screen from the revisit, the server's new eyes, and the shine.

### Added

- **Show cards, rebuilt** — `show game/vid/channel` renders as a real
  card instead of flattened html soup: the details as a zebra kv table
  (the left column's Stats and Shinies counters folded in as rows; long
  values like tags and channel descriptions wrap inside their own
  column, stripes intact), the description on its own under a hairline,
  and — for games — the web's Score and Time-to-beat bars, 1:1: the
  bracketed `=` fill painted by the exact CSS ramp (hard red edges
  through green), the `|` tick at the score with the value as an
  invert chip beside it, the TTB heat gradient read from the payload's
  own per-game stop positions, footage chip, colored main/extras/
  completionist ticks, hour values under their ticks, and the legend
  row. Labels keep the server's own padding so the two bars' brackets
  align, exactly like the web.
- **A reusable score bar** — the bar engine (positioned-stop gradients,
  ticks, invert chips) and `ScoreBar` are standalone primitives, ready
  for any future surface that carries a 0-100 rating.
- **No images, still** — cover art, banners, and thumbnails are skipped
  by construction (no placeholders, no ascii stand-ins); platform icons
  contribute their alt text ("PlayStation Switch Xbox Steam") because
  that is data, not imagery.
- **The game's enhanced segments** — `show game … with similar,
  channels, linked-videos` (and the segment verbs) render terminal-native:
  *similar* as recommendation rows — `#id Title` beside a label-less
  score bar, brackets aligned down the strip; *channels* as the coverage
  block (one solid colored share bar per channel, redrawn from the
  distribution legend's own data, caption below) plus the fit-score
  roster (handle from the avatar's alt, score bar per row);
  *linked-videos* was already a `Video::List` payload and rides the
  shared list viewer — table, zebra, and the full mutate reply surface
  (with/without/sort) included. The async channels fill waits quietly
  ("mapping the territory…") instead of leaking the web's braille
  canvas, and fills in on replace/resync.
- **At-a-glance, everywhere** — `show channel/vid/game … with
  at-a-glance` (and the glance verb) renders the web's panel
  terminal-native: five metric cells, each a 2-row braille sparkline —
  the web's own BrailleAreaChart rows lifted verbatim from the payload,
  42 columns, identical curve by construction — over a legend that
  gives the curve its scalar meaning ("Views 7.7K", "Subs +11 / -28",
  "Likes 81 Likes / 11 Dislikes"). Cells sit two-up on wide terminals
  and stack on narrow ones; the async fill waits with "crunching the
  numbers…" and the nudge line rides below. The dotted-paper background
  grid stays on the web.
- **The vid's linked game** — `show vid … with linked-game` renders the
  game card as the same zebra kv the show cards wear (no cover art, no
  placeholders), reply handle carrying the full game_detail surface.
- **The glance contract, live-specced** — `TestGlanceContract` waits
  out the AnalyticsFillJob for all three nouns, asserts the ready body
  carries every fragment the renderer lifts, checks the linked-game
  card, and fires the `#handle analyze` reply.
- **The segments reply contract, live-specced** — `TestGameSegmentsContract`
  drives the linked-videos list's with/sort/without cycle, the similar
  strip's `#handle show #id` reply, the channels fill reaching ready,
  and pito's handle-sweep rule (an appending reply retires every prior
  live hashtag — mutate replies retire nothing), all against a real
  instance.
- **The show reply contract, live-specced** — `TestShowReplyContract`
  verifies each card arrives with every fragment the renderer parses,
  that append replies (`#handle analyze`, `#handle shinies`) land new
  messages below the card, and that an invalid action answers with the
  error path — against a real instance.

- **The glossy pass** — the web's charm, terminal-native: the pito-blue
  shimmer band now sweeps every chart fill (score bars, TTB, coverage
  bars, glance sparklines — the same `pito-bar-shimmer` motion the web
  runs, riding the existing animation loop); sparklines sit on the
  dotted graph paper (⠂ dots, ⣀ baseline) exactly like the web's
  canvas; product copy renders its `backtick` command spans in the
  accent so footers and nudges read like the web's inline code; and the
  status bar wears the host in the pito brand gradient.

- **Braille, everywhere charts live** — the analyze deep-dive now draws
  its curves with a dot-exact Go port of pito's own BrailleAreaChart
  (2-row braille on the dotted graph paper, target-aware ceiling, the
  scalar legend below) — the old solid block runes are gone. Everything
  chart-like in the terminal is braille now; the score and TTB bars keep
  their `=` fills because that is literally what the web renders.
- **Charts and bars grow in** — the web's pito-bar-reveal, ported:
  freshly-arrived fills (score bars, TTB, coverage, braille curves)
  ease in over ~600ms — bars left-to-right, curves bottom-up — then
  settle; backfilled scrollback renders instantly.
- **Live confirmations breathe** — an unresolved confirmation's warn
  border pulses gently until a reply resolves it.
- **The picker, dressed up** — brand-gradient title, a hairline under
  the header, and the selection riding a full-width candy-plum stripe.

- **Shimmer, staggered and smoothed** — every animated element (marked
  words, bar fills, coverage bars, braille curves) now carries its own
  phase offset from a stable seed, so neighbors never pulse in sync —
  the terminal cousin of the web's shimmer stagger buckets. Shinies and
  platform chips scatter on the web's exact 20 discrete buckets. The
  sweep band widened into a soft cosine-falloff gradient instead of a
  hard-edged stripe, the whole TUI sweeps at the web's 130° angle
  (multi-row charts lean the band across rows), and the animation loop
  doubled to 25fps with a slightly quicker (~2.7s) cycle — motion, not
  ticking.

- **The context meter** — the web's thin gradient bar, above the
  prompt: server-computed fill (the server is the source of truth — the
  TUI never counts), drawn as a braille hairline wearing the green→red
  meter ramp with the shimmer sweep, conversation name at the left when
  named, the percent counter at the right. Updates live every turn via
  the new `conversation.update` cable message.
- **The mini status, completed** — the status line now leads with who
  you are (`@handle`, from the server) and trails with the unread
  notification count when there is one — both patched live by the same
  per-turn cable message.

- **Shift+↑/↓ scroll the conversation** — web parity, and stronger:
  they work even while you're mid-sentence in the prompt (arrow keys
  with shift never collide with typing). The vim keys (j/k, ctrl-d/u,
  g/G) keep working on an empty prompt as before.

- **Gold coins and platform chips** — two web details the flattener
  used to drop, now data again (owner call): a game's price wears its
  Mario-style coins (gold ● per coin from the payload's own count, the
  gold ★ for FREE, an honest — when unpriced), and platforms render as
  brand chips — white text on PlayStation blue, Switch red, Xbox green,
  Steam navy — with a glassy highlight sweeping each chip on its own
  stagger. Inside lipgloss tables the chips stay plain short labels
  (PS/Switch/Xbox/Steam), which also fixes the silently-empty Platform
  column in lists.

- **Shinies, the loot log** — pito's achievement redesign (G127),
  terminal-native: badges are material pills — each of the eleven
  materials (wood through opal, the silver/gold/diamond awards) wears
  its exact web palette as a gradient body with ink text, edge caps,
  and a gleam sweeping on its own stagger; iridescent materials throw
  more light. `shinies channel|vid|game` and the `#handle shinies`
  reply render the full lanes: one rail per metric — reached ticks lit
  in their step's material, the NEXT threshold breathing, the rest dim,
  awards squared — with the "at X · next: Y (Material)" legend and the
  obtained badges flowing beneath. Compact faces trim with an ellipsis,
  unlock dates stay web-only, unknown materials degrade to a neutral
  pill. `ShinyBadge` is reusable anywhere badge-shaped content appears.
- **Stats and Shinies, their own table** — the show cards' Stats and
  Shinies rows moved out of the details zebra into a dedicated block
  above it, a blank row between the two tables (owner call).
- **The channel's games, structured** — `games channel @handle` (and
  the games segment) renders from the payload's new `games` rows
  (tui-needs.md item 5): the standard list table — #id, title, vids —
  intro kept, the web's cover grid ignored. Gated on field presence.
- **Segment renames tracked** — pito renamed game `linked-videos` →
  `videos` and vid `linked-game` → `game`; live specs and docs updated
  (payload shapes unchanged).

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
