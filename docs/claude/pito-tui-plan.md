# pito-tui — Bubble Tea terminal client (starting-point plan)

Goal: a Go TUI client (repo `gmrdad82/pito-tui`, AGPL-3.0) for a PITO
instance. Mirrors the web chat-shell UX: scrollback of turns, one prompt,
slash commands, live updates over ActionCable. Server-side grammar stays
authoritative — the TUI sends raw text and renders JSON events; it never
parses commands. Owner does not write Go: Claude Code implements everything;
steps marked **HANDED TO YOU** are the only ones the owner runs manually.

Depends on the Rails-side JSON support (see pito-rails-tui-support-prompt.md).
The JSON contract both sides implement (corrected 2026-07-04 against the
actual pito codebase + the Rails-side session decisions):

```
POST /session   (JSON: otp)   -> Set-Cookie pito_session   (TOTP-only; pito
                                 has no passwords. Statuses: ok / invalid /
                                 throttled. 24h sliding idle timeout.)
GET  /resume.json             -> { recent: [...], older: [...] } rows of
                                 uuid + title + last-activity (recency_groups)
GET  /chat/:uuid.json         -> { conversation, events: [...] }
POST /chat      (JSON: input, uuid, viewport_width) ->
                 { accepted, turn_id }        existing conversation
                 { uuid } 201                 blank uuid: conversation created
                 { error: "web-only", verb }  web-only verbs (/themes, /resume,
                                              /new, /connect, bare show game/vid,
                                              /games import) -> TUI dim notice
Cable: subscribe channel "TuiChannel" with { uuid }; session-authenticated
  (reject guests). Stream "pito:json:conversation:<uuid>" messages:
  { "type": "event.append" | "event.replace",
    "event": { "id", "turn_id", "kind", "payload", "created_at" } }
Event kinds (Event::KINDS): echo system error enhanced thinking confirmation
  system_follow_up enhanced_follow_up confirmation_follow_up theme_diff
  (unknown kinds must render via fallback, never crash)
```

Rails-side coordination notes: CSRF must be skipped/null_session for JSON on
/session + /chat (the TUI cannot fabricate a token); cable
allowed_request_origins must accept `Origin: <instance_url>` as the TUI sends
it; non-TLS dev instances need TLS-or-no-Secure cookies (Go's cookiejar will
not send a Secure cookie over http://).

## Phase 0 — Environment + repo (adopt pito conventions via gh)
- [x] Go toolchain: installed via `mise use go@latest` (go 1.26.4, pinned in
      mise.toml — no sudo needed; supersedes the pacman hand-off). gh was
      already installed + authed.
- [ ] **HANDED TO YOU (later, Phase 7):** AUR account at aur.archlinux.org,
      SSH key added to it; GitHub secrets: AUR_SSH_PRIVATE_KEY, SLACK_WEBHOOK
- [x] Repo existed empty on GitHub → `gh repo edit` set description/topics
      pito-style; AGPL LICENSE, README added
- [ ] Port shared conventions FROM gmrdad82/pito via gh (fetch files, adapt):
      `.github/actions/slack-notify` composite action copied verbatim
      (it is language-agnostic: job status + failing-step lookup + Deadpan
      Butler voice, `actions: read` permission required in callers)
- [ ] Repo scaffold: go.mod (module github.com/gmrdad82/pito-tui),
      .github/workflows/{ci.yml,release.yml} per Phases 6–7

## Phase 1 — Config + auth
- [ ] Config file ~/.config/pito-tui/config.toml: instance_url (default
      https://app.pitomd.com), sounds on/off; flags override file
- [ ] Login flow: prompt for the TOTP code only (pito is TOTP-only, no
      email/password) → POST /session (JSON: otp) → persist cookie jar to
      ~/.config/pito-tui/cookies.json (0600); reuse jar on start, re-login
      only on 401; throttled → friendly stop, never retry-loop
- [ ] Tests: config precedence, jar round-trip, 401→relogin path (httptest)

## Phase 2 — HTTP client
- [ ] GET /chat/:uuid.json → decode conversation + events (initial paint
      and reconnect re-sync); POST /chat with input text
- [ ] Typed event model matching the contract; unknown `kind` decodes into
      a fallback (render as raw payload, never crash)
- [ ] Tests: golden JSON fixtures for each known kind + unknown-kind fallback

## Phase 3 — ActionCable client (minimal, in-repo)
- [ ] Implement the ActionCable wire protocol over gorilla/websocket:
      welcome, subscribe (identifier for the TUI channel with conversation
      uuid), confirm_subscription, ping tracking, message dispatch. Send
      the session cookie on the ws handshake. No external cable libs.
- [ ] Reconnect with exponential backoff; on reconnect: refetch scrollback
      JSON and diff-append (cable has no replay — HTTP is the re-sync)
- [ ] Tests: protocol framing against a scripted httptest websocket server;
      reconnect triggers re-sync

## Phase 4 — Bubble Tea UI
- [ ] Model: viewport (scrollback) + textinput (prompt) + status bar
      (connection state, conversation name, instance host)
- [ ] Cable messages → tea.Msg; event.append opens/extends its turn block,
      event.replace rewrites in place (mirrors the web turn containers)
- [ ] Per-kind renderers with lipgloss (start: system, enhanced; glamour
      for markdown-ish payload text; fallback renderer for the rest)
- [ ] Pending turn → bubbles spinner until first event of the turn arrives
- [ ] Keybindings (keyboard-only user): vim-style scroll (j/k, ctrl-d/u,
      g/G), enter send, ctrl-c quit; slash commands pass through as text
- [ ] Tests: update-loop unit tests + teatest golden-frame tests for the
      main states (empty, streaming, replaced event, disconnected banner)

## Phase 5 — Sounds (optional, graceful)
- [ ] On first run fetch /sounds/{send,receive,notify}.mp3 from the
      instance → cache in ~/.cache/pito-tui/; play via first available of
      `paplay`/`mpv --no-video`; silently disable if neither exists
- [ ] Tests: player selection + cache logic (exec faked)

## Phase 6 — CI (ci.yml, mirrors pito's structure adapted to Go)
- [ ] Jobs: `test` (gofmt check, go vet, staticcheck,
      `go test -race -cover ./...` with 80% floor), `lint-shell`
      (shellcheck for any repo scripts), `vuln` (govulncheck — the Go
      analog of pito's bundler-audit job)
- [ ] EVERY job ends with the ported slack-notify step, pito-style:
      `if: always()`, `continue-on-error: true`, SLACK_WEBHOOK secret,
      `permissions: actions: read` + `contents: read`
- [ ] Triggers: push, pull_request, and the weekly cron pito uses
      (`0 12 * * 1`) as a bit-rot canary
- [ ] Matrix: linux amd64 + arm64 build check

## Phase 7 — Release: gated, GitHub + deb + AUR (release.yml, on tag)
- [ ] Port pito's `verify-ci` gate job: query the Actions API for all
      workflow runs on the tag's head SHA; proceed ONLY if every non-release
      workflow is green (no builds on red CI, verbatim pito behavior)
- [ ] `release` job `needs: verify-ci`: goreleaser → linux amd64+arm64
      static binaries, checksums, GitHub Release; nfpm → pito-tui_*.deb
      attached (Ubuntu: download → `apt install ./pito-tui_*.deb`)
- [ ] goreleaser AUR publisher → `pito-tui-bin` pushed to AUR via
      AUR_SSH_PRIVATE_KEY (then `yay -S pito-tui-bin`)
- [ ] Final slack-notify with pito's headline/extra pattern: version
      published headline + `yay -S pito-tui-bin` snippet as the extra line;
      failure notifications name the broken step
- [ ] **HANDED TO YOU:** first ssh push initializes the AUR package;
      add both secrets; tag v1.0.0 (owner decision 2026-07-04: first release
      is 1.0.0; semantic versioning after that, bump level at Claude's
      discretion)

## Acceptance
- [ ] `yay -S pito-tui-bin` on Omarchy; login with TOTP once; open the
      conversation; a message sent from the TUI appears live in the web
      app and Android app, and vice versa; kill wifi → banner → reconnect
      re-syncs with nothing missing; `go test ./...` green at ≥80% coverage
