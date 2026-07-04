package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"
)

// PersistentJar wraps net/http/cookiejar with a write-through JSON file so
// the pito session cookie survives restarts. The stdlib jar keeps all the
// matching semantics (domain, path, Secure); this wrapper only owns the
// flat file. The Rails session cookie has no Expires — persisting it is
// the point: it stays valid until the server 401s (24h idle timeout).
type PersistentJar struct {
	mu      sync.Mutex
	path    string
	inner   *cookiejar.Jar
	entries map[string]storedCookie // name + "\x00" + host
}

type storedCookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	URL      string    `json:"url"` // request URL it was set against — the replay key
	Expires  time.Time `json:"expires,omitzero"`
	MaxAge   int       `json:"max_age,omitempty"`
	Secure   bool      `json:"secure,omitempty"`
	HTTPOnly bool      `json:"http_only,omitempty"`
	Path     string    `json:"path,omitempty"`
	Domain   string    `json:"domain,omitempty"`
}

// LoadJar reads the jar file at path (missing file → empty jar) and replays
// each entry through a fresh stdlib jar so host-only vs domain cookies are
// re-derived exactly as when they were first set.
func LoadJar(path string) (*PersistentJar, error) {
	inner, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, err
	}
	j := &PersistentJar{path: path, inner: inner, entries: map[string]storedCookie{}}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return j, nil
		}
		return nil, fmt.Errorf("api: reading cookie jar %s: %w", path, err)
	}
	var stored []storedCookie
	if err := json.Unmarshal(raw, &stored); err != nil {
		// A corrupt jar must never brick the client — start clean; the
		// next login rewrites it.
		return j, nil
	}
	now := time.Now()
	for _, e := range stored {
		if !e.Expires.IsZero() && e.Expires.Before(now) {
			continue // expired persistent cookie
		}
		u, err := url.Parse(e.URL)
		if err != nil || u.Host == "" {
			continue
		}
		j.inner.SetCookies(u, []*http.Cookie{e.toHTTP()})
		j.entries[e.Name+"\x00"+u.Host] = e
	}
	return j, nil
}

func (j *PersistentJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.inner.SetCookies(u, cookies)
	for _, c := range cookies {
		key := c.Name + "\x00" + u.Host
		if c.MaxAge < 0 || (!c.Expires.IsZero() && c.Expires.Before(time.Now())) {
			delete(j.entries, key) // deletion cookie
			continue
		}
		j.entries[key] = fromHTTP(c, u)
	}
	j.saveLocked()
}

func (j *PersistentJar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.inner.Cookies(u)
}

// saveLocked writes the jar atomically (tmp file + rename) with 0600 —
// the session cookie is a credential.
func (j *PersistentJar) saveLocked() {
	stored := make([]storedCookie, 0, len(j.entries))
	for _, e := range j.entries {
		stored = append(stored, e)
	}
	raw, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return
	}
	dir := filepath.Dir(j.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	tmp := j.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, j.path)
}

func fromHTTP(c *http.Cookie, u *url.URL) storedCookie {
	return storedCookie{
		Name:     c.Name,
		Value:    c.Value,
		URL:      u.Scheme + "://" + u.Host + "/",
		Expires:  c.Expires,
		Secure:   c.Secure,
		HTTPOnly: c.HttpOnly,
		Path:     c.Path,
		Domain:   c.Domain,
	}
}

func (e storedCookie) toHTTP() *http.Cookie {
	return &http.Cookie{
		Name:     e.Name,
		Value:    e.Value,
		Expires:  e.Expires,
		Secure:   e.Secure,
		HttpOnly: e.HTTPOnly,
		Path:     e.Path,
		Domain:   e.Domain,
	}
}
