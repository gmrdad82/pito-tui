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

func main() {
	var (
		instance    = flag.String("instance", "", "PITO instance URL for this run only (config wins otherwise)")
		configPath  = flag.String("config", "", "config file path (default ~/.config/pito-tui/config.toml)")
		sounds      = flag.String("sounds", "", "sound cues: on|off (overrides config.toml)")
		showVersion = flag.Bool("version", false, "print version and exit")
		tour        = flag.Bool("tour", false, "play a scripted, self-driving demo against a new conversation, then hand back control")
		tourAI      = flag.Bool("tour-ai", false, "include an @ai step in --tour (spends the configured AI provider's budget; no-op without --tour)")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `usage: pito-tui [flags] [conversation-uuid]
       pito-tui config [key=value ...]   show or update the config
                                         (keys: server, sounds, conversation)
       pito-tui version                  print version and exit
       pito-tui --tour                   play a scripted demo, then hand back control

`)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion || flag.Arg(0) == "version" {
		fmt.Printf("pito-tui %s (%s %s)\n", version.Version, version.Commit, version.Date)
		return
	}

	cfgPath := *configPath
	if cfgPath == "" {
		var err error
		if cfgPath, err = config.DefaultPath(); err != nil {
			fail(err)
		}
	}

	if flag.Arg(0) == "config" {
		if pairs := flag.Args()[1:]; len(pairs) > 0 {
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
	switch *sounds {
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
	if *instance != "" {
		instanceFlag = instance
	}

	err := app.Run(app.Options{
		ConfigPath:   cfgPath,
		InstanceURL:  instanceFlag,
		Sounds:       soundsFlag,
		Conversation: flag.Arg(0),
		Tour:         *tour,
		TourAI:       *tourAI,
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
