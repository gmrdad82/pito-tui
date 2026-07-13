package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFootageFolderMissingFileIsEmpty(t *testing.T) {
	if got := LoadFootageFolder(filepath.Join(t.TempDir(), "nope.json")); got != "" {
		t.Errorf("missing file must answer \"\", got %q", got)
	}
}

func TestLoadFootageFolderCorruptFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "footage.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadFootageFolder(path); got != "" {
		t.Errorf("corrupt file must answer \"\", got %q", got)
	}
}

func TestSaveFootageFolderRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "footage.json")
	if err := SaveFootageFolder(path, "/home/owner/Videos/Season 1"); err != nil {
		t.Fatal(err)
	}
	if got := LoadFootageFolder(path); got != "/home/owner/Videos/Season 1" {
		t.Errorf("round trip = %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("footage state file perm = %o, want 0600", perm)
	}
}

func TestSaveFootageFolderOverwritesPreviousValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "footage.json")
	if err := SaveFootageFolder(path, "/first"); err != nil {
		t.Fatal(err)
	}
	if err := SaveFootageFolder(path, "/second"); err != nil {
		t.Fatal(err)
	}
	if got := LoadFootageFolder(path); got != "/second" {
		t.Errorf("LoadFootageFolder = %q, want /second (the newest save)", got)
	}
}

func TestFootagePathLivesUnderConfigDir(t *testing.T) {
	path, err := FootagePath()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir || filepath.Base(path) != "footage.json" {
		t.Errorf("FootagePath = %q, want %s/footage.json", path, dir)
	}
}
