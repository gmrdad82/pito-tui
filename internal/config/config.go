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
	// Telemetry configures optional AppSignal (OpenTelemetry) reporting.
	// It only ever activates on RELEASE builds with both endpoint and key
	// set (internal/telemetry gates on version.IsRelease()); source builds
	// and unconfigured installs send nothing, ever.
	Telemetry Telemetry `toml:"telemetry"`
	// Fx tunes the ambient effects' activity gating (the star sky, the
	// @ai gradient bars, shiny words). Rendering pauses/throttles when
	// the terminal is unfocused or idle; these knobs say when.
	Fx Fx `toml:"fx"`
}

// Fx is the [fx] table: every rate and gate of the ambient effects'
// activity-gated rendering. Partial tables overlay the defaults key by
// key (TOML decode semantics), so setting just one knob keeps the rest.
type Fx struct {
	// Sky toggles the drifting star sky on blank rows entirely — the
	// first runtime toggle for the eye-candy (no rebuild needed).
	Sky bool `toml:"sky"`
	// PauseOnBlur freezes all effects the instant the terminal reports
	// losing focus (background tab/window); focus-in resumes them
	// exactly where they paused. Terminals that never report focus are
	// unaffected (they fail safe to focused).
	PauseOnBlur bool `toml:"pause_on_blur"`
	// IdleGraceSeconds: how long after the last keystroke/mouse/cable
	// message the effects keep their full 60fps rate.
	IdleGraceSeconds int `toml:"idle_grace_seconds"`
	// IdleFPS: the throttled frame rate once the grace expires — same
	// wall-clock motion, fewer frames. Any activity snaps back to 60.
	IdleFPS int `toml:"idle_fps"`
	// DeepIdleMinutes: after this long with no input and no cable
	// traffic the effects pause entirely even while focused (near-zero
	// CPU). 0 = never pause while focused.
	DeepIdleMinutes int `toml:"deep_idle_minutes"`
}

// defaultFx returns the [fx] defaults — mirrored by ui.NewModel's own
// zero-config values so an absent table and a default table behave
// identically.
func defaultFx() Fx {
	return Fx{Sky: true, PauseOnBlur: true, IdleGraceSeconds: 30, IdleFPS: 8, DeepIdleMinutes: 5}
}

// Telemetry is the [telemetry] table. Zero value = disabled: activation
// needs Endpoint AND Key non-empty AND Enabled true (Enabled is an opt-out
// for a configured install, defaulted true in defaults()).
type Telemetry struct {
	// Endpoint is the AppSignal hosted collector base URL (from the
	// AppSignal UI's OpenTelemetry setup page).
	Endpoint string `toml:"endpoint"`
	// Key is the AppSignal Push API key (App settings → Push & deploy).
	Key string `toml:"key"`
	// Enabled set to false silences telemetry even when configured.
	Enabled bool `toml:"enabled"`
}

// Active reports whether this config asks for telemetry at all — the
// build-type gate (release vs dev) lives in internal/telemetry, not here.
func (t Telemetry) Active() bool {
	return t.Enabled && t.Endpoint != "" && t.Key != ""
}

func defaults() Config {
	// No default instance: pito is self-hosted, and this client must not
	// steer anyone toward any particular install. Unconfigured means
	// unconfigured — the app stops with instructions instead.
	return Config{
		Sounds: true,
		// Opt-out default: harmless while endpoint/key are empty, and a
		// configured install works without also hunting for a switch.
		Telemetry: Telemetry{Enabled: true},
		Fx:        defaultFx(),
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
	telemetry := "# [telemetry]\n# endpoint = \"\"\n# key = \"\"\n# enabled = true"
	if cfg.Telemetry.Endpoint != "" || cfg.Telemetry.Key != "" {
		telemetry = fmt.Sprintf("[telemetry]\nendpoint = %q\nkey = %q\nenabled = %v",
			cfg.Telemetry.Endpoint, cfg.Telemetry.Key, cfg.Telemetry.Enabled)
	}
	fx := "# [fx]\n# sky = true\n# pause_on_blur = true\n# idle_grace_seconds = 30\n# idle_fps = 8\n# deep_idle_minutes = 5"
	if cfg.Fx != defaultFx() {
		fx = fmt.Sprintf("[fx]\nsky = %v\npause_on_blur = %v\nidle_grace_seconds = %d\nidle_fps = %d\ndeep_idle_minutes = %d",
			cfg.Fx.Sky, cfg.Fx.PauseOnBlur, cfg.Fx.IdleGraceSeconds, cfg.Fx.IdleFPS, cfg.Fx.DeepIdleMinutes)
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

# Optional AppSignal error/performance reporting — RELEASE builds only,
# and only when both values below are set (source builds never send).
# endpoint: your hosted collector URL; key: the Push API key — both from
# the AppSignal UI. enabled = false opts out without deleting them.
%s

# Ambient-effect activity gating (`+"`pito-tui config fx.<key>=<value>`"+`):
# sky toggles the star sky; pause_on_blur freezes effects while the
# terminal is unfocused; after idle_grace_seconds without input the
# frame rate throttles to idle_fps (same motion, fewer frames); after
# deep_idle_minutes with no input/cable traffic effects pause entirely
# (0 = never). Any keystroke or server message snaps back instantly.
%s
`, cfg.InstanceURL, cfg.Sounds, conversation, telemetry, fx)
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
