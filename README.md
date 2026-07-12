# pito-tui

[![CI](https://github.com/gmrdad82/pito-tui/actions/workflows/ci.yml/badge.svg)](https://github.com/gmrdad82/pito-tui/actions/workflows/ci.yml)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Sponsor](https://img.shields.io/badge/Sponsor-%E2%9D%A4-ff69b4?logo=githubsponsors)](https://github.com/sponsors/gmrdad82)

![pito-tui: starfield boot, braille analytics in color, the game picker](docs/media/pito-tui-loop.gif)

Terminal client for [PITO](https://github.com/gmrdad82/pito) — the
self-hosted, chat-first YouTube channel manager. One scrollback, one prompt,
the same server-side command grammar as the web chatbox, live over
ActionCable. PITO in the place you already live.

The TUI is a thin client by design: it sends raw text (slash commands
included) and renders the JSON events the server emits. All parsing,
grammar, and behavior live in the Rails app — this binary is the window,
your server is the product.

Built on the [Charm](https://github.com/charmbracelet) stack — Bubble
Tea, Lip Gloss, Bubbles, Harmonica, and VHS for every capture you see
here. The star sky, the shimmer, the springs: that's their toolkit,
pushed as far as we could take it.

Phone person? The same instance also fits in your pocket:
[**pito-android**](https://github.com/gmrdad82/pito-android), a native
Android shell around your instance's own UI. And for the full tour before
you commit to anything, the showcase lives at
[**pitomd.com**](https://pitomd.com) ([source](https://github.com/gmrdad82/pitomd)).

## Install

**Arch / Omarchy**

```sh
yay -S pito-tui-bin
```

Updates arrive with your normal `yay -Syu` — the release pipeline
maintains the AUR package. (Landing in the first patch release: the
AUR's account registration was closed upstream when 1.0.0 shipped.
Until then, Arch folks: Homebrew below, the `.deb`, or the tarball.)

**Installed a raw binary?** It updates itself:

```sh
pito-tui --update
```

**macOS / Linux via Homebrew**

```sh
brew install gmrdad82/tap/pito-tui
```

One formula, both platforms (Apple Silicon and Intel included); `brew
upgrade` keeps it current. On macOS sounds play through the built-in
`afplay` — nothing extra to install.

**Ubuntu / Debian** — download `pito-tui_*.deb` for your architecture
from the [latest release](https://github.com/gmrdad82/pito-tui/releases/latest),
then:

```sh
sudo apt install ./pito-tui_*.deb
```

(No apt repository — grab the new `.deb` when a release catches your eye.)

**Anything else** — static binaries for linux and darwin, amd64 and
arm64, are on every release. Untar and put `pito-tui` on your PATH:

```sh
tar -xzf pito-tui_*_linux_amd64.tar.gz
install -Dm755 pito-tui ~/.local/bin/pito-tui
```

## Usage

```sh
pito-tui                      # conversation picker (recent first)
pito-tui <conversation-uuid>  # open a conversation directly
```

pito-tui ships pointing at nothing — it's a client for *your* server,
and it will never suggest anyone else's. First run tells you exactly
that and how to fix it:

```
$ pito-tui
pito-tui: no PITO instance configured.

Point pito-tui at your install:

  pito-tui config server=https://pito.example.com   (saved to ~/.config/pito-tui/config.toml)
  pito-tui --instance https://pito.example.com      (this run only)
```

(A bare host gets `https://` for free.) Logging in then happens where
everything else does — in the chat: when the banner says so, send
`/login <code>` with your TOTP, exactly like the web chatbox. The
server mints the session; the TUI just keeps the cookie
(`~/.config/pito-tui/cookies.json`) until it expires.

## Configuration

`~/.config/pito-tui/config.toml` — created by `pito-tui config
server=…`, yours to edit, or drive it from the CLI:

```sh
pito-tui config                              # show server, sounds, file path
pito-tui config server=pito.example.com     # set/switch backends (persists)
pito-tui config sounds=off                   # keys: server, sounds, conversation
pito-tui version                             # what am I running
```

```toml
instance_url = "https://pito.example.com"  # your PITO instance
sounds = true                              # send/receive/notify sounds
```

Flags override the file per run: `--instance <url>` (never rewrites the
config), `--sounds=on|off`, `--config <path>` for an alternate file
entirely. Sessions and sound caches are kept per backend, so hopping
between a dev and a production instance never crosses wires.

Sounds play through `paplay` or `mpv` on Linux and the built-in
`afplay` on macOS, and stay silent when nothing can play.

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

Releases are tag-driven (`v*`): CI must be green on the tagged commit,
then goreleaser publishes static binaries (linux+darwin, amd64+arm64), a
`.deb`, the `pito-tui-bin` AUR package, and the Homebrew formula.

## License

AGPL-3.0 — same as PITO.
