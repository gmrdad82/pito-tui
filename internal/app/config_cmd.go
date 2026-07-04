package app

import (
	"fmt"
	"io"
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
	fmt.Fprintf(out, "file    %s\n", path)
	if !config.Exists(path) {
		fmt.Fprintln(out, "        (not written yet — first run or a `config server=…` will create it)")
	}
	return nil
}

// SetConfig applies key=value pairs and persists them —
// `pito-tui config server=https://pito.example.com sounds=off`.
// Accepted keys: server (aliases: host, instance), sounds.
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
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "server", "host", "instance", "instance_url":
			normalized, err := config.NormalizeInstanceURL(value)
			if err != nil {
				return err
			}
			cfg.InstanceURL = normalized
		case "sounds":
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "on", "true", "yes":
				cfg.Sounds = true
			case "off", "false", "no":
				cfg.Sounds = false
			default:
				return fmt.Errorf("config: sounds takes on|off, got %q", value)
			}
		case "conversation":
			cfg.Conversation = strings.TrimSpace(value)
		default:
			return fmt.Errorf("config: unknown key %q (server, sounds, conversation)", key)
		}
	}
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	return ShowConfig(out, path)
}
