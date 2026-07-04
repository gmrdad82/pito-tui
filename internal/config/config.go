// Package config loads ~/.config/pito-tui/config.toml and merges CLI flags
// over it. Precedence: built-in defaults ← config file ← flags.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const DefaultInstanceURL = "https://app.pitomd.com"

type Config struct {
	// InstanceURL is the PITO instance the client talks to. Everything —
	// login, scrollback, cable, sounds — derives from this one URL.
	InstanceURL string `toml:"instance_url"`
	// Sounds toggles send/receive/notify playback.
	Sounds bool `toml:"sounds"`
	// Conversation optionally pins a default conversation uuid so the
	// picker is skipped. The positional CLI argument wins over it.
	Conversation string `toml:"conversation"`
}

func defaults() Config {
	return Config{
		InstanceURL: DefaultInstanceURL,
		Sounds:      true,
	}
}

// Dir returns the pito-tui config directory (~/.config/pito-tui on Linux).
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: resolving user config dir: %w", err)
	}
	return filepath.Join(base, "pito-tui"), nil
}

// DefaultPath returns the config file location inside Dir().
func DefaultPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load reads the TOML file at path over the built-in defaults. A missing
// file is not an error — first runs work with defaults alone. Unknown keys
// are tolerated so older binaries survive newer config files.
func Load(path string) (Config, error) {
	cfg := defaults()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("config: reading %s: %w", path, err)
	}
	if err := toml.Unmarshal(raw, &cfg); err != nil {
		return defaults(), fmt.Errorf("config: parsing %s: %w", path, err)
	}
	return cfg, nil
}

// WithFlags overlays CLI flags. Nil pointers mean "flag not set" so an
// explicit --sounds=off can override sounds = true from the file.
func (c Config) WithFlags(instanceURL *string, sounds *bool, conversation string) Config {
	if instanceURL != nil && *instanceURL != "" {
		c.InstanceURL = *instanceURL
	}
	if sounds != nil {
		c.Sounds = *sounds
	}
	if conversation != "" {
		c.Conversation = conversation
	}
	return c
}
