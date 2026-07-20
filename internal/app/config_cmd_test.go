package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmrdad82/pito-tui/internal/config"
)

func TestShowConfigDefaultsBeforeFirstRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	var out strings.Builder
	if err := ShowConfig(&out, path); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "(not set") {
		t.Errorf("unset server must say so, not suggest an install:\n%s", got)
	}
	if !strings.Contains(got, "not written yet") {
		t.Errorf("output must say the file does not exist yet:\n%s", got)
	}
}

func TestSetConfigUpdatesServerAndSounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	var out strings.Builder
	if err := SetConfig(&out, path, []string{"server=dev.pitomd.com", "sounds=off"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InstanceURL != "https://dev.pitomd.com" {
		t.Errorf("InstanceURL = %q — bare hosts must normalize", cfg.InstanceURL)
	}
	if cfg.Sounds {
		t.Error("sounds=off did not persist")
	}
	if !strings.Contains(out.String(), "server  https://dev.pitomd.com") {
		t.Errorf("confirmation output wrong:\n%s", out.String())
	}
}

func TestSetConfigAliasesAndPreservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := SetConfig(&strings.Builder{}, path, []string{"sounds=off"}); err != nil {
		t.Fatal(err)
	}
	// host= is the alias the owner reaches for; earlier keys must survive.
	if err := SetConfig(&strings.Builder{}, path, []string{"host=pito.example.com"}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load(path)
	if cfg.InstanceURL != "https://pito.example.com" || cfg.Sounds {
		t.Errorf("cfg = %+v, want new host with sounds still off", cfg)
	}
}

// The [fx] table's whole command surface: booleans in the house on/off
// vocabulary, bounded integers, earlier keys preserved, and the show
// line reporting the effective values.
func TestSetConfigFxKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	var out strings.Builder
	if err := SetConfig(&out, path, []string{
		"fx.sky=off", "fx.pause_on_blur=on",
		"fx.idle_grace_seconds=60", "fx.idle_fps=12", "fx.deep_idle_minutes=0",
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	fx := cfg.Fx
	if fx.Sky || !fx.PauseOnBlur || fx.IdleGraceSeconds != 60 || fx.IdleFPS != 12 || fx.DeepIdleMinutes != 0 {
		t.Errorf("fx did not persist: %+v", fx)
	}
	if !strings.Contains(out.String(), "sky=false") || !strings.Contains(out.String(), "idle_fps=12") {
		t.Errorf("confirmation output must show the fx values:\n%s", out.String())
	}
	// A later unrelated set keeps the fx values (the preservation rule
	// every other key already follows).
	if err := SetConfig(&strings.Builder{}, path, []string{"sounds=off"}); err != nil {
		t.Fatal(err)
	}
	cfg, _ = config.Load(path)
	if cfg.Fx != fx {
		t.Errorf("fx = %+v after unrelated set, want %+v preserved", cfg.Fx, fx)
	}
}

func TestSetConfigRejectsNonsense(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	for _, pairs := range [][]string{
		{"server"},            // not key=value
		{"volume=11"},         // unknown key
		{"sounds=loud"},       // bad value
		{"server=ftp://nope"}, // bad scheme
		{"fx.idle_fps=0"},     // below the 1..60 bound
		{"fx.idle_fps=61"},    // above it
		{"fx.sky=maybe"},      // not on|off
		{"fx.deep_idle_minutes=-1"},
	} {
		if err := SetConfig(&strings.Builder{}, path, pairs); err == nil {
			t.Errorf("SetConfig(%v) accepted nonsense", pairs)
		}
	}
	if config.Exists(path) {
		t.Error("failed sets must not write the file")
	}
}
