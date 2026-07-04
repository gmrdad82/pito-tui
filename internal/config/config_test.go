package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("missing file must not error, got %v", err)
	}
	if cfg.InstanceURL != "" {
		t.Errorf("InstanceURL = %q — there must be NO default instance", cfg.InstanceURL)
	}
	if !cfg.Sounds {
		t.Error("Sounds default must be true")
	}
	if cfg.Conversation != "" {
		t.Errorf("Conversation default must be empty, got %q", cfg.Conversation)
	}
}

func TestLoadReadsValues(t *testing.T) {
	path := write(t, `
instance_url = "https://pito.example.com"
sounds = false
conversation = "abc-123"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InstanceURL != "https://pito.example.com" {
		t.Errorf("InstanceURL = %q", cfg.InstanceURL)
	}
	if cfg.Sounds {
		t.Error("Sounds = true, want false")
	}
	if cfg.Conversation != "abc-123" {
		t.Errorf("Conversation = %q", cfg.Conversation)
	}
}

func TestLoadPartialFileKeepsOtherDefaults(t *testing.T) {
	cfg, err := Load(write(t, `instance_url = "http://localhost:3000"`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InstanceURL != "http://localhost:3000" {
		t.Errorf("InstanceURL = %q", cfg.InstanceURL)
	}
	if !cfg.Sounds {
		t.Error("Sounds must stay true when the file omits it")
	}
}

func TestLoadUnknownKeysTolerated(t *testing.T) {
	cfg, err := Load(write(t, `
sounds = false
future_option = "whatever"
`))
	if err != nil {
		t.Fatalf("unknown keys must not error (older binary, newer file): %v", err)
	}
	if cfg.Sounds {
		t.Error("Sounds = true, want false")
	}
}

func TestLoadMalformedTOML(t *testing.T) {
	_, err := Load(write(t, `instance_url = [broken`))
	if err == nil {
		t.Fatal("malformed TOML must error")
	}
}

func TestWithFlagsPrecedence(t *testing.T) {
	file := Config{InstanceURL: "https://file.example", Sounds: true, Conversation: "from-file"}

	t.Run("unset flags keep file values", func(t *testing.T) {
		got := file.WithFlags(nil, nil, "")
		if got != file {
			t.Errorf("got %+v, want %+v", got, file)
		}
	})

	t.Run("set flags override file", func(t *testing.T) {
		instance := "https://flag.example"
		soundsOff := false
		got := file.WithFlags(&instance, &soundsOff, "from-arg")
		if got.InstanceURL != instance {
			t.Errorf("InstanceURL = %q", got.InstanceURL)
		}
		if got.Sounds {
			t.Error("--sounds=off must override sounds=true from the file")
		}
		if got.Conversation != "from-arg" {
			t.Errorf("Conversation = %q", got.Conversation)
		}
	})

	t.Run("empty instance flag does not blank the URL", func(t *testing.T) {
		empty := ""
		got := file.WithFlags(&empty, nil, "")
		if got.InstanceURL != "https://file.example" {
			t.Errorf("InstanceURL = %q", got.InstanceURL)
		}
	})
}

func TestDirAndDefaultPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, err := Dir()
	if err != nil || !strings.HasSuffix(dir, "pito-tui") {
		t.Errorf("Dir = %q, %v", dir, err)
	}
	path, err := DefaultPath()
	if err != nil || !strings.HasSuffix(path, filepath.Join("pito-tui", "config.toml")) {
		t.Errorf("DefaultPath = %q, %v", path, err)
	}
}

func TestSaveRoundTripsAndStaysEditable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	if Exists(path) {
		t.Fatal("Exists must be false before Save")
	}
	saved := Config{InstanceURL: "https://dev.pitomd.com", Sounds: false}
	if err := Save(path, saved); err != nil {
		t.Fatal(err)
	}
	if !Exists(path) {
		t.Fatal("Exists must be true after Save")
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.InstanceURL != saved.InstanceURL || loaded.Sounds != saved.Sounds {
		t.Errorf("round trip = %+v, want %+v", loaded, saved)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "# ") || !strings.Contains(string(raw), "--instance") {
		t.Error("saved config must keep its editing instructions")
	}
}

func TestNormalizeInstanceURL(t *testing.T) {
	good := map[string]string{
		"dev.pitomd.com":            "https://dev.pitomd.com",
		"  https://a.example.com/ ": "https://a.example.com",
		"http://localhost:3000":     "http://localhost:3000",
		"https://x.dev/pito/":       "https://x.dev/pito",
	}
	for in, want := range good {
		if got, err := NormalizeInstanceURL(in); err != nil || got != want {
			t.Errorf("NormalizeInstanceURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "ftp://nope", "https://"} {
		if got, err := NormalizeInstanceURL(bad); err == nil {
			t.Errorf("NormalizeInstanceURL(%q) = %q, want error", bad, got)
		}
	}
}
