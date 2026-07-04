// Package app wires everything into a runnable client: config, the
// pre-TUI login preflight, the cable→Bubble Tea bridge, and program
// startup. main() stays a flag parser; the logic lives here where tests
// can reach it.
package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

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
	Stdin        io.Reader
	Stdout       io.Writer
}

// Run is the program: load config, ensure a session, start the TUI.
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

	if err := Preflight(ctx, client, opts.Stdin, opts.Stdout); err != nil {
		return err
	}

	player := sound.New(cfg.InstanceURL, cfg.Sounds)

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

	modelOpts := []ui.Option{ui.WithSounds(player)}
	if cfg.Conversation != "" {
		modelOpts = append(modelOpts, ui.WithConversation(cfg.Conversation))
	}
	model := ui.NewModel(client, connect, modelOpts...)

	program = tea.NewProgram(model, tea.WithAltScreen())
	_, err = program.Run()
	return err
}

// Preflight makes sure the session cookie works before the TUI takes over
// the terminal: one cheap authenticated call, and on 401 the interactive
// TOTP login (the jar may be empty, or past its 24h idle timeout).
func Preflight(ctx context.Context, client *api.Client, in io.Reader, out io.Writer) error {
	_, err := client.Resume(ctx)
	if err == nil {
		return nil
	}
	if !errors.Is(err, api.ErrUnauthorized) {
		return fmt.Errorf("cannot reach %s: %w", client.BaseURL().Host, err)
	}
	fmt.Fprintf(out, "Logging in to %s\n", client.BaseURL().Host)
	if err := EnsureLogin(ctx, client.Login, &stdinPrompter{in: bufio.NewReader(in), out: out}, out); err != nil {
		return err
	}
	if _, err := client.Resume(ctx); err != nil {
		return fmt.Errorf("login succeeded but the session does not stick: %w", err)
	}
	return nil
}

// stdinPrompter reads the TOTP code from the terminal. The code is a
// 6-digit one-time value — no masking needed, same as the web chatbox.
type stdinPrompter struct {
	in  *bufio.Reader
	out io.Writer
}

func (p *stdinPrompter) TOTP() (string, error) {
	fmt.Fprint(p.out, "TOTP code: ")
	line, err := p.in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
