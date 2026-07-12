package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// aiPickerServer serves the /settings/ai pair: the GET state (counting
// fetches so refetch behavior is observable) and the PATCH echo,
// recording every write body it receives.
func aiPickerServer(t *testing.T, patches chan map[string]any, fetches *int) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/settings/ai", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPatch {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			patches <- body
			fmt.Fprint(w, `{"provider":"opencode","model":"claude-fable-5","key_present":true,
				"effort":null,"favorites":["opencode/claude-fable-5"],"recents":["opencode/claude-fable-5"]}`)
			return
		}
		*fetches++
		fmt.Fprint(w, `{
			"providers": [
				{"provider": "opencode", "label": "OpenCode Zen", "key_present": true,
				 "reasoning": "effort",
				 "models": [{"id": "claude-fable-5", "pinned": false}, {"id": "claude-sonnet-5", "pinned": false}]},
				{"provider": "anthropic", "label": "Anthropic", "key_present": false,
				 "reasoning": "none", "models": []}
			],
			"active_provider": "opencode",
			"active_model": "claude-sonnet-5",
			"effort": null,
			"favorites": [],
			"recents": ["opencode/claude-sonnet-5"],
			"conversation_models": []
		}`)
	})
	return mux
}

func TestAiPickerTrigger(t *testing.T) {
	for text, want := range map[string]bool{
		"/config ai": true, "/CONFIG AI": true, "  /config   ai  ": true,
		"/config ai api_key=x": false, "/config google": false, "config ai": false,
	} {
		if got := aiPickerTrigger(text); got != want {
			t.Errorf("aiPickerTrigger(%q) = %v, want %v", text, got, want)
		}
	}
}

func openAiPickerForTest(t *testing.T, patches chan map[string]any, fetches *int) Model {
	t.Helper()
	m, _ := newTestModel(t, aiPickerServer(t, patches, fetches), WithConversation("u-1"))
	m = sized(m)
	m.input.SetValue("/config ai")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.mode != modeAiPicker {
		t.Fatal("bare /config ai must open the AI picker")
	}
	return drive(m, m.aiPickerFetchCmd()())
}

func TestAiPickerRendersTheWebAnatomy(t *testing.T) {
	fetches := 0
	m := openAiPickerForTest(t, make(chan map[string]any, 4), &fetches)
	view := ansi.Strip(m.aiPickerView())

	for _, want := range []string{
		"AI Models",                            // pito.palette.ai_picker.title
		"Esc",                                  // right-edge hint
		"Search models",                        // search placeholder
		"effort model default", "enter cycles", // effort row (opencode declares reasoning)
		"Recents",                  // the one non-empty entry group
		"OpenCode Zen", "key ●●●●", // provider header + chip
		"Anthropic", "no key", // keyless provider header + chip
		"+ paste Anthropic API key",             // connect row
		"Models will load once a key is added.", // key_gate copy
		"● claude-sonnet-5",                     // active marker
		// The web footer's words (KbdBare pads the keys themselves).
		"ctrl+f", "favorite", "ctrl+x", "clear key", "select/connect",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("picker view missing %q:\n%s", want, view)
		}
	}
	// Owner ruling: the title row carries NO current-model summary.
	titleRow := strings.SplitN(view, "\n", 2)[0]
	if strings.Contains(titleRow, "claude-sonnet-5") {
		t.Fatalf("title row must not name the active model: %q", titleRow)
	}
	// Conversation/Favorites headers hide when their groups are empty.
	if strings.Contains(view, "Conversation") || strings.Contains(view, "Favorites") {
		t.Fatalf("empty groups must not render headers:\n%s", view)
	}
}

func TestAiPickerNavigationSkipsChromeRows(t *testing.T) {
	fetches := 0
	m := openAiPickerForTest(t, make(chan map[string]any, 4), &fetches)
	rows := m.aiPicker.buildRows()
	if !rows[m.aiPicker.cursor].selectable() {
		t.Fatal("cursor must rest on a selectable row")
	}
	for range rows {
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = next.(Model)
		if r := m.aiPicker.buildRows()[m.aiPicker.cursor]; !r.selectable() {
			t.Fatalf("cursor landed on a %v row", r.kind)
		}
	}
}

func TestAiPickerSelectFavoriteAndEffortPatch(t *testing.T) {
	patches := make(chan map[string]any, 4)
	fetches := 0
	m := openAiPickerForTest(t, patches, &fetches)

	// The cursor opens on the effort row; enter cycles off → low.
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	m = drive(m, cmd())
	if got := <-patches; got["effort"] != "low" || len(got) != 1 {
		t.Fatalf("effort cycle must PATCH {effort: low}, got %v", got)
	}

	// The effort echo rewrote recents (server truth wins) — Down lands
	// on the rebuilt Recents entry, claude-fable-5; select it.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	m = drive(m, cmd())
	if got := <-patches; got["model"] != "claude-fable-5" || got["provider"] != "opencode" {
		t.Fatalf("model select must PATCH provider+model, got %v", got)
	}
	if m.aiPicker.flash != "model saved: opencode/claude-fable-5" {
		t.Fatalf("flash must mirror the web's wording, got %q", m.aiPicker.flash)
	}
	if m.mode != modeAiPicker {
		t.Fatal("a selection keeps the picker open, like the web")
	}

	// ctrl+f favorites the selected model row.
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = next.(Model)
	m = drive(m, cmd())
	if got := <-patches; got["favorite"] != "opencode/claude-fable-5" {
		t.Fatalf("ctrl+f must PATCH the favorite entry, got %v", got)
	}
	// The echo's favorites land in state without a refetch.
	if !contains(m.aiPicker.state.Favorites, "opencode/claude-fable-5") {
		t.Fatalf("patch echo must update favorites, got %v", m.aiPicker.state.Favorites)
	}
}

func TestAiPickerKeyEntryFlow(t *testing.T) {
	patches := make(chan map[string]any, 4)
	fetches := 0
	m := openAiPickerForTest(t, patches, &fetches)
	baseline := fetches

	// Filter down to nothing so navigation math can't matter, then walk
	// to the Anthropic connect row directly.
	rows := m.aiPicker.buildRows()
	for i, row := range rows {
		if row.kind == aiRowConnect {
			m.aiPicker.cursor = i
			break
		}
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.aiPicker.keyProvider != "anthropic" {
		t.Fatalf("connect row must open the key entry, got %q", m.aiPicker.keyProvider)
	}

	// Esc backs out to the list (staged dismiss), not out of the modal.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.mode != modeAiPicker || m.aiPicker.keyProvider != "" {
		t.Fatal("esc inside the key entry must back out to the list only")
	}

	// Reopen, type a key, enter submits {provider, api_key} + refetches.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	for _, r := range "sk-test" {
		next, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(Model)
	}
	view := ansi.Strip(m.aiPickerView())
	if strings.Contains(view, "sk-test") {
		t.Fatal("the key must never echo in clear text")
	}
	if !strings.Contains(view, "•••••••") {
		t.Fatalf("the key entry must mask input:\n%s", view)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	m = drive(m, cmd())
	if got := <-patches; got["api_key"] != "sk-test" || got["provider"] != "anthropic" {
		t.Fatalf("key submit must PATCH provider+api_key, got %v", got)
	}
	m = drive(m, m.aiPickerFetchCmd()())
	if fetches <= baseline {
		t.Fatal("a key save must refetch the state (models appear)")
	}
}

func TestAiPickerFilterAndEscClose(t *testing.T) {
	fetches := 0
	m := openAiPickerForTest(t, make(chan map[string]any, 4), &fetches)

	for _, r := range "fable" {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(Model)
	}
	view := ansi.Strip(m.aiPickerView())
	if !strings.Contains(view, "claude-fable-5") || strings.Contains(view, "● claude-sonnet-5") {
		t.Fatalf("filter must hide non-matching model rows:\n%s", view)
	}
	// Chrome rows survive the filter (web parity: only model rows hide).
	if !strings.Contains(view, "effort") || !strings.Contains(view, "+ paste Anthropic API key") {
		t.Fatalf("effort/connect rows must survive the filter:\n%s", view)
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.mode != modeChat || m.aiPicker.state != nil {
		t.Fatal("esc must close the picker and clear its state")
	}
}
