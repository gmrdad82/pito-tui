package sound

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeExec scripts which binaries exist and which invocations fail.
type fakeExec struct {
	mu       sync.Mutex
	binaries map[string]bool
	failures map[string]int // bin → number of times Run fails
	runs     []string
}

func (f *fakeExec) LookPath(name string) (string, error) {
	if f.binaries[name] {
		return "/usr/bin/" + name, nil
	}
	return "", errors.New("not found")
}

func (f *fakeExec) Run(name string, args ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, name+" "+strings.Join(args, " "))
	if f.failures[name] > 0 {
		f.failures[name]--
		return errors.New("exit status 1")
	}
	return nil
}

func (f *fakeExec) ran() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.runs...)
}

// soundServer serves fake mp3 bytes and counts requests.
func soundServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if !strings.HasPrefix(r.URL.Path, "/sounds/") || !strings.HasSuffix(r.URL.Path, ".mp3") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("ID3fake-mp3-bytes"))
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func newPlayer(t *testing.T, e *fakeExec) (*Player, *int) {
	t.Helper()
	srv, requests := soundServer(t)
	p := New(srv.URL, true, withExec(e), WithCacheDir(filepath.Join(t.TempDir(), "cache")))
	return p, requests
}

func TestPrefersPaplay(t *testing.T) {
	e := &fakeExec{binaries: map[string]bool{"paplay": true, "mpv": true}}
	p, _ := newPlayer(t, e)
	p.play(CueSend)

	runs := e.ran()
	if len(runs) != 1 || !strings.HasPrefix(runs[0], "paplay ") {
		t.Errorf("runs = %v, want one paplay invocation", runs)
	}
	if !strings.Contains(runs[0], "send.mp3") {
		t.Errorf("wrong file: %v", runs[0])
	}
}

func TestFallsBackToMpv(t *testing.T) {
	e := &fakeExec{binaries: map[string]bool{"mpv": true}}
	p, _ := newPlayer(t, e)
	p.play(CueReceive)

	runs := e.ran()
	if len(runs) != 1 || !strings.HasPrefix(runs[0], "mpv --no-video") {
		t.Errorf("runs = %v, want one mpv invocation", runs)
	}
}

func TestNoPlayerMeansSilence(t *testing.T) {
	e := &fakeExec{binaries: map[string]bool{}}
	p, requests := newPlayer(t, e)
	p.play(CueSend)

	if len(e.ran()) != 0 {
		t.Errorf("runs = %v, want none", e.ran())
	}
	if *requests != 0 {
		t.Error("no player must mean no fetches either")
	}
}

func TestDemotesPaplayOnFailure(t *testing.T) {
	// paplay exists but cannot decode mp3 (old libsndfile): first Run
	// fails, the player demotes to mpv silently — same call, no error.
	e := &fakeExec{
		binaries: map[string]bool{"paplay": true, "mpv": true},
		failures: map[string]int{"paplay": 1},
	}
	p, _ := newPlayer(t, e)
	p.play(CueSend)

	runs := e.ran()
	if len(runs) != 2 || !strings.HasPrefix(runs[0], "paplay") || !strings.HasPrefix(runs[1], "mpv") {
		t.Errorf("runs = %v, want paplay failure then mpv", runs)
	}

	// Demotion sticks: the next cue goes straight to mpv.
	p.play(CueReceive)
	runs = e.ran()
	if last := runs[len(runs)-1]; !strings.HasPrefix(last, "mpv") {
		t.Errorf("after demotion, run = %q, want mpv", last)
	}
}

func TestAllPlayersFailingDisables(t *testing.T) {
	e := &fakeExec{
		binaries: map[string]bool{"paplay": true, "mpv": true},
		failures: map[string]int{"paplay": 99, "mpv": 99},
	}
	p, _ := newPlayer(t, e)
	p.play(CueSend)
	before := len(e.ran())

	p.play(CueReceive) // disabled: no further invocations
	if len(e.ran()) != before {
		t.Errorf("disabled player still ran: %v", e.ran())
	}
}

func TestCacheFetchesOnceAndReuses(t *testing.T) {
	e := &fakeExec{binaries: map[string]bool{"paplay": true}}
	p, requests := newPlayer(t, e)

	p.play(CueSend)
	if *requests != 3 {
		t.Fatalf("first play fetched %d files, want all 3", *requests)
	}
	for _, cue := range cues {
		if _, err := os.Stat(filepath.Join(p.cacheDir, string(cue)+".mp3")); err != nil {
			t.Errorf("cache missing %s: %v", cue, err)
		}
	}

	p.play(CueReceive)
	if *requests != 3 {
		t.Errorf("second play re-fetched: %d requests", *requests)
	}
}

func TestFetchFailureDisablesQuietly(t *testing.T) {
	e := &fakeExec{binaries: map[string]bool{"paplay": true}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	p := New(srv.URL, true, withExec(e), WithCacheDir(filepath.Join(t.TempDir(), "c")))

	p.play(CueSend)
	if len(e.ran()) != 0 {
		t.Errorf("player ran despite failed fetch: %v", e.ran())
	}
}

func TestDisabledByConfig(t *testing.T) {
	e := &fakeExec{binaries: map[string]bool{"paplay": true}}
	srv, requests := soundServer(t)
	p := New(srv.URL, false, withExec(e), WithCacheDir(t.TempDir()))

	p.play(CueSend)
	if len(e.ran()) != 0 || *requests != 0 {
		t.Error("sounds=off must be a complete no-op")
	}
	_ = srv
}

func TestAsyncCuesEventuallyPlay(t *testing.T) {
	e := &fakeExec{binaries: map[string]bool{"paplay": true}}
	p, _ := newPlayer(t, e)

	p.Send()
	p.Receive()
	p.Notify()

	deadline := time.After(5 * time.Second)
	for {
		if len(e.ran()) == 3 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("async cues played %d/3: %v", len(e.ran()), e.ran())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestNilPlayerIsSafe(t *testing.T) {
	var p *Player
	p.play(CueSend) // must not panic
}

func TestCacheIsNamespacedPerBackend(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	a := New("https://one.example.com", true, withExec(&fakeExec{}))
	b := New("https://two.example.com:8443", true, withExec(&fakeExec{}))
	if a.cacheDir == b.cacheDir {
		t.Errorf("cache dirs collide across backends: %q", a.cacheDir)
	}
	if !strings.HasSuffix(a.cacheDir, filepath.Join("pito-tui", "one.example.com")) {
		t.Errorf("cacheDir = %q, want host-suffixed", a.cacheDir)
	}
}
