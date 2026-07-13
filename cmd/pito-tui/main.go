// pito-tui — terminal client for PITO. All logic lives in internal/app;
// this is the flag-and-subcommand surface.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/gmrdad82/pito-tui/internal/app"
	"github.com/gmrdad82/pito-tui/internal/config"
	"github.com/gmrdad82/pito-tui/internal/version"
)

// cliFlags is main's parsed CLI surface, pulled out of main() itself so
// flag parsing is unit-testable (main_test.go) without exec'ing the
// binary or letting a bad flag os.Exit the test process.
type cliFlags struct {
	instance    string
	configPath  string
	sounds      string
	showVersion bool
	tour        bool
	tourAI      bool
	update      bool
	resume      string
	args        []string // positional args left after flag parsing
}

// arg mirrors flag.Arg(i): the i'th positional argument, or "" past the end.
func (f *cliFlags) arg(i int) string {
	if i < len(f.args) {
		return f.args[i]
	}
	return ""
}

// parseFlags registers pito-tui's flags on fs and parses args. fs's own
// ErrorHandling decides what happens on a bad flag: main passes
// flag.CommandLine (ExitOnError), which prints usage and os.Exit(2)s
// inside fs.Parse itself; tests pass a flag.ContinueOnError set to get the
// error back instead.
func parseFlags(fs *flag.FlagSet, args []string) (*cliFlags, error) {
	f := &cliFlags{}
	fs.StringVar(&f.instance, "instance", "", "PITO instance URL for this run only (config wins otherwise)")
	fs.StringVar(&f.configPath, "config", "", "config file path (default ~/.config/pito-tui/config.toml)")
	fs.StringVar(&f.sounds, "sounds", "", "sound cues: on|off (overrides config.toml)")
	fs.BoolVar(&f.showVersion, "version", false, "print version and exit")
	fs.BoolVar(&f.tour, "tour", false, "play a scripted, self-driving demo against a new conversation, then hand back control")
	fs.BoolVar(&f.tourAI, "tour-ai", false, "include an @ai step in --tour (spends the configured AI provider's budget; no-op without --tour)")
	fs.BoolVar(&f.update, "update", false, "self-update: download and install the latest GitHub release, then exit")
	fs.StringVar(&f.resume, "resume", "", "resume an existing conversation by uuid or name, instead of the default fresh chat")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), `usage: pito-tui [flags] [conversation-uuid]
       pito-tui config [key=value ...]   show or update the config
                                         (keys: server, sounds, conversation)
       pito-tui version                  print version and exit
       pito-tui --tour                   play a scripted demo, then hand back control
       pito-tui --resume <uuid-or-name>  resume an existing conversation directly

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	f.args = fs.Args()
	return f, nil
}

func main() {
	f, err := parseFlags(flag.CommandLine, os.Args[1:])
	if err != nil {
		// flag.CommandLine is ExitOnError: in real use fs.Parse already
		// printed usage and exited before this could ever run. Handled
		// anyway so parseFlags stays honest about its signature.
		fail(err)
	}

	if f.update {
		if err := app.Update(os.Stdout, "https://api.github.com", ""); err != nil {
			fail(err)
		}
		return
	}
	if f.showVersion || f.arg(0) == "version" {
		fmt.Printf("pito-tui %s (%s %s)\n", version.Version, version.Commit, version.Date)
		return
	}

	cfgPath := f.configPath
	if cfgPath == "" {
		if cfgPath, err = config.DefaultPath(); err != nil {
			fail(err)
		}
	}

	if f.arg(0) == "config" {
		if pairs := f.args[1:]; len(pairs) > 0 {
			if err := app.SetConfig(os.Stdout, cfgPath, pairs); err != nil {
				fail(err)
			}
			return
		}
		if err := app.ShowConfig(os.Stdout, cfgPath); err != nil {
			fail(err)
		}
		return
	}

	var soundsFlag *bool
	switch f.sounds {
	case "":
	case "on":
		v := true
		soundsFlag = &v
	case "off":
		v := false
		soundsFlag = &v
	default:
		fmt.Fprintln(os.Stderr, "pito-tui: --sounds takes on or off")
		os.Exit(2)
	}
	var instanceFlag *string
	if f.instance != "" {
		instanceFlag = &f.instance
	}

	err = app.Run(app.Options{
		ConfigPath:   cfgPath,
		InstanceURL:  instanceFlag,
		Sounds:       soundsFlag,
		Conversation: f.arg(0),
		Resume:       f.resume,
		Tour:         f.tour,
		TourAI:       f.tourAI,
		Stdout:       os.Stdout,
	})
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "pito-tui:", err)
	os.Exit(1)
}
