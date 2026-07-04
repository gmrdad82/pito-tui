package api

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func tempJarPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "cookies.json")
}

func TestJarRoundTrip(t *testing.T) {
	path := tempJarPath(t)
	u := mustURL(t, "https://app.example.com/")

	first, err := LoadJar(path)
	if err != nil {
		t.Fatal(err)
	}
	// A Rails-style session cookie: no Expires, HttpOnly, Secure.
	first.SetCookies(u, []*http.Cookie{{
		Name: "pito_session", Value: "s3cr3t", Path: "/", Secure: true, HttpOnly: true,
	}})

	second, err := LoadJar(path)
	if err != nil {
		t.Fatal(err)
	}
	got := second.Cookies(u)
	if len(got) != 1 || got[0].Name != "pito_session" || got[0].Value != "s3cr3t" {
		t.Fatalf("reloaded cookies = %v, want the persisted session cookie", got)
	}
}

func TestJarFileMode(t *testing.T) {
	path := tempJarPath(t)
	j, err := LoadJar(path)
	if err != nil {
		t.Fatal(err)
	}
	j.SetCookies(mustURL(t, "https://app.example.com/"), []*http.Cookie{{Name: "a", Value: "b"}})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("jar file mode = %o, want 600 (it holds a credential)", perm)
	}
}

func TestJarDropsExpiredOnLoad(t *testing.T) {
	path := tempJarPath(t)
	u := mustURL(t, "https://app.example.com/")

	j, err := LoadJar(path)
	if err != nil {
		t.Fatal(err)
	}
	j.SetCookies(u, []*http.Cookie{{
		Name: "stale", Value: "x", Expires: time.Now().Add(30 * time.Millisecond),
	}})
	time.Sleep(50 * time.Millisecond)

	reloaded, err := LoadJar(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Cookies(u); len(got) != 0 {
		t.Errorf("expired cookie survived reload: %v", got)
	}
}

func TestJarDeletionCookieRemovesEntry(t *testing.T) {
	path := tempJarPath(t)
	u := mustURL(t, "https://app.example.com/")

	j, err := LoadJar(path)
	if err != nil {
		t.Fatal(err)
	}
	j.SetCookies(u, []*http.Cookie{{Name: "pito_session", Value: "old"}})
	j.SetCookies(u, []*http.Cookie{{Name: "pito_session", Value: "", MaxAge: -1}})

	reloaded, err := LoadJar(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Cookies(u); len(got) != 0 {
		t.Errorf("deletion cookie must clear the entry, still have %v", got)
	}
}

func TestJarSecureCookieNotSentOverHTTP(t *testing.T) {
	// Documented pitfall: a Secure cookie against an http:// dev instance
	// silently never rides along. The jar must enforce it (stdlib does).
	path := tempJarPath(t)
	j, err := LoadJar(path)
	if err != nil {
		t.Fatal(err)
	}
	j.SetCookies(mustURL(t, "https://dev.example.com/"), []*http.Cookie{{
		Name: "pito_session", Value: "x", Secure: true,
	}})
	if got := j.Cookies(mustURL(t, "http://dev.example.com/")); len(got) != 0 {
		t.Errorf("Secure cookie returned for http://: %v", got)
	}
}

func TestJarCorruptFileStartsClean(t *testing.T) {
	path := tempJarPath(t)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	j, err := LoadJar(path)
	if err != nil {
		t.Fatalf("corrupt jar must not brick the client: %v", err)
	}
	if got := j.Cookies(mustURL(t, "https://app.example.com/")); len(got) != 0 {
		t.Errorf("corrupt jar produced cookies: %v", got)
	}
}
