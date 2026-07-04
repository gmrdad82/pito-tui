// Package sound plays the instance's send/receive/notify cues through
// whatever player the machine has — paplay first, mpv second, silence
// otherwise. Sounds are decoration: every failure path is a silent no-op,
// never an error surfaced to the UI.
package sound

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Cue names the three sounds; they map to /sounds/<cue>.mp3 on the instance.
type Cue string

const (
	CueSend    Cue = "send"
	CueReceive Cue = "receive"
	CueNotify  Cue = "notify"
)

var cues = []Cue{CueSend, CueReceive, CueNotify}

// execer seams os/exec for tests.
type execer interface {
	LookPath(name string) (string, error)
	Run(name string, args ...string) error
}

type realExec struct{}

func (realExec) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (realExec) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = nil, nil
	return cmd.Run()
}

// players in preference order. paplay ships with pulseaudio/pipewire but
// only decodes mp3 with libsndfile 1.1+ — a nonzero exit demotes to the
// next candidate at runtime rather than trusting LookPath alone. afplay
// is macOS's built-in player, so darwin gets sounds with zero installs.
var players = []struct {
	bin  string
	args func(file string) []string
}{
	{"paplay", func(f string) []string { return []string{f} }},
	{"mpv", func(f string) []string { return []string{"--no-video", "--really-quiet", f} }},
	{"afplay", func(f string) []string { return []string{f} }},
}

type Player struct {
	enabled  bool
	cacheDir string
	fetch    *http.Client
	baseURL  string
	exec     execer

	mu       sync.Mutex
	player   int  // index into players
	disabled bool // demoted past the last candidate
	cached   bool
}

// Option configures a Player.
type Option func(*Player)

func withExec(e execer) Option {
	return func(p *Player) { p.exec = e }
}

// WithCacheDir overrides ~/.cache/pito-tui.
func WithCacheDir(dir string) Option {
	return func(p *Player) { p.cacheDir = dir }
}

// WithHTTPClient overrides the sound-fetching client (cookie jar not
// needed — /sounds/* is public).
func WithHTTPClient(hc *http.Client) Option {
	return func(p *Player) { p.fetch = hc }
}

// New builds a player for the instance. enabled=false (config sounds=off)
// short-circuits everything.
func New(instanceURL string, enabled bool, opts ...Option) *Player {
	p := &Player{
		enabled: enabled,
		baseURL: instanceURL,
		fetch:   http.DefaultClient,
		exec:    realExec{},
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.cacheDir == "" {
		// Namespace the cache per backend host so switching instances
		// (--instance / config edit) never plays another backend's cues.
		base, err := os.UserCacheDir()
		u, uerr := url.Parse(instanceURL)
		if err == nil && uerr == nil && u.Host != "" {
			p.cacheDir = filepath.Join(base, "pito-tui", u.Host)
		} else {
			p.enabled = false
		}
	}
	if p.enabled {
		p.pickPlayer()
	}
	return p
}

func (p *Player) pickPlayer() {
	for i, candidate := range players {
		if _, err := p.exec.LookPath(candidate.bin); err == nil {
			p.player = i
			return
		}
	}
	p.disabled = true
}

// Send, Receive, Notify play their cue asynchronously (fire and forget).
func (p *Player) Send()    { go p.play(CueSend) }
func (p *Player) Receive() { go p.play(CueReceive) }
func (p *Player) Notify()  { go p.play(CueNotify) }

// play resolves the cached file and runs the player, demoting on failure.
// Blocking variant used directly by tests.
func (p *Player) play(cue Cue) {
	if p == nil || !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.disabled {
		return
	}
	if !p.cached {
		if err := p.ensureCacheLocked(context.Background()); err != nil {
			p.disabled = true
			return
		}
		p.cached = true
	}
	file := filepath.Join(p.cacheDir, string(cue)+".mp3")
	for !p.disabled {
		candidate := players[p.player]
		if err := p.exec.Run(candidate.bin, candidate.args(file)...); err == nil {
			return
		}
		// Demote: this player exists but cannot play the file (classic
		// paplay-without-mp3-support). Try the next, then give up quietly.
		p.demoteLocked()
	}
}

func (p *Player) demoteLocked() {
	for next := p.player + 1; next < len(players); next++ {
		if _, err := p.exec.LookPath(players[next].bin); err == nil {
			p.player = next
			return
		}
	}
	p.disabled = true
}

// ensureCacheLocked downloads the three cues once into the cache dir.
// Files already present are kept — the cache never re-fetches.
func (p *Player) ensureCacheLocked(ctx context.Context) error {
	if err := os.MkdirAll(p.cacheDir, 0o755); err != nil {
		return err
	}
	for _, cue := range cues {
		path := filepath.Join(p.cacheDir, string(cue)+".mp3")
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := p.fetchCue(ctx, cue, path); err != nil {
			return err
		}
	}
	return nil
}

func (p *Player) fetchCue(ctx context.Context, cue Cue, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/sounds/"+string(cue)+".mp3", nil)
	if err != nil {
		return err
	}
	resp, err := p.fetch.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &httpError{status: resp.Status}
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.ReadFrom(resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type httpError struct{ status string }

func (e *httpError) Error() string { return "sound: fetch failed: " + e.status }
