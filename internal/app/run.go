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
	"runtime/debug"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/muesli/termenv"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/cable"
	"github.com/gmrdad82/pito-tui/internal/config"
	"github.com/gmrdad82/pito-tui/internal/sound"
	"github.com/gmrdad82/pito-tui/internal/telemetry"
	"github.com/gmrdad82/pito-tui/internal/ui"
)

// Options carries the parsed CLI surface.
type Options struct {
	ConfigPath   string  // "" → default path
	InstanceURL  *string // nil → not set on the CLI
	Sounds       *bool   // nil → not set on the CLI
	Conversation string  // positional argument, "" → picker
	Resume       string  // --resume <uuid-or-name>, "" → not requested
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

	// Telemetry (AppSignal via OTLP): inert unless this is a RELEASE build
	// with a configured [telemetry] table — see internal/telemetry. Shutdown
	// is declared BEFORE the panic capture so LIFO ordering runs the capture
	// (report + flush + re-panic) first, and the final flush covers clean
	// exits. Bubble Tea already restores the terminal before re-panicking
	// out of its loop, so re-panicking here keeps the default crash output.
	// Limitation: a panic on a non-Tea goroutine (e.g. cable) kills the
	// process without unwinding this stack and is not captured.
	reporter := telemetry.Init(cfg.Telemetry)
	defer reporter.Shutdown()
	defer func() {
		if rec := recover(); rec != nil {
			reporter.ReportPanic(rec, debug.Stack())
			panic(rec)
		}
	}()

	dir, err := config.Dir()
	if err != nil {
		return err
	}
	client, err := api.New(cfg.InstanceURL, filepath.Join(dir, "cookies.json"))
	if err != nil {
		return err
	}
	client.WrapTransport(reporter.Transport)

	// ctrl+f footage flow: the last-used folder persists across runs in its
	// own small state file (config.FootagePath, jar.go's own write-through
	// JSON pattern) — the ui package never touches disk itself.
	footagePath, err := config.FootagePath()
	if err != nil {
		return err
	}
	lastFootageFolder := config.LoadFootageFolder(footagePath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Preflight's messages are self-contained per failure state (owner
	// 2026-07-14: no "switch backends" framing — self-hosted means there is
	// no elsewhere; the remedy is almost always "start your box").
	authed, err := Preflight(ctx, client)
	if err != nil {
		return err
	}

	// --resume <uuid-or-name> (owner 2026-07-14, "like claude does"):
	// resolved once, up front, so a bad name fails fast with a clean
	// error instead of silently landing on the fresh-chat default.
	resumeUUID, err := resolveResumeArg(ctx, client, opts.Resume, authed)
	if err != nil {
		return err
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

	modelOpts := []ui.Option{
		ui.WithSounds(player),
		ui.WithGlamourStyle(glamourStyle),
		ui.WithTruecolor(truecolor),
		// The [fx] table (config.Fx): the sky toggle plus the ambient
		// heartbeat's activity-gating knobs — see ui/ambient.go's state
		// ladder for what each one gates.
		ui.WithStarSky(cfg.Fx.Sky),
		ui.WithFxTuning(ui.FxTuning{
			PauseOnBlur: cfg.Fx.PauseOnBlur,
			IdleGrace:   time.Duration(cfg.Fx.IdleGraceSeconds) * time.Second,
			IdleFPS:     cfg.Fx.IdleFPS,
			DeepIdle:    time.Duration(cfg.Fx.DeepIdleMinutes) * time.Minute,
		}),
		ui.WithFootageFolder(lastFootageFolder, func(folder string) error {
			return config.SaveFootageFolder(footagePath, folder)
		}),
	}
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
	case resumeUUID != "":
		// --resume, already resolved to a uuid above — an explicit flag
		// wins over the persisted config.Conversation default.
		modelOpts = append(modelOpts, ui.WithConversation(resumeUUID))
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
	finalModel, runErr := program.Run()
	// Post-quit hint (owner 2026-07-14, "like claude does"): Bubble Tea
	// has released the terminal by the time program.Run() returns, so
	// this is the one place in Run() safe to print to — never earlier,
	// never from inside the Model/Program themselves.
	printResumeHint(opts, finalModel, runErr)
	return runErr
}

// Preflight distinguishes "reachable but not logged in" (the TUI starts
// unauthenticated and the user types /login <code> — the app's own
// grammar, like the web chatbox) from "cannot reach the backend at all"
// (an error worth stopping for).
func Preflight(ctx context.Context, client *api.Client) (authed bool, err error) {
	_, err = client.FetchResume(ctx, "", 0)
	var se *api.StatusError
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, api.ErrUnauthorized):
		return false, nil
	case errors.As(err, &se):
		// The server ANSWERED, badly — a proxy/tunnel fronting a dead PITO
		// (the classic 502). Owner-operator voice: name the fix commands.
		return false, fmt.Errorf(
			"%s answered %d — something is listening, but PITO isn't.\nIf that's your box: `pito logs` will say why, `pito up -d` brings it back",
			client.BaseURL().Host, se.Code)
	default:
		// Nothing answered at all: DNS, refused connection, or timeout.
		// (Owner 2026-07-14: "the server", not the host echo.)
		return false, errors.New(
			"no answer from the server — nothing is listening there.\nCheck the address and your network; if that's your box, is it up?")
	}
}
