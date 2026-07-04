// Package config loads ~/.config/pito-tui/config.toml and merges CLI flags
// over it. Precedence: built-in defaults ← config file ← flags.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

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
	// No default instance: pito is self-hosted, and this client must not
	// steer anyone toward any particular install. Unconfigured means
	// unconfigured — the app stops with instructions instead.
	return Config{
		Sounds: true,
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

// Exists reports whether a config file is present at path.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Save writes a commented config file so the owner can edit the backend by
// hand later — configuring outside the UI is a supported path, not a hack.
func Save(path string, cfg Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: creating %s: %w", dir, err)
	}
	conversation := "# conversation = \"\""
	if cfg.Conversation != "" {
		conversation = fmt.Sprintf("conversation = %q", cfg.Conversation)
	}
	body := fmt.Sprintf(`# pito-tui configuration.

# The PITO backend this client talks to. Change it here, or with
# `+"`pito-tui config server=<url>`"+`; --instance <url> overrides per run.
instance_url = %q

# Send/receive/notify sound cues. --sounds=on|off overrides per run.
sounds = %v

# Optional: a default conversation uuid to open directly (skips the
# picker). The positional CLI argument wins over it.
%s
`, cfg.InstanceURL, cfg.Sounds, conversation)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o600); err != nil {
		return fmt.Errorf("config: writing %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("config: writing %s: %w", path, err)
	}
	return nil
}

// NormalizeInstanceURL turns first-run input into a usable base URL: bare
// hosts get https://, trailing slashes go away, and anything unparseable
// errors instead of surfacing later as a cryptic dial failure.
func NormalizeInstanceURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("config: empty instance URL")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("config: %q is not a usable instance URL (want e.g. https://pito.example.com)", raw)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
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
