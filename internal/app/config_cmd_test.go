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

func TestSetConfigRejectsNonsense(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	for _, pairs := range [][]string{
		{"server"},            // not key=value
		{"volume=11"},         // unknown key
		{"sounds=loud"},       // bad value
		{"server=ftp://nope"}, // bad scheme
	} {
		if err := SetConfig(&strings.Builder{}, path, pairs); err == nil {
			t.Errorf("SetConfig(%v) accepted nonsense", pairs)
		}
	}
	if config.Exists(path) {
		t.Error("failed sets must not write the file")
	}
}
