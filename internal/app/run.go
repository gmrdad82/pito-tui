// Package app wires everything into a runnable client: config, the
// backend preflight, the cable→Bubble Tea bridge, and program startup.
// main() stays a flag parser; the logic lives here where tests reach it.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/muesli/termenv"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/cable"
	"github.com/gmrdad82/pito-tui/internal/config"
	"github.com/gmrdad82/pito-tui/internal/sound"
	"github.com/gmrdad82/pito-tui/internal/ui"
)

// Options carries the parsed CLI surface.
type Options struct {
	ConfigPath   string  // "" → default path
	InstanceURL  *string // nil → not set on the CLI
	Sounds       *bool   // nil → not set on the CLI
	Conversation string  // positional argument, "" → picker
	Tour         bool    // --tour: play the self-driving walkthrough, then hand back control
	TourAI       bool    // --tour-ai: include the @ai step (spends the AI provider's budget) — no-op without Tour
	Stdout       io.Writer
}

// Run is the program: load config, check the backend, start the TUI.
func Run(opts Options) error {
	cfgPath := opts.ConfigPath
	if cfgPath == "" {
		var err error
		if cfgPath, err = config.DefaultPath(); err != nil {
			return err
		}
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	cfg = cfg.WithFlags(opts.InstanceURL, opts.Sounds, opts.Conversation)

	// No instance configured: stop gracefully with the way in. There is
	// deliberately NO default and NO suggestion — pito is self-hosted and
	// this client points wherever the owner says, nowhere else.
	if cfg.InstanceURL == "" {
		return fmt.Errorf(`no PITO instance configured.

Point pito-tui at your install:

  pito-tui config server=https://pito.example.com   (saved to %s)
  pito-tui --instance https://pito.example.com      (this run only)`, cfgPath)
	}

	dir, err := config.Dir()
	if err != nil {
		return err
	}
	client, err := api.New(cfg.InstanceURL, filepath.Join(dir, "cookies.json"))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	authed, err := Preflight(ctx, client)
	if err != nil {
		return fmt.Errorf("%w\n(switch backends with --instance <url> or `pito-tui config server=<url>`)", err)
	}

	player := sound.New(cfg.InstanceURL, cfg.Sounds)

	// Truecolor detection is env-only (COLORTERM) — same no-query rule.
	truecolor := strings.Contains(os.Getenv("COLORTERM"), "truecolor") ||
		strings.Contains(os.Getenv("COLORTERM"), "24bit")

	// Resolve the markdown style NOW, before Bubble Tea owns the terminal:
	// termenv's background query talks over stdin, and doing it inside the
	// program deadlocks against tea's input reader (the "loading…" freeze).
	// Bubble Tea v2 still owns stdin exclusively once Program.Run() starts —
	// this synchronous, pre-tea query is unchanged and still required. v2
	// does add an async alternative (tea.RequestBackgroundColor in Init(),
	// tea.BackgroundColorMsg in Update()) for programs that want the query
	// to run inside the event loop, but that trades this one blocking call
	// for an async render-setup dance (the renderer isn't built until the
	// message arrives) — not worth it here, and the v2 upgrade guide's own
	// "compat" quick-path keeps doing exactly this: blocking, outside tea.
	glamourStyle := "dark"
	if !termenv.HasDarkBackground() {
		glamourStyle = "light"
	}

	var program *tea.Program
	connect := func(uuid string) {
		cab, err := cable.New(cable.Config{
			InstanceURL: cfg.InstanceURL,
			UUID:        uuid,
			Jar:         client.Jar(),
			OnMessage: func(m cable.StreamMessage) {
				program.Send(ui.CableEventMsg{M: m})
			},
			OnState: func(s cable.ConnState, err error) {
				program.Send(ui.ConnStateMsg{State: s, Err: err})
			},
		})
		if err != nil {
			return
		}
		go func() { _ = cab.Run(ctx) }()
	}

	modelOpts := []ui.Option{ui.WithSounds(player), ui.WithGlamourStyle(glamourStyle), ui.WithTruecolor(truecolor)}
	switch {
	case !authed:
		// Login is the app's own grammar: the user types /login <code>
		// into the chat, exactly like the web chatbox. No side-channel
		// prompt — the TUI opens unauthenticated and says so. (This wins
		// over --tour too: the script sends real messages, so it needs a
		// real session — an unauthenticated run just falls back to the
		// ordinary login banner instead of typing into a dead end.)
		modelOpts = append(modelOpts, ui.WithLoginRequired())
	case opts.Tour:
		// --tour/--tour-ai (ambassador wave): a self-playing, zero-
		// interaction walkthrough against a brand-new conversation — see
		// ui.TourScript/ui.WithTour.
		modelOpts = append(modelOpts, ui.WithTour(ui.TourScript(opts.TourAI)))
	case cfg.Conversation != "":
		modelOpts = append(modelOpts, ui.WithConversation(cfg.Conversation))
	default:
		// pito's own flow (owner 2026-07-12): boot lands on a FRESH chat
		// — the splash plays, the prompt waits, the first send creates
		// the conversation. The conversations list is /resume's job now,
		// never the landing screen.
		modelOpts = append(modelOpts, ui.WithNewConversation())
	}
	model := ui.NewModel(client, connect, modelOpts...)

	// AltScreen moved from a NewProgram option to a declarative field on
	// the model's View() (tea.View.AltScreen) in v2 — see ui.Model.View.
	program = tea.NewProgram(model)
	_, err = program.Run()
	return err
}

// Preflight distinguishes "reachable but not logged in" (the TUI starts
// unauthenticated and the user types /login <code> — the app's own
// grammar, like the web chatbox) from "cannot reach the backend at all"
// (an error worth stopping for).
func Preflight(ctx context.Context, client *api.Client) (authed bool, err error) {
	_, err = client.FetchResume(ctx, "", 0)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, api.ErrUnauthorized):
		return false, nil
	default:
		return false, fmt.Errorf("cannot reach %s: %w", client.BaseURL().Host, err)
	}
}
