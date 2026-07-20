package app

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gmrdad82/pito-tui/internal/config"
)

// ShowConfig prints the effective configuration — `pito-tui config`.
func ShowConfig(out io.Writer, path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	server := cfg.InstanceURL
	if server == "" {
		server = "(not set — pito-tui config server=https://pito.example.com)"
	}
	fmt.Fprintf(out, "server  %s\n", server)
	fmt.Fprintf(out, "sounds  %v\n", cfg.Sounds)
	if cfg.Conversation != "" {
		fmt.Fprintf(out, "conversation  %s\n", cfg.Conversation)
	}
	fmt.Fprintf(out, "fx      sky=%v pause_on_blur=%v idle_grace_seconds=%d idle_fps=%d deep_idle_minutes=%d\n",
		cfg.Fx.Sky, cfg.Fx.PauseOnBlur, cfg.Fx.IdleGraceSeconds, cfg.Fx.IdleFPS, cfg.Fx.DeepIdleMinutes)
	fmt.Fprintf(out, "file    %s\n", path)
	if !config.Exists(path) {
		fmt.Fprintln(out, "        (not written yet — first run or a `config server=…` will create it)")
	}
	return nil
}

// parseOnOff maps the config surface's boolean vocabulary; the key name
// only decorates the error.
func parseOnOff(key, value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "yes":
		return true, nil
	case "off", "false", "no":
		return false, nil
	}
	return false, fmt.Errorf("config: %s takes on|off, got %q", key, value)
}

// parseBoundedInt parses an integer config value inside [min, max] —
// the fx table's numeric knobs all validate here so a typo'd value
// errors at the command instead of surfacing as odd runtime behavior.
func parseBoundedInt(key, value string, min, max int) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("config: %s takes %d..%d, got %q", key, min, max, value)
	}
	return n, nil
}

// SetConfig applies key=value pairs and persists them —
// `pito-tui config server=https://pito.example.com sounds=off`.
// Accepted keys: server (aliases: host, instance), sounds, conversation,
// and the [fx] table's fx.sky, fx.pause_on_blur, fx.idle_grace_seconds,
// fx.idle_fps, fx.deep_idle_minutes.
func SetConfig(out io.Writer, path string, pairs []string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			return fmt.Errorf("config: %q is not key=value (try server=https://pito.example.com)", pair)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		switch key {
		case "server", "host", "instance", "instance_url":
			normalized, err := config.NormalizeInstanceURL(value)
			if err != nil {
				return err
			}
			cfg.InstanceURL = normalized
		case "sounds":
			on, err := parseOnOff(key, value)
			if err != nil {
				return err
			}
			cfg.Sounds = on
		case "conversation":
			cfg.Conversation = strings.TrimSpace(value)
		case "fx.sky":
			on, err := parseOnOff(key, value)
			if err != nil {
				return err
			}
			cfg.Fx.Sky = on
		case "fx.pause_on_blur":
			on, err := parseOnOff(key, value)
			if err != nil {
				return err
			}
			cfg.Fx.PauseOnBlur = on
		case "fx.idle_grace_seconds":
			n, err := parseBoundedInt(key, value, 0, 3600)
			if err != nil {
				return err
			}
			cfg.Fx.IdleGraceSeconds = n
		case "fx.idle_fps":
			n, err := parseBoundedInt(key, value, 1, 60)
			if err != nil {
				return err
			}
			cfg.Fx.IdleFPS = n
		case "fx.deep_idle_minutes":
			// 0 = never deep-idle while focused.
			n, err := parseBoundedInt(key, value, 0, 1440)
			if err != nil {
				return err
			}
			cfg.Fx.DeepIdleMinutes = n
		default:
			return fmt.Errorf("config: unknown key %q (server, sounds, conversation, fx.sky, fx.pause_on_blur, fx.idle_grace_seconds, fx.idle_fps, fx.deep_idle_minutes)", key)
		}
	}
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	return ShowConfig(out, path)
}
