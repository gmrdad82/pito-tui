package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// importRequest is one decoded /games/import POST body.
type importRequest struct {
	IgdbID int    `json:"igdb_id"`
	Title  string `json:"title"`
	UUID   string `json:"uuid"`
}

// importPickerServer wires POST /games/search and POST /games/import for
// the tests below. Every search's raw query lands on queries (nil skips
// recording); search builds the JSON body to answer with (nil defaults to
// an empty-hits envelope). Every import POST decodes onto imports (nil
// skips recording); importStatus controls the reply (0 => 204, matching
// the real server's fire-and-forget contract).
func importPickerServer(t *testing.T, queries chan string, search func(query string) string, imports chan importRequest, importStatus int) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/games/search", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if queries != nil {
			queries <- body.Query
		}
		w.Header().Set("Content-Type", "application/json")
		out := `{"hits":[]}`
		if search != nil {
			out = search(body.Query)
		}
		fmt.Fprint(w, out)
	})
	mux.HandleFunc("/games/import", func(w http.ResponseWriter, r *http.Request) {
		var req importRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if imports != nil {
			imports <- req
		}
		if importStatus != 0 && importStatus != http.StatusNoContent {
			w.WriteHeader(importStatus)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

// searchJSON builds a /games/search response body matching the envelope
// api.SearchIGDB decodes (hits, library_ids, error.message).
func searchJSON(t *testing.T, hits []api.IgdbHit, library []int, errMsg string) string {
	t.Helper()
	payload := map[string]any{"hits": hits}
	if library != nil {
		payload["library_ids"] = library
	}
	if errMsg != "" {
		payload["error"] = map[string]string{"message": errMsg}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestImportTrigger(t *testing.T) {
	cases := []struct {
		text        string
		wantPrefill string
		wantOK      bool
	}{
		{"import", "", true},
		{"import game", "", true},
		{"import Hollow Knight", "Hollow Knight", true},
		{"import game Celeste", "Celeste", true},
		{"/games import", "", true},
		{"/games import Hades", "Hades", true},
		{"import videos", "", false},
		{"import vids", "", false},
		{"show game", "", false},
		{"importantly", "", false}, // prefix boundary: "import" without the space must not match
	}
	for _, c := range cases {
		prefill, ok := importTrigger(c.text)
		if ok != c.wantOK {
			t.Errorf("importTrigger(%q) ok = %v, want %v", c.text, ok, c.wantOK)
			continue
		}
		if ok && prefill != c.wantPrefill {
			t.Errorf("importTrigger(%q) prefill = %q, want %q", c.text, prefill, c.wantPrefill)
		}
	}
}

func TestImportPickerOpenWithPrefillSearchesImmediately(t *testing.T) {
	queries := make(chan string, 4)
	hits := []api.IgdbHit{
		{ID: 1, Name: "Hollow Knight"},
		{ID: 2, Name: "Hollow Knight: Silksong", TypeNote: "(remake)"},
	}
	handler := importPickerServer(t, queries, func(string) string {
		return searchJSON(t, hits, []int{2}, "")
	}, nil, 0)
	m, _ := newTestModel(t, handler, WithConversation("u-1"))
	m = sized(m)

	m.input.SetValue("import Hollow Knight")
	next, cmd := m.Update(key("enter"))
	m = next.(Model)
	if m.mode != modeImport {
		t.Fatalf("import trigger must open the import picker, mode=%v", m.mode)
	}
	if cmd == nil {
		t.Fatal("opening with a prefill must return an immediate-search command")
	}
	m = runCmd(m, cmd)

	select {
	case q := <-queries:
		if q != "Hollow Knight" {
			t.Fatalf("search query = %q, want %q", q, "Hollow Knight")
		}
	default:
		t.Fatal("opening with a prefill must POST /games/search immediately")
	}
	if len(queries) != 0 {
		t.Fatalf("opening with a prefill must fire exactly one search, %d extra queued", len(queries))
	}

	if len(m.importP.hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(m.importP.hits))
	}
	view := ansi.Strip(m.importPickerView())
	for _, want := range []string{"Hollow Knight", "Hollow Knight: Silksong", "(remake)", "in library"} {
		if !strings.Contains(view, want) {
			t.Errorf("picker view missing %q:\n%s", want, view)
		}
	}
}

func TestImportPickerDebounceGenerations(t *testing.T) {
	queries := make(chan string, 4)
	handler := importPickerServer(t, queries, func(string) string {
		return searchJSON(t, nil, nil, "")
	}, nil, 0)
	m, _ := newTestModel(t, handler, WithConversation("u-1"))
	m = sized(m)

	// Bare `import` opens empty — no prefill, no immediate search, gen 0.
	m.input.SetValue("import")
	next, _ := m.Update(key("enter"))
	m = next.(Model)
	if m.mode != modeImport || m.importP.gen != 0 {
		t.Fatalf("bare import must open empty at gen 0, mode=%v gen=%d", m.mode, m.importP.gen)
	}

	// Two quick keystrokes bump the generation twice, each returning its
	// own debounce-tick command (a real tea.Tick — it sleeps the debounce
	// window when actually invoked, so below we drive the ticks it WOULD
	// have produced directly instead of calling the real cmd funcs).
	next, cmd1 := m.Update(key("H"))
	m = next.(Model)
	if m.importP.gen != 1 || cmd1 == nil {
		t.Fatalf("first keystroke must bump gen to 1 and return a debounce cmd, gen=%d", m.importP.gen)
	}
	next, cmd2 := m.Update(key("K"))
	m = next.(Model)
	if m.importP.gen != 2 || cmd2 == nil {
		t.Fatalf("second keystroke must bump gen to 2 and return a debounce cmd, gen=%d", m.importP.gen)
	}
	if m.importP.query != "HK" {
		t.Fatalf("query = %q, want %q", m.importP.query, "HK")
	}

	// Delivering the stale gen-1 tick is a no-op: no search fires.
	m = drive(m, ImportSearchTickMsg{Gen: 1})
	select {
	case q := <-queries:
		t.Fatalf("a stale tick must not fire a search, got query %q", q)
	default:
	}

	// Delivering the current gen-2 tick fires exactly one search.
	next, tickCmd := m.Update(ImportSearchTickMsg{Gen: 2})
	m = next.(Model)
	if tickCmd == nil {
		t.Fatal("the current-gen tick must fire a search")
	}
	m = runCmd(m, tickCmd)
	select {
	case q := <-queries:
		if q != "HK" {
			t.Fatalf("search query = %q, want %q", q, "HK")
		}
	default:
		t.Fatal("the current-gen tick must have POSTed /games/search")
	}
	if len(queries) != 0 {
		t.Fatalf("the gen-2 tick must fire exactly one search, %d extra queued", len(queries))
	}
}

func TestImportPickerStaleSearchReplyDropped(t *testing.T) {
	m, _ := newTestModel(t, importPickerServer(t, nil, nil, nil, 0), WithConversation("u-1"))
	m = sized(m)

	// Open with a prefill (gen -> 1) but never resolve that search — it's
	// still "in flight" from the picker's point of view.
	m.input.SetValue("import Alpha")
	next, _ := m.Update(key("enter"))
	m = next.(Model)
	if m.importP.gen != 1 {
		t.Fatalf("setup: expected gen 1 after the prefill open, got %d", m.importP.gen)
	}

	// The owner keeps typing while that first search is still in flight —
	// this bumps the generation to 2 without ever resolving gen 1.
	next, _ = m.Update(key("z"))
	m = next.(Model)
	if m.importP.gen != 2 {
		t.Fatalf("typing while a search is in flight must bump gen, got %d", m.importP.gen)
	}
	before := append([]api.IgdbHit(nil), m.importP.hits...)

	// gen 1's reply finally lands — it must be dropped, not applied.
	staleHits := []api.IgdbHit{{ID: 99, Name: "Should Not Apply"}}
	next, cmd := m.Update(ImportSearchedMsg{Gen: 1, Res: &api.IgdbSearch{Hits: staleHits}})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("a stale search reply must be a pure no-op")
	}
	if len(m.importP.hits) != len(before) {
		t.Fatalf("a stale search reply must not change hits, got %#v", m.importP.hits)
	}
	for _, h := range m.importP.hits {
		if h.Name == "Should Not Apply" {
			t.Fatal("a stale search reply's hits leaked into the picker")
		}
	}
}

func TestImportPickerEnterImportsAndNotifies(t *testing.T) {
	hits := []api.IgdbHit{{ID: 10, Name: "Hollow Knight"}}
	imports := make(chan importRequest, 2)
	handler := importPickerServer(t, nil, func(string) string {
		return searchJSON(t, hits, nil, "")
	}, imports, 0) // 0 => 204 success
	m, _ := newTestModel(t, handler, WithConversation("import-uuid"))
	m = sized(m)

	m.input.SetValue("import Hollow Knight")
	next, cmd := m.Update(key("enter"))
	m = next.(Model)
	m = runCmd(m, cmd) // the immediate search resolves: one hit, cursor 0

	next, importCmd := m.Update(key("enter"))
	m = next.(Model)
	if m.mode != modeChat {
		t.Fatalf("enter on a hit must return to chat, mode=%v", m.mode)
	}
	if m.importP.query != "" || len(m.importP.hits) != 0 || m.importP.gen != 0 {
		t.Fatalf("enter must clear the picker state, got %#v", m.importP)
	}
	if importCmd == nil {
		t.Fatal("enter on a hit must fire the import POST")
	}
	m = drive(m, importCmd())

	select {
	case got := <-imports:
		if got.IgdbID != 10 || got.Title != "Hollow Knight" || got.UUID != "import-uuid" {
			t.Fatalf("import request = %#v, want id=10 title=Hollow Knight uuid=import-uuid", got)
		}
	default:
		t.Fatal("enter on a hit must POST /games/import")
	}
	if len(m.notices) == 0 || m.notices[len(m.notices)-1] != "importing Hollow Knight — the chat will narrate it" {
		t.Fatalf("a successful import must notice, got %v", m.notices)
	}

	// A failed import notices the error instead.
	next, _ = m.Update(GameImportedMsg{Title: "Celeste", Err: errors.New("boom")})
	m = next.(Model)
	if len(m.notices) == 0 || m.notices[len(m.notices)-1] != "import failed: boom" {
		t.Fatalf("a failed import must notice the error, got %v", m.notices)
	}
}

func TestImportPickerEscReturnsToChatAndClearsState(t *testing.T) {
	hits := []api.IgdbHit{{ID: 1, Name: "Hollow Knight"}}
	handler := importPickerServer(t, nil, func(string) string {
		return searchJSON(t, hits, nil, "")
	}, nil, 0)
	m, _ := newTestModel(t, handler, WithConversation("u-1"))
	m = sized(m)

	m.input.SetValue("import Hollow Knight")
	next, cmd := m.Update(key("enter"))
	m = next.(Model)
	m = runCmd(m, cmd)
	if len(m.importP.hits) != 1 {
		t.Fatalf("setup: expected 1 hit before esc, got %d", len(m.importP.hits))
	}

	next, _ = m.Update(key("esc"))
	m = next.(Model)
	if m.mode != modeChat {
		t.Fatalf("esc must return to chat, mode=%v", m.mode)
	}
	if m.importP.query != "" || len(m.importP.hits) != 0 || m.importP.searched || m.importP.gen != 0 {
		t.Fatalf("esc must clear the picker state, got %#v", m.importP)
	}
}

func TestImportPickerEmptyResultState(t *testing.T) {
	handler := importPickerServer(t, nil, func(string) string {
		return searchJSON(t, nil, nil, "")
	}, nil, 0)
	m, _ := newTestModel(t, handler, WithConversation("u-1"))
	m = sized(m)

	m.input.SetValue("import zzzznothing")
	next, cmd := m.Update(key("enter"))
	m = next.(Model)
	m = runCmd(m, cmd)

	if !m.importP.searched || len(m.importP.hits) != 0 {
		t.Fatalf("setup: expected a completed empty search, got %#v", m.importP)
	}
	view := ansi.Strip(m.importPickerView())
	if !strings.Contains(view, "nothing on IGDB by that name") {
		t.Fatalf("a completed empty search must show the empty-state line:\n%s", view)
	}
}

func TestImportPickerServerErrorEnvelopeRenders(t *testing.T) {
	handler := importPickerServer(t, nil, func(string) string {
		return searchJSON(t, nil, nil, "IGDB credentials missing")
	}, nil, 0)
	m, _ := newTestModel(t, handler, WithConversation("u-1"))
	m = sized(m)

	m.input.SetValue("import Anything")
	next, cmd := m.Update(key("enter"))
	m = next.(Model)
	m = runCmd(m, cmd)

	if m.importP.err != "IGDB credentials missing" {
		t.Fatalf("err = %q, want the server's error.message", m.importP.err)
	}
	view := ansi.Strip(m.importPickerView())
	if !strings.Contains(view, "IGDB credentials missing") {
		t.Fatalf("the server's error envelope must render in the panel, not crash:\n%s", view)
	}
}

// TestImportSearchSendsViewportDerivedLimit pins viewportRows' composition
// at the import-search call site (importSearchCmd, owner 2026-07-15,
// viewport-driven paging): the outgoing POST /games/search limit is
// viewportRows(m.height - 6) — see importSearchCmd's doc comment.
func TestImportSearchSendsViewportDerivedLimit(t *testing.T) {
	var gotLimit string
	mux := http.NewServeMux()
	mux.HandleFunc("/games/search", func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"hits":[]}`)
	})
	m, _ := newTestModel(t, mux, WithConversation("u-1"))
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 30})

	// A prefilled trigger searches immediately (no debounce) — same seam
	// TestImportPickerOpenWithPrefillSearchesImmediately drives.
	m.input.SetValue("import Hollow Knight")
	next, cmd := m.Update(key("enter"))
	m = next.(Model)
	if m.mode != modeImport || cmd == nil {
		t.Fatal("opening with a prefill must return an immediate-search command")
	}
	m = runCmd(m, cmd)

	want := itoa(viewportRows(m.height - 6))
	if gotLimit != want {
		t.Errorf("limit = %q, want %q (viewportRows(%d - 6))", gotLimit, want, m.height)
	}
}
