package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmrdad82/pito-tui/internal/api"
)

type fakePrompter struct {
	codes []string
	calls int
}

func (p *fakePrompter) TOTP() (string, error) {
	if p.calls >= len(p.codes) {
		return "", errors.New("prompter exhausted")
	}
	code := p.codes[p.calls]
	p.calls++
	return code, nil
}

// newTestClient wires a real api.Client at a TOTP-checking httptest server:
// POST /session mints a cookie for the right code, 401s otherwise, and
// GET /chat/x.json requires the cookie — the full 401→relogin→retry shape.
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

func sessionHandler(t *testing.T, goodCode string) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			OTP string `json:"otp"`
		}
		if err := decodeJSON(r, &body); err != nil {
			t.Errorf("bad /session body: %v", err)
		}
		if body.OTP != goodCode {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "pito_session", Value: "minted", Path: "/", HttpOnly: true})
		w.WriteHeader(http.StatusCreated)
	})
	return mux
}

func TestEnsureLoginSuccessFirstTry(t *testing.T) {
	client, _ := newTestClient(t, sessionHandler(t, "123456"))
	prompt := &fakePrompter{codes: []string{"123456"}}
	var out strings.Builder

	if err := EnsureLogin(t.Context(), client.Login, prompt, &out); err != nil {
		t.Fatalf("EnsureLogin = %v", err)
	}
	if prompt.calls != 1 {
		t.Errorf("prompted %d times, want 1", prompt.calls)
	}
}

func TestEnsureLoginRetriesInvalidThenSucceeds(t *testing.T) {
	client, _ := newTestClient(t, sessionHandler(t, "222222"))
	prompt := &fakePrompter{codes: []string{"111111", "222222"}}
	var out strings.Builder

	if err := EnsureLogin(t.Context(), client.Login, prompt, &out); err != nil {
		t.Fatalf("EnsureLogin = %v", err)
	}
	if prompt.calls != 2 {
		t.Errorf("prompted %d times, want 2", prompt.calls)
	}
	if !strings.Contains(out.String(), "Invalid code") {
		t.Errorf("output %q missing the retry notice", out.String())
	}
}

func TestEnsureLoginStopsAfterMaxInvalid(t *testing.T) {
	client, _ := newTestClient(t, sessionHandler(t, "999999"))
	prompt := &fakePrompter{codes: []string{"1", "2", "3", "4"}}
	var out strings.Builder

	err := EnsureLogin(t.Context(), client.Login, prompt, &out)
	if !errors.Is(err, api.ErrInvalidTOTP) {
		t.Fatalf("EnsureLogin = %v, want ErrInvalidTOTP", err)
	}
	if prompt.calls != maxLoginAttempts {
		t.Errorf("prompted %d times, want %d", prompt.calls, maxLoginAttempts)
	}
}

func TestEnsureLoginStopsImmediatelyOnThrottle(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	client, _ := newTestClient(t, mux)
	prompt := &fakePrompter{codes: []string{"111111", "222222"}}
	var out strings.Builder

	err := EnsureLogin(t.Context(), client.Login, prompt, &out)
	if !errors.Is(err, api.ErrThrottled) {
		t.Fatalf("EnsureLogin = %v, want ErrThrottled", err)
	}
	if prompt.calls != 1 {
		t.Errorf("prompted %d times, want 1 — throttled must never retry-loop", prompt.calls)
	}
	if !strings.Contains(out.String(), "throttled") {
		t.Errorf("output %q missing the throttle notice", out.String())
	}
}

func TestEnsureLoginPrompterError(t *testing.T) {
	client, _ := newTestClient(t, sessionHandler(t, "123456"))
	prompt := &fakePrompter{} // exhausted immediately (EOF-style)

	if err := EnsureLogin(t.Context(), client.Login, prompt, &strings.Builder{}); err == nil {
		t.Fatal("prompter error must propagate")
	}
}

// TestReloginAfterExpiry exercises the mainline 24h-idle shape: a fetch
// 401s, EnsureLogin re-mints the cookie, the retried fetch succeeds.
func TestReloginAfterExpiry(t *testing.T) {
	authed := false
	mux := http.NewServeMux()
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, r *http.Request) {
		authed = true
		http.SetCookie(w, &http.Cookie{Name: "pito_session", Value: "fresh", Path: "/"})
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("GET /chat/abc.json", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("pito_session"); err != nil || c.Value != "fresh" || !authed {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"conversation":{"uuid":"abc","name":"n"},"events":[]}`))
	})
	client, _ := newTestClient(t, mux)

	_, err := client.FetchChat(t.Context(), "abc")
	if !errors.Is(err, api.ErrUnauthorized) {
		t.Fatalf("pre-login fetch = %v, want ErrUnauthorized", err)
	}

	prompt := &fakePrompter{codes: []string{"123456"}}
	if err := EnsureLogin(t.Context(), client.Login, prompt, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}

	page, err := client.FetchChat(t.Context(), "abc")
	if err != nil {
		t.Fatalf("post-login fetch = %v", err)
	}
	if page.Conversation.UUID != "abc" {
		t.Errorf("conversation uuid = %q", page.Conversation.UUID)
	}
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
