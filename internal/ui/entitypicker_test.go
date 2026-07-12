package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// entityServer serves two pages of /games/picker.json (the house
// after=/next_cursor convention) plus a /chat sink for the selection send.
func entityServer(t *testing.T, sent chan string) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/games/picker.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after") == "" {
			rows := make([]map[string]any, 0, 50)
			for i := 1; i <= 50; i++ {
				rows = append(rows, map[string]any{"id": i, "title": fmt.Sprintf("Game %02d", i)})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"rows": rows, "next_cursor": "page2"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rows":        []map[string]any{{"id": 99, "title": "Zelda"}},
			"next_cursor": nil,
		})
	})
	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		sent <- body.Input
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"turn_id":7}`)
	})
	return mux
}

func TestEntityPickerTrigger(t *testing.T) {
	for text, want := range map[string]string{
		"show game": "games", "show vids": "videos", "SHOW GAME": "games",
		"show video": "videos", "show game 3": "", "list games": "", "show": "",
	} {
		noun, _, ok := entityPickerTrigger(text)
		if want == "" && ok {
			t.Errorf("%q must not trigger the picker", text)
		}
		if want != "" && noun != want {
			t.Errorf("%q → %q, want %q", text, noun, want)
		}
	}
}

func TestEntityPickerPagesFiltersAndSends(t *testing.T) {
	sent := make(chan string, 1)
	m, _ := newTestModel(t, entityServer(t, sent), WithConversation("u-1"))
	m = sized(m)

	// Bare `show game` opens the picker and fires page 1.
	m.input.SetValue("show game")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.mode != modeEntityPicker {
		t.Fatal("bare show game must open the entity picker")
	}
	_ = cmd // the batch wraps fetch+animate; call the fetch directly
	m = drive(m, m.entityFetchCmd()())
	if len(m.entity.rows) != 50 || m.entity.next != "page2" {
		t.Fatalf("page 1 wrong: %d rows next=%q", len(m.entity.rows), m.entity.next)
	}
	view := ansi.Strip(m.entityPickerView())
	if !strings.Contains(view, "Game 01") || !strings.Contains(view, "Esc") {
		t.Fatalf("picker view missing rows or the Esc chip:\n%s", view)
	}

	// Typing filters client-side and eagerly pulls the remaining pages so
	// the search sees the whole list.
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("an active filter with pages left must fetch eagerly")
	}
	m = drive(m, m.entityFetchCmd()())
	rows := m.entity.visibleRows()
	if len(rows) != 1 || rows[0].Title != "Zelda" {
		t.Fatalf("filter must reach the eagerly-paged rows: %#v", rows)
	}

	// Enter sends `show game <id>` through the real send path. (The
	// overlay never animated open in this test — pos rests at 0 — so the
	// close is immediate and the send cmd comes straight back.)
	next, sendCmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.mode != modeChat {
		t.Fatalf("selection must land back in chat, mode=%v", m.mode)
	}
	if sendCmd == nil {
		t.Fatal("selection must produce the send command")
	}
	m = drive(m, sendCmd())
	select {
	case got := <-sent:
		if got != "show game 99" {
			t.Fatalf("sent %q, want %q", got, "show game 99")
		}
	default:
		t.Fatal("selection must SEND the command (web games_nav submits)")
	}
	if len(m.histEntries) == 0 || m.histEntries[0] != "show game 99" {
		t.Fatalf("the sent selection must enter input history: %#v", m.histEntries)
	}
}

func TestEntityPickerEscReturnsToChat(t *testing.T) {
	m, _ := newTestModel(t, entityServer(t, make(chan string, 1)), WithConversation("u-1"))
	m = sized(m)
	m.input.SetValue("show vid")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.mode != modeEntityPicker || m.entity.noun != "videos" {
		t.Fatalf("show vid must open the videos picker, got %q", m.entity.noun)
	}
	// Deliver the fetch (the fixture has no videos route, so it lands as
	// an error and the panel rests showing it), settle the open spring,
	// then esc home.
	_ = cmd
	m = drive(m, m.entityFetchCmd()())
	m = driveAnim(t, m, 90)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	m = driveAnim(t, m, 90)
	if m.mode != modeChat {
		t.Fatal("esc must return to chat")
	}
	_ = api.PickerRow{}
}
