// pito-tui — terminal client for PITO. All logic lives in internal/app;
// this is the flag surface.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/gmrdad82/pito-tui/internal/app"
	"github.com/gmrdad82/pito-tui/internal/version"
)

func main() {
	var (
		instance    = flag.String("instance", "", "PITO instance URL (overrides config.toml)")
		configPath  = flag.String("config", "", "config file path (default ~/.config/pito-tui/config.toml)")
		sounds      = flag.String("sounds", "", "sound cues: on|off (overrides config.toml)")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: pito-tui [flags] [conversation-uuid]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Printf("pito-tui %s (%s %s)\n", version.Version, version.Commit, version.Date)
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
		ConfigPath:   *configPath,
		InstanceURL:  instanceFlag,
		Sounds:       soundsFlag,
		Conversation: flag.Arg(0),
		Stdin:        os.Stdin,
		Stdout:       os.Stdout,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "pito-tui:", err)
		os.Exit(1)
	}
}
