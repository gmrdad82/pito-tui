# pito-tui

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Sponsor](https://img.shields.io/badge/Sponsor-%E2%9D%A4-ff69b4?logo=githubsponsors)](https://github.com/sponsors/gmrdad82)

Terminal client for [PITO](https://github.com/gmrdad82/pito) — the
self-hosted, chat-first YouTube channel manager. One scrollback, one prompt,
the same server-side command grammar as the web chatbox, live over
ActionCable. PITO in the place you already live.

The TUI is a thin client by design: it sends raw text (slash commands
included) and renders the JSON events the server emits. All parsing,
grammar, and behavior live in the Rails app — this binary is the window,
your server is the product.

Phone person? The same instance also fits in your pocket:
[**`pito-android`**](https://github.com/gmrdad82/pito-android), a native
Android shell around your instance's own UI. And for the full tour before
you commit to anything, the showcase lives at
[**pitomd.com**](https://pitomd.com) ([source](https://github.com/gmrdad82/pitomd)).

## Install

**Arch / Omarchy**

```sh
yay -S pito-tui-bin
```

**Ubuntu / Debian** — download `pito-tui_*.deb` from the
[latest release](https://github.com/gmrdad82/pito-tui/releases/latest), then:

```sh
sudo apt install ./pito-tui_*.deb
```

**Anything else (linux amd64/arm64)** — grab the static binary from the
release tarball and put it on your PATH.

## Usage

```sh
pito-tui                      # conversation picker (recent first)
pito-tui <conversation-uuid>  # open a conversation directly
```

First run asks for your TOTP code (the same 6-digit code as
`/authenticate` on the web) and keeps the session cookie in
`~/.config/pito-tui/cookies.json` until it expires.

## Configuration

`~/.config/pito-tui/config.toml`:

```toml
instance_url = "https://app.pitomd.com"  # your PITO instance
sounds = true                            # send/receive/notify sounds
```

Flags override the file: `--instance <url>`, `--sounds=on|off`.

Sounds play through `paplay` or `mpv` if either is installed, and stay
silent otherwise.

## Keys

| Key | Action |
| --- | --- |
| `enter` | send the prompt |
| `j` / `k` | scroll one line (when the prompt is empty) |
| `ctrl-d` / `ctrl-u` | scroll half a page |
| `g` / `G` | top / bottom (G re-enables follow) |
| `ctrl-c` | quit |

Everything else you type goes into the prompt — including slash commands,
which are sent to the server as-is.

## Development

Go 1.26+ (pinned via [mise](https://mise.jdx.dev) in `mise.toml`).

```sh
go test -race ./...      # full suite
scripts/coverage.sh      # suite + the 80% coverage floor CI enforces
```

Releases are tag-driven (`v*`): CI must be green on the tagged commit, then
goreleaser publishes static binaries, a `.deb`, and the `pito-tui-bin` AUR
package.

## License

AGPL-3.0 — same as PITO.
