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

func TestLoadOldConfigWithoutTelemetryTable(t *testing.T) {
	cfg, err := Load(write(t, `
instance_url = "https://pito.example.com"
sounds = false
`))
	if err != nil {
		t.Fatalf("config file without [telemetry] must still load: %v", err)
	}
	want := Telemetry{Enabled: true, Endpoint: "", Key: ""}
	if cfg.Telemetry != want {
		t.Errorf("Telemetry = %+v, want %+v", cfg.Telemetry, want)
	}
	if cfg.Telemetry.Active() {
		t.Error("Active() must be false with no endpoint/key configured")
	}
}

func TestLoadReadsTelemetryValues(t *testing.T) {
	path := write(t, `
instance_url = "https://pito.example.com"

[telemetry]
endpoint = "https://collector.example"
key = "abc123"
enabled = true
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Telemetry.Endpoint != "https://collector.example" {
		t.Errorf("Telemetry.Endpoint = %q", cfg.Telemetry.Endpoint)
	}
	if cfg.Telemetry.Key != "abc123" {
		t.Errorf("Telemetry.Key = %q", cfg.Telemetry.Key)
	}
	if !cfg.Telemetry.Enabled {
		t.Error("Telemetry.Enabled = false, want true")
	}
	if !cfg.Telemetry.Active() {
		t.Error("Active() must be true with endpoint, key, and enabled all set")
	}
}

func TestTelemetryActive(t *testing.T) {
	cases := []struct {
		name string
		t    Telemetry
		want bool
	}{
		{"zero value", Telemetry{}, false},
		{"endpoint only", Telemetry{Endpoint: "https://collector.example"}, false},
		{"key only", Telemetry{Key: "abc123"}, false},
		{"both but disabled", Telemetry{Endpoint: "https://collector.example", Key: "abc123", Enabled: false}, false},
		{"both and enabled", Telemetry{Endpoint: "https://collector.example", Key: "abc123", Enabled: true}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.t.Active(); got != c.want {
				t.Errorf("Active() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestSaveTelemetryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	saved := Config{
		InstanceURL: "https://dev.pitomd.com",
		Telemetry:   Telemetry{Endpoint: "https://collector.example", Key: "k", Enabled: true},
	}
	if err := Save(path, saved); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Telemetry != saved.Telemetry {
		t.Errorf("Telemetry round trip = %+v, want %+v", loaded.Telemetry, saved.Telemetry)
	}
}

func TestSaveEmptyTelemetryWritesCommentedStub(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, Config{InstanceURL: "https://dev.pitomd.com"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "# [telemetry]") {
		t.Error("saved config with no endpoint/key must keep the [telemetry] table commented out")
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Telemetry{Enabled: true}
	if loaded.Telemetry != want {
		t.Errorf("Telemetry = %+v, want default %+v", loaded.Telemetry, want)
	}
}

// The [fx] table: absent → defaults; partial → key-by-key overlay;
// changed → written as a real table; default → the commented stub.
func TestFxDefaultsAndPartialOverlay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fx != defaultFx() {
		t.Errorf("missing file Fx = %+v, want defaults %+v", cfg.Fx, defaultFx())
	}
	if err := os.WriteFile(path, []byte("instance_url = \"https://dev.pitomd.com\"\n[fx]\nidle_fps = 4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := defaultFx()
	want.IdleFPS = 4
	if cfg.Fx != want {
		t.Errorf("partial [fx] table = %+v, want overlay %+v", cfg.Fx, want)
	}
}

func TestSaveFxRoundTripAndCommentedStub(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	// Default fx: the saved file keeps the table as a commented stub.
	if err := Save(path, Config{InstanceURL: "https://dev.pitomd.com", Fx: defaultFx()}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "# [fx]") {
		t.Error("a default fx table must stay a commented stub")
	}
	// Changed fx: written for real, and round-trips.
	changed := Fx{Sky: false, PauseOnBlur: true, IdleGraceSeconds: 60, IdleFPS: 12, DeepIdleMinutes: 0}
	if err := Save(path, Config{InstanceURL: "https://dev.pitomd.com", Fx: changed}); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Fx != changed {
		t.Errorf("Fx round trip = %+v, want %+v", loaded.Fx, changed)
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
