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

func TestPreflightUnreachableInstance(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	client, _ := newTestClient(t, mux)

	_, err := Preflight(t.Context(), client)
	if err == nil || !strings.Contains(err.Error(), "cannot reach") {
		t.Errorf("err = %v, want a reachability error", err)
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
