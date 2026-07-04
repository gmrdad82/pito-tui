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
- **First-run backend setup** — with no config file, the client asks
  which PITO instance to talk to (enter keeps the default, bare hosts
  get `https://`) and writes a commented `config.toml` you can edit by
  hand later. `--instance <url>` switches backends for a single run
  without touching the file; `--config <path>` swaps the whole file.
  Cookies and sound caches are keyed per backend, so switching
  instances never crosses sessions or cues.
- **TOTP-only login** — `POST /session {otp}`, the same 6-digit code as
  the web's `/authenticate`. The session cookie persists to
  `~/.config/pito-tui/cookies.json` (0600, atomic writes) and survives
  restarts; the 24h idle timeout re-prompts exactly once needed, and a
  throttled login says so instead of digging deeper.
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
  linux amd64+arm64 binaries with checksums, a `.deb`, and the
  `pito-tui-bin` AUR package.
- **Dependabot** — weekly grouped gomod + github-actions PRs, same
  grouping intent as pito; vulnerability alerts and automated security
  fixes enabled.
