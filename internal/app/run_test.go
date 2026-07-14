package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// newTestClient wires a real api.Client at an httptest server.
func newTestClient(t *testing.T, handler http.Handler) (*api.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := api.New(srv.URL, filepath.Join(t.TempDir(), "cookies.json"))
	if err != nil {
		t.Fatal(err)
	}
	return client, srv
}

func TestPreflightPassesWithValidSession(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"recent":[],"older":[]}`))
	})
	client, _ := newTestClient(t, mux)

	authed, err := Preflight(t.Context(), client)
	if err != nil || !authed {
		t.Fatalf("Preflight = %v, %v; want authed and no error", authed, err)
	}
}

func TestPreflightUnauthenticatedStartsInAppLogin(t *testing.T) {
	// 401 is NOT an error: the TUI opens unauthenticated and the user
	// types /login <code> — the server grammar owns the login flow.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	client, _ := newTestClient(t, mux)

	authed, err := Preflight(t.Context(), client)
	if err != nil {
		t.Fatalf("Preflight = %v; a 401 must not be an error", err)
	}
	if authed {
		t.Error("authed = true on a 401")
	}
}

func TestPreflightServerAnswered5xx(t *testing.T) {
	// A proxy/tunnel fronting a dead PITO: the request completes, badly.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	client, _ := newTestClient(t, mux)

	_, err := Preflight(t.Context(), client)
	if err == nil || !strings.Contains(err.Error(), "answered 502") ||
		!strings.Contains(err.Error(), "pito logs") {
		t.Errorf("err = %v, want the answered-but-down message", err)
	}
}

func TestPreflightNothingAnswering(t *testing.T) {
	// A dead address: the transport itself fails (connection refused).
	srv := httptest.NewServer(http.NotFoundHandler())
	deadURL := srv.URL
	srv.Close()
	client, err := api.New(deadURL, filepath.Join(t.TempDir(), "cookies.json"))
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}

	_, err = Preflight(t.Context(), client)
	if err == nil || !strings.Contains(err.Error(), "no answer from the server") ||
		!strings.Contains(err.Error(), "is it up?") {
		t.Errorf("err = %v, want the nothing-answering message", err)
	}
}

func TestRunStopsGracefullyWithoutServer(t *testing.T) {
	var out strings.Builder
	err := Run(Options{
		ConfigPath: filepath.Join(t.TempDir(), "config.toml"),
		Stdout:     &out,
	})
	if err == nil {
		t.Fatal("no configured server must stop the run")
	}
	msg := err.Error()
	if !strings.Contains(msg, "no PITO instance configured") ||
		!strings.Contains(msg, "pito-tui config server=") {
		t.Errorf("message must explain the way in:\n%s", msg)
	}
	if strings.Contains(msg, "pitomd.com") {
		t.Errorf("the message must not propose any particular install:\n%s", msg)
	}
}
