# Changelog

All notable changes to pito-tui are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/); from 1.0.0 onward the
project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed

- **Grammar snapshot re-pinned to pito v3.4.0** —
  `internal/grammar/grammar.json` regenerated from the paired release's
  `tools.yml`: `footage` is gone entirely (chat verb and its `game_detail`
  reply target both retired on the pito side — the `update game footage`
  form and the games list `footage` column are untouched), the reply-only
  `apply` tool arrives (`ai_message` target, `use`/`accept` aliases — the
  AI-answer command-staging fallback), and `schedule` (along with most
  other video reply tools) picks up a `video_search` reply target so
  scheduling from a search-results list works the same as from `list`.
  Tests/docs only — the runtime binary carries no grammar knowledge either
  way.

### Fixed

- **Thinking indicators no longer stack above their own echo** — a
  broadcast missed during a reconnect's subscribe-confirm gap (easy on a
  starved server) came back via the re-sync merge AFTER the turn's later
  events had already arrived live, so a turn's echo could slot in under
  its own thinking indicator and the spinner read as a second indicator
  stacked on the previous turn. Events now slot within their turn by the
  server's own `position` (already on both the cable and the backfill
  wire), so cable and re-sync deliveries always reassemble in server
  order; position-less events keep plain arrival order.

## [3.0.0] — 2026-07-16

### Added

- **ctrl+/ toggles notifications, web parity** — opens the exact overlay
  the typed `/notifications` command does (`ctrl+_` works too — the
  legacy alias terminals without kitty disambiguation deliver ctrl+/ as,
  0x1F); pressing it again while the overlay is open closes it, the same
  toggle shape ctrl+k already has. The status bar's unread chip now
  fronts the hint itself, reading `ctrl+/ N ⚑`, so the affordance is
  visible before you ever reach for it.
- **Jump straight to a conversation-search hit** — Shift+J at an empty
  prompt, right after a `search conversations for/like …` reply, opens a
  numbered overlay of its hits: digits 1-9, ↑/↓, enter jumps, esc closes.
  A hit already in the loaded transcript scrolls straight to its anchor
  and drops follow mode; a hit from another conversation shows as
  "(elsewhere)" and submits `/resume <uuid>` — the same command a click on
  the web's conversation-name cell types+submits — instead of jumping
  locally.
- **Four more ctrl+k entries** — `search games for` / `search games
  like` join the youtube section, `search conversations for` / `search
  conversations like` join conversations. Labels land with the next
  copygen re-pin against pito's palette locale; until then they fall
  back to their insert text, the same COPY LAW fallback every other
  ctrl+k label already has.
- **A live frame-rate chip** — a small "NN fps" readout pinned to the
  viewport's top-left corner, the same shape as pito web and pitomd
  (cross-repo parity). The rate is measured, not simulated: a
  self-rescheduling ~100ms tick keeps the chip breathing at a ~10fps
  floor while idle (Bubble Tea only repaints on Update, so a genuinely
  idle terminal would otherwise read as frozen at 0), and that same loop
  reports whatever true rate a busier moment — typing, scrolling, a
  streaming reply — actually achieves.

### Changed

- **Notifications, `/resume`, and import-game search pull a full
  screenful now, not a fixed 50** — each panel's own visible row
  capacity becomes the page size it requests, floored at 10 so a tiny
  terminal never falls back to single-row fetches. The server still
  clamps to each tool's configured maximum, so nothing new can be
  over-fetched, and resizing between pages changes the very next page's
  size to match.
- **Status bar's action hints lose their doubled space** — `ctrl+f
  footage · ctrl+k commands` replaces `ctrl+f update footage · ctrl+k
  commands`: a new `KbdPlain` chip renderer owns no self-padding of its
  own, instead of `KbdBare`, which was already padding a space the
  caller was also adding.
- **Scroll pills stop counting past ten** — the ctrl+home / ctrl+end
  pills now read "10+ msgs before/after" once more than ten messages sit
  out of view; counts one through ten stay exact. Mirrors the web app.

### Fixed

- **Rich message HTML renders as terminal styling, not raw tags** — chat
  bodies carrying the server's colored `<span class="text-…">` spans,
  `<pre>` ASCII/braille art, and `<br>` (e.g. the disconnect confirmation's
  shrug art) used to print the literal markup on screen. They now convert
  to terminal color (the `text-*` classes), preserve `<pre>`
  whitespace/newlines, strip other tags, and unescape entities.
- **The conversation-search hit picker's server contract, corrected before
  ship** — `internal/api/events.go` decoded a top-level `conversation_hits`
  array the server had already deleted by the time this client's code was
  written, so the picker could never open (Shift+J just typed "J"). The
  real reply (`search_conversations`, pito's
  `lib/pito/message_builder/conversation/hits.rb`) is the generic
  table_heading/table_rows list card every other list uses, each row
  carrying `conversation_uuid` + `anchor_event_id`. The picker now decodes
  that shape directly, shows the score/occurrence value in place of the
  never-real snippet column, and a cross-conversation hit submits
  `/resume <uuid>` for real instead of a "not wired yet" notice.

## [2.7.0] — 2026-07-15

### Added

- **Import a game, without leaving the terminal** — `import <title>` (or
  bare `import`, or `/games import`, or the ctrl+k palette) opens a picker
  that searches IGDB as you type: the terminal's first as-you-type remote
  search, debounced so IGDB isn't hammered. Remakes and remasters carry
  their notes, games already in your library say so (picking one re-syncs
  instead of duplicating), and Enter starts the import — the conversation
  then narrates it exactly like the web: the imported announce, then the
  done card. `import videos` still means what it always meant.

## [2.6.1] — 2026-07-14

### Changed

- **Backend failures speak owner now** — a server that answers badly (the
  classic 502 from a tunnel fronting a stopped PITO) gets its own message
  naming the fix (`pito logs` / `pito up -d`), distinct from a server
  that doesn't answer at all (check the address, the network, the box).
  The old one-size "cannot reach … (switch backends …)" line — transport
  guts and all — is gone: self-hosted means there is no elsewhere to
  switch to.

## [2.6.0] — 2026-07-14

### Added

- **Optional AppSignal telemetry, release builds only** — a new
  `[telemetry]` table in config.toml (collector `endpoint` + Push API
  `key`, `enabled = false` to opt out) ships API timings and crashes to
  AppSignal over OpenTelemetry. The gate is absolute: source builds,
  unconfigured installs, and opted-out installs send nothing — no
  goroutines, no network. Spans carry method, path, and status only; never
  bodies, queries, or cookies. A crash reports and flushes before the
  panic continues, and exit flushes are bounded so ctrl+c never hangs on a
  slow collector.

## [2.5.0] — 2026-07-14

### Added

- **`--resume` picks up where you left off, like claude does** — quitting
  with the ctrl+c double-tap now prints a parting line once the TUI has
  torn down: `To resume this conversation: pito-tui --resume "<name>"`
  (falls back to the uuid for a still-untitled conversation, and mirrors
  `--instance` into the suggested command when this run itself was
  started with one). `pito-tui --resume <uuid-or-name>` opens straight
  into that conversation instead of the default fresh chat — a uuid-shaped
  argument skips resolution entirely, anything else resolves by exact
  title (case-insensitive) against the same `/resume.json` the picker
  itself walks, and an ambiguous or missing name fails fast on stderr with
  the close candidates listed rather than guessing.
- **Grammar resync: `vids` filter vocabulary gains `private`** —
  `internal/grammar/grammar.json` regenerated from pito's `tools.yml`
  (capabilities.filters.vids), joining `published`/`unlisted`/`scheduled`
  (scope `private_unscheduled`).

## [2.4.0] — 2026-07-14

### Added

- **`/resume` picker learns `n` rename and `dd` delete** — web sidebar
  parity (resume_controller.js). `n` on the highlighted conversation opens
  an inline text input seeded with its current title (same textinput
  styling as the chatbox); enter PATCHes `/chat/:uuid` `{title:}` and
  restyles the row from the server's canonical reply, esc cancels with no
  network call, and a blank/whitespace submit cancels the same way. `d`
  arms the highlighted row for ~500ms — a second `d` within the window
  DELETEs `/chat/:uuid` and drops the row; moving the highlight or
  pressing esc disarms early, and the window auto-expires on its own,
  mirroring the web chord's timing exactly. Deleting the conversation the
  picker is open over clears it, same as the web's post-delete
  navigate-home rule. The picker's hint line now reads
  `n rename · dd delete` alongside the existing keys.

## [2.3.1] — 2026-07-13

### Fixed

- The footage probe's in-flight dots read from the left edge like every
  other line, instead of floating mid-screen.

## [2.3.0] — 2026-07-13

### Added

- **ctrl+f "update footage"** — the mini status's own hint now does
  something: ffprobe on PATH gates the flow (missing binary opens a
  warning overlay naming the distro package to install, any key
  dismisses); the existing show-game picker reopens retitled "footage"
  and, instead of sending `show game`, hands the pick to a FolderPicker
  seeded at the last folder you confirmed (persisted across runs,
  `$HOME` the first time); confirming probes every selected file with
  `ffprobe -show_entries format=duration` one at a time — a live "N/M
  probed" line, each file rounded up to the next half hour, the total
  ceiled to a whole hour (pito's `footage_hours` is integer) — then
  sends `update game footage <game> <hours>` through the ordinary send
  path, same as any typed command. Unreadable files count zero and are
  called out in a notice rather than aborting the batch; esc backs out
  cleanly from the folder or probing step. The picked game stays
  visible as a persistent breadcrumb through the folder and probing
  steps, and the reused game picker now says why it's open.

### Changed

- **Scroll-nav pills read one clear line now** — "N msgs before" /
  "N msgs after" replace the 50-variant random-pick pool with pito's
  single fixed copy string per side, shared verbatim with the web. The
  copy words render in the default (white) foreground instead of the
  muted dim style; the kbd token and jump glyph keep their own look.

## [2.2.0] — 2026-07-13

### Added

- **The `/config ai` model picker** — the web's OpenCode-style overlay,
  in the terminal: every provider with its live model list and key
  chip, Conversation/Favorites/Recents groups, type-to-filter, enter
  selects (or reveals a masked API-key entry on keyless providers),
  ctrl+f favorites, ctrl+x clears a key, and the effort cycler when
  the active provider reasons. State and writes ride the same
  `/settings/ai` pair as the web, so the two faces cannot drift.
- **Scroll-nav pills + ctrl+home / ctrl+end** — the web's floating
  "N messages above/below" pills land on the conversation's top and
  bottom edges while you're scrolled away, drawing from the same
  50-variant server-authored copy pool; ctrl+home jumps to the start,
  ctrl+end glides back to the live edge.

## [2.1.0] — 2026-07-12

### Added

- **`--update`** — the binary updates itself from the latest GitHub
  release: checksum-verified download, atomic replace. For installs
  outside brew (the AUR is still waiting on registrations).
- **The AI chrome moves faster** — the answer's gradient border bar
  sweeps at twice the house cadence (~1.3s), and the chatbox ">" now
  pulses purple↔pito-blue while an @ai turn is being typed, matching
  the web's animated chatbox bar.
- **Unified modal cursor** — /resume, /notifications, ctrl+k and the
  show pickers all wear the ls-vids look: zebra rows, ▌ bar, and a
  full-width cursor stripe.
- **The sky turned real** — stars now come in stellar colors
  (near-white, blue-white, warm yellow, purple) and four sizes from
  braille dust to a rare brilliant ✦, each breathing on its own period
  instead of the whole field pulsing in step.

### Fixed

- **Multiline notifications no longer break the panel** — messages
  collapse to one row (the IGDB sync used to spill its game list across
  lines and scatter every timestamp after it).
- The unread badge glyph is now ⚑ (the ✉ envelope rendered as tofu in
  common terminal fonts).

## [2.0.0] — 2026-07-12

The AI moved in, the grammar became tools, every screen answered the
web eye to eye, and the whole terminal came alive — the ambassador
release, shipped in step with pito 2.0.0.

### Added

- **@ai, first-class** — pito 2.0.0's assistant renders natively:
  typed content blocks (text with the content-ontology inline markup —
  bold, italic, the four sanctioned colors, kaomoji; kv tables with
  typed values where price wears the coin glyphs; data tables;
  42-cell sparklines; area/bar/heatmap charts; the braille heart,
  ported dot-for-dot from the web's own curve; score and time-to-beat
  gauges; numbered suggestions), all under the AI chrome: a traveling
  purple→pito-blue gradient bar, and a `✦ model · cost` receipt line
  that shows the provider's reported cost or nothing at all. Media
  blocks are skipped by rule — no images, ever.
- **Live block streaming** — @ai answers land block by block as the
  model produces them, with the server's playful status lines
  shimmering in the pending message; the final payload stays
  authoritative. Replies continue threads: Shift+R on an ai message
  prefills `#handle @ai `.
- **The notifications panel** — `/notifications` opens a picker-style
  overlay: ● unread / ○ read rows with day-aware stamps, fifty at a
  time, the shimmer-dots loader fetching more as you scroll.
- **A living conversation picker** — conversations with AI messages
  wear a shimmering ✦; the list lazy-loads through the server's new
  cursor pagination instead of drowning in hundreds of rows.
- **Scope cyclers, fully armed** — chat.json now serves channels and
  scope, so Shift+Tab cycles channels and Ctrl+Space cycles periods
  exactly like the web chatbox.
- **The ambassador wave** — a whisper-faint braille star-field for the
  margin the optional width cap leaves free (dormant while the app
  runs full-width); a shimmer conductor that periodically
  phases every gleam on screen into one traveling wave; status-bar
  ripples on live updates and an odometer-rolling unread badge; error
  shakes, armed-confirmation glints, self-typing ghost hints, a
  braille scroll thumb; a gradient-swept braille PITO splash; the `?`
  keymap footer in quiet kbd chips; OSC window titles and real
  clickable share links — every effect behind its own kill-switch.
- **`--tour`** — a self-playing walkthrough that types real commands
  through the real send path with caption cards, ends interactive,
  and never touches the AI provider unless `--tour-ai` says so.
- **The ctrl+k command palette** — the web's own command list, fuzzy
  subsequence search and all: Enter pre-fills the prompt with the
  command (placeholders included) and leaves the sending to you; an
  unauthenticated session sees exactly one offer, `/login`.
- **The show game / show vid picker** — the bare forms open a searching,
  50-a-page picker fed by pito's new picker endpoints; Enter sends the
  command like the web sidebar does. `/resume` opens the conversations
  picker the same way, Esc chip on the right edge of every modal.
- **A living sky** — two parallax layers of braille stars drift through
  the empty space under short conversations and behind the boot logo,
  moving even at rest; the boot tip decodes in Crush-style from braille
  static; the animation loop runs a full 60fps (transcript shimmer on
  a 30fps beat, chrome at the full rate); keyboard chips gleam with the
  brand ramp on an elevated bed.
- **Notifications mark themselves read** — arriving on an unread row
  reads it (Enter toggles), the ✉ badge rolls live and is always
  present once signed in; boot lands on a fresh chat like the web's
  start screen instead of the conversations list.
- **Input history on ↑/↓** — oh-my-zsh-style prefix recall, exactly
  the web chatbox's: the first ↑ snapshots what you've typed and walks
  only the entries that start with it, ↓ returns through newer ones to
  your draft, and the list seeds itself from the conversation's own
  echoes so it survives restarts.

### Changed

- **Verbs are tools** — the grammar snapshot generator reads pito's
  `config/pito/tools.yml` (toolsgen, pinned to the paired pito
  release), the inventory and live contracts speak "tool", and the
  wire sweeps accept both old and new field spellings.
- **Full-width rendering** — the conversation, meter, prompt and
  status bar all stretch to the terminal's real width (charts stay 42
  cells, gauges cap at 56), and the server is told that width so its
  tables use it too. The earlier 100-column containment experiment
  survives behind a build flag, off by default.
- **Mouse wheel scrolling** — the wheel scrolls the conversation
  (three lines a notch, waking the braille scroll thumb) and moves the
  cursor in the notifications overlay.
- **Select to copy** — drag over any text Claude-Code-style: the
  selection highlights live, lands on the system clipboard the moment
  you release (OSC 52), and a small toast on the status row confirms it
  with one of the owner's fifty quips ("Yoinked to your clipboard.",
  "Copied. It's your problem now.") — the pool lives in pito's copy
  file like every other word. The terminal's own selection stays a
  shift+drag away.
- **Charm v2 stack** — bubbletea, lipgloss and bubbles moved to the
  charm.land v2 generation (new cell-diffing renderer); goldens
  proved the move pixel-invisible.
- **Tables, aligned to Charm** — bold purple header rows, purple
  rules, alternating neutral-gray row foregrounds; the plum background
  zebra is gone everywhere (detail cards included), and selections sit
  on a neutral elevated gray.
- **Timestamps grew days** — today stays `15:04`; the rest of the
  year reads `6 Jul 15:04`; other years `2 Jan '25 15:04`.
- **Analyze charts grew up** — every stash metric renders the full
  ticked area chart (duration ticks as M:SS, watched-percent on a
  fixed 0–100% axis, date x-ticks), keeping the TUI's own
  total/prev/target caption; breakdowns draws its whole payload now —
  demographic and device bar groups with colored legends, the
  day-of-week heatmap, retention and comments curves.
- **Subjects shimmer pink, references shimmer cyan** — each base paired
  with a band mathematically derived from it (+24° of hue, a touch of
  light), theme-agnostic by construction; the orange era is over.
- **Charts came alive** — braille fills ride the web's own red→orange→
  yellow→green health ramp against each chart's data-driven target
  anchor (no target means all green), with the white glint still
  sweeping over the color; help and /config section headers
  render bold purple with proper breathing room; multi-paragraph
  descriptions keep their paragraphs.
- **Reply affordances match the web** — `#handle shift+r` with a
  quiet kbd chip, one shared renderer for every message kind.
- **The thinking indicator grew up** — a braille spinner beside the
  web's own cycling verbs ("Frobnicating…", "Bribing the cache…" —
  Capitalized, network-shimmered, replayed from the payload's shared
  time formula so a browser on the same turn shows the same word),
  resolving to a seeded kaomoji and the past-tense receipt
  ("\o/ Read tea leaves for 1.02s"). The pools ride a new copygen
  snapshot of pito's committed copy — the client never authors words
  (owner's copy law); the analyze/glance pending fills hold their space
  with the web's own dot-grid canvas, and the send-window comet dropped
  its invented "thinking…" caption (the web shows none).
- **The status bar slimmed to the bone** — one dot, one tag: `■ dev`
  against a development server (dev.pitomd.com or localhost), `■ 2.0.0`
  against a released one (the tag fetched from the server's own
  /version, refreshed on every reconnect), pito's word "tarnished"
  while logged out. The nickname is gone — pito cut the whole "me"
  concept and the TUI followed. The dot: red while unauthenticated,
  orange while an authenticated session connects or drops, green when
  live — taking a single breath per delivered cable message instead of
  idling. The conversation name lives where the web puts it (the meter
  row), the cycler hints moved down beside the status dot, and every
  modal wears its Esc chip on the right edge.
- **The boot screen is pito's** — the exact block-art logo in brand
  blue, and a start-screen tip from pito's own fifty ("Type /help when
  you inevitably forget everything.") instead of the client-made
  tagline; the cycler hints above the prompt wear the web's kbd-chip +
  value shape, red `none` included.
- **Shinies gleam like they mean it** — a breathing halo, a sharper
  material-tinted gleam that inks the face as it crosses, an
  iridescent ✦ sparkle reserved for the iridescent materials (pearl,
  opal, diamond), and extended badges now carry their unlock month.

### Removed

- The drawer-slide and grow-in spring animations — smoke-tested and cut
  for cause: the slide's anchoring flickered on short panels and the
  bounce read as delay on high-refresh displays. Overlays open settled,
  new content lands at full height, and the follow-glide carries the
  arrival motion. The boot splash's rise and the `?` footer's accordion
  keep their springs.

### Fixed

- `/resume` opens the TUI's own conversation picker (the server now
  reserves its version for browsers), and esc closes it back into the
  conversation it was opened over.
- The suggestions palette overlays the conversation instead of
  reflowing it — no more layout jumps while typing commands.
- The conversation is virtualized: only visible lines are rendered,
  measured, or animated — a thousand-turn scrollback types and scrolls
  like an empty one, and off-screen shimmer costs nothing.
- Ctrl+C asks before it quits: the first press arms a two-second
  "again to quit" window on the status row.
- New content glides the conversation to the bottom instead of
  snapping; chrome-only animation frames stopped re-rendering the whole
  scrollback 25 times a second.
- Resolved thinking lines and settled confirmations no longer hold
  their turns on the animation loop — a long scrollback types smoothly
  again instead of re-rendering the whole transcript 25 times a second.
- The vim scroll letters (j/k/g/G at an empty prompt) are gone — 2.0.0
  commands start with them, so "glance" typed at rest reached the
  server as "lance". Scrolling keeps ctrl+u/d, pgup/pgdn, shift+↑/↓
  and the mouse wheel; every letter now types.
- `/jobs status` renders its queue table (kv-hash rows with the
  web's own class colors) instead of dropping it.
- Old servers see byte-identical requests despite the new
  pagination-aware clients.

### Known

- A conversation stacking many simultaneously-shimmering turns (the
  tour does this deliberately) grows the per-tick re-render cost —
  the wire is cheap on the v2 renderer, the string rebuild is not;
  a render-cache sweep is queued for 2.0.x.

## [1.2.0] — 2026-07-11

The grammar comes home, the chatbox learns the web's hands, and every
YouTube-safe mutation ran the gauntlet on dev.

### Added

- **The grammar importer (verbsgen)** — `tools/verbsgen` reads pito's
  `config/pito/verbs.yml` straight from git (pinned to the paired pito
  release, `PITO_REF=v1.6.0`; a dirty pito checkout can never leak WIP
  grammar) and emits a typed, deterministic snapshot
  (`internal/grammar/grammar.json`, go:embed loader). Live contract
  specs now assert list columns, aliases, sortability, and the new
  v1.6.0 **filters** (genres/platforms vocabularies, `list games rpg`,
  `list vids scheduled`) from the generated tables instead of
  hand-copied ones — when pito's grammar moves, one ref bump moves the
  specs. The runtime binary stays grammar-free (enforced by test).
- **Chatbox hands, web parity** — `Shift+R` at an empty prompt prefills
  the newest live reply handle (several → picker, none → types
  through); `Shift+Tab` cycles the channel scope while a `list
vids/games` hint is live; `Ctrl+Space` cycles the analytics period
  while an `analyze` hint is live (the web's Shift+Space is invisible
  to terminals). Scope rides the send only while its hint shows, and
  persists via the conversation PATCH — exactly the web's rules.
- **Space dismisses the palette** and the space still types, Enter
  always sends the raw input (both pinned by regression tests) — the
  web's suggestion semantics, key for key.
- **Video tags** render as their own section on `show vid` cards
  (v1.6.0 moved them out of the kv grid), and channel descriptions
  keep their standalone block.
- **No-data charts wear the server's copy** — "No data yet." centered
  over the paper grid, straight from the payload's nodata label.
- **Confirmation cards show the stats detail** — the web's
  `expand_detail` block (e.g. a disconnect's Subs/Views/Vids tally)
  renders 1:1 under a hairline with cyan keys while the confirmation
  is pending, and drops once resolved.
- **Help blocks keep their layout** — `--help` output is pre-formatted
  text and now renders line for line instead of collapsing into a
  paragraph.

### Changed

- **Zebra, retuned** — table stripes settled on a subtler plum
  (`#1B142B`): still reads purple, no longer shouts.

### Verified

- The full YouTube-safe mutation round ran live against dev with a
  disposable IGDB import: link/unlink, `update game
price/platform/footage`, card replies, delete + reindex confirmation
  flows (confirm AND cancel), `/rename`, `/config` getters + reversible
  setters, `/disconnect` (cancel), `/logout` → `/login`. Game imports
  are web-only for JSON clients by server design — the TUI renders the
  server's notice. `publish`/`unlist`/`schedule`/`sync` stay untouched
  by policy: `delete vid` hard-deletes on YouTube and is never fired.

## [1.1.1] — 2026-07-06

The gloss, buffed until nothing catches.

### Changed

- **The surface gloss, de-harshed** — every surface glint (charts,
  platform chips, shiny badges) moved from a hard-edged additive band
  to a Gaussian sheen: no cutoff, blur-soft falloff, every cell always
  carrying some light around a traveling peak — the same smoothness the
  text shimmer has. Glints are tinted, not white-stamped: charts lift
  toward an airy pito-blue, chips toward warm glass, and every shiny
  badge gleams in its own material's tone (amber warm, jade green) —
  while pearl, opal, and diamond iridesce, their gleam cycling through
  the brand ramp as it travels. Coins catch the light in turn along
  their run, Mario-style, and the FREE star twinkles with them.

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
  _similar_ as recommendation rows — `#id Title` beside a label-less
  score bar, brackets aligned down the strip; _channels_ as the coverage
  block (one solid colored share bar per channel, redrawn from the
  distribution legend's own data, caption below) plus the fit-score
  roster (handle from the avatar's alt, score bar per row);
  _linked-videos_ was already a `Video::List` payload and rides the
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
