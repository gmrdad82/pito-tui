package app

import (
	"bufio"
	"net/http"
	"strings"
	"testing"
)

func TestPreflightPassesWithValidSession(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"recent":[],"older":[]}`))
	})
	client, _ := newTestClient(t, mux)

	var out strings.Builder
	if err := Preflight(t.Context(), client, newBufReader(""), &out); err != nil {
		t.Fatalf("Preflight = %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("valid session must be silent, got %q", out.String())
	}
}

func TestPreflightLogsInOn401(t *testing.T) {
	authed := false
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		if !authed {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"recent":[],"older":[]}`))
	})
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, r *http.Request) {
		authed = true
		http.SetCookie(w, &http.Cookie{Name: "pito_session", Value: "ok", Path: "/"})
		w.WriteHeader(http.StatusCreated)
	})
	client, _ := newTestClient(t, mux)

	var out strings.Builder
	err := Preflight(t.Context(), client, newBufReader("123456\n"), &out)
	if err != nil {
		t.Fatalf("Preflight = %v", err)
	}
	if !strings.Contains(out.String(), "TOTP code:") {
		t.Errorf("output %q missing the prompt", out.String())
	}
}

func TestPreflightUnreachableInstance(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	client, _ := newTestClient(t, mux)

	err := Preflight(t.Context(), client, newBufReader(""), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "cannot reach") {
		t.Errorf("err = %v, want a reachability error", err)
	}
}

func TestStdinPrompterTrimsAndReads(t *testing.T) {
	var out strings.Builder
	p := &stdinPrompter{in: newBufReader("  123456  \n"), out: &out}
	code, err := p.TOTP()
	if err != nil || code != "123456" {
		t.Errorf("TOTP = %q, %v", code, err)
	}
}

func TestStdinPrompterEOFWithoutInput(t *testing.T) {
	p := &stdinPrompter{in: newBufReader(""), out: &strings.Builder{}}
	if _, err := p.TOTP(); err == nil {
		t.Error("EOF with no input must error")
	}
}

func newBufReader(s string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(s))
}

func TestPromptInstanceURL(t *testing.T) {
	cases := map[string]struct{ input, want string }{
		"enter keeps default":     {"\n", "https://app.pitomd.com"},
		"bare host gets https":    {"dev.pitomd.com\n", "https://dev.pitomd.com"},
		"full url passes through": {"http://localhost:3000\n", "http://localhost:3000"},
		"trailing slash trimmed":  {"https://pito.example.com/\n", "https://pito.example.com"},
		"nonsense is re-asked":    {"ftp://nope\ndev.pitomd.com\n", "https://dev.pitomd.com"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var out strings.Builder
			got, err := promptInstanceURL(newBufReader(tc.input), &out, "https://app.pitomd.com")
			if err != nil || got != tc.want {
				t.Errorf("promptInstanceURL(%q) = %q, %v; want %q", tc.input, got, err, tc.want)
			}
		})
	}
}

func TestPromptInstanceURLEOF(t *testing.T) {
	if _, err := promptInstanceURL(newBufReader(""), &strings.Builder{}, "x"); err == nil {
		t.Error("EOF must surface an error, not hang or default silently")
	}
}
