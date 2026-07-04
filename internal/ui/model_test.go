package ui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/cable"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

var fixedNow = time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

type connectRecorder struct {
	mu    sync.Mutex
	uuids []string
}

func (c *connectRecorder) connect(uuid string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.uuids = append(c.uuids, uuid)
}

func (c *connectRecorder) calls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.uuids...)
}

type soundRecorder struct{ sends, receives int }

func (s *soundRecorder) Send()    { s.sends++ }
func (s *soundRecorder) Receive() { s.receives++ }

func newTestModel(t *testing.T, handler http.Handler, opts ...Option) (Model, *connectRecorder) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := api.New(srv.URL, filepath.Join(t.TempDir(), "cookies.json"))
	if err != nil {
		t.Fatal(err)
	}
	rec := &connectRecorder{}
	opts = append([]Option{WithPlainRender(), WithNow(func() time.Time { return fixedNow })}, opts...)
	m := NewModel(client, rec.connect, opts...)
	return m, rec
}

// drive applies messages, discarding returned commands.
func drive(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

// driveCmd applies one message and returns the model plus its command.
func driveCmd(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func sized(m Model) Model {
	return drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

const chatPageJSON = `{
  "conversation": {"uuid": "u1", "name": "release prep"},
  "events": [
    {"id": 1, "turn_id": 7, "kind": "echo", "payload": {"text": "ping"}, "created_at": "2026-07-04T11:59:00Z"},
    {"id": 2, "turn_id": 7, "kind": "system", "payload": {"text": "pong"}, "created_at": "2026-07-04T11:59:01Z"}
  ]
}`

const resumeJSON = `{
  "recent": [{"uuid": "u1", "title": "release prep", "last_activity_at": "2026-07-04T11:58:00Z"}],
  "older":  [{"uuid": "u2", "title": "thumbnail ideas", "last_activity_at": "2026-06-28T20:30:00Z"}]
}`

func chatServer(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resumeJSON))
	})
	mux.HandleFunc("GET /chat/u1.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatPageJSON))
	})
	return mux
}

func TestPickerSelectionOpensConversation(t *testing.T) {
	m, rec := newTestModel(t, chatServer(t))
	m = sized(m)

	// Init's resume fetch, executed by hand.
	m, cmd := driveCmd(m, nil)
	_ = cmd
	resumeMsg := m.fetchResumeCmd()()
	m = drive(m, resumeMsg)
	if len(m.rows) != 3 { // new + recent + older
		t.Fatalf("rows = %d, want 3", len(m.rows))
	}

	// j moves past "new conversation" to the first resume row; enter opens.
	m = drive(m, key("j"))
	m, cmd = driveCmd(m, key("enter"))
	if m.mode != modeChat || cmd == nil {
		t.Fatal("enter on a resume row must switch to chat and fetch")
	}
	m = drive(m, cmd())

	if m.conv.UUID != "u1" || m.conv.Name != "release prep" {
		t.Errorf("conversation = %+v", m.conv)
	}
	if got := rec.calls(); len(got) != 1 || got[0] != "u1" {
		t.Errorf("connect calls = %v, want [u1]", got)
	}
	if view := m.View(); !strings.Contains(view, "pong") {
		t.Errorf("scrollback missing from view:\n%s", view)
	}
}

func TestNewConversationDeferredUUIDFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"uuid":"fresh"}`))
	})
	mux.HandleFunc("GET /chat/fresh.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"conversation":{"uuid":"fresh","name":""},"events":[]}`))
	})
	m, rec := newTestModel(t, mux, WithNewConversation())
	m = sized(m)

	m.input.SetValue("hello there")
	m, cmd := driveCmd(m, key("enter"))
	if cmd == nil {
		t.Fatal("enter with input must send")
	}
	m, cmd = driveCmd(m, cmd()) // SendResultMsg{CreatedUUID}
	if m.conv.UUID != "fresh" {
		t.Fatalf("conv uuid = %q, want fresh", m.conv.UUID)
	}
	if cmd == nil {
		t.Fatal("created-uuid reply must trigger a scrollback fetch")
	}
	m = drive(m, cmd())
	if got := rec.calls(); len(got) != 1 || got[0] != "fresh" {
		t.Errorf("connect calls = %v, want [fresh]", got)
	}
}

func TestWebOnlyNoticeRendersDim(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"web-only","verb":"/themes"}`))
	})
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m)

	m.input.SetValue("/themes")
	m, cmd := driveCmd(m, key("enter"))
	m = drive(m, cmd())

	if view := m.View(); !strings.Contains(view, "/themes is web-only") {
		t.Errorf("view missing the web-only notice:\n%s", view)
	}
}

func TestPendingSpinnerClearsOnTurnArrival(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":true,"turn_id":9}`))
	})
	sounds := &soundRecorder{}
	m, _ := newTestModel(t, mux, WithConversation("u1"), WithSounds(sounds))
	m = sized(m)

	m.input.SetValue("do the thing")
	m, cmd := driveCmd(m, key("enter"))
	m, spinCmd := driveCmd(m, cmd())
	if !m.pending[9] {
		t.Fatal("turn 9 must be pending after the ack")
	}
	if spinCmd == nil {
		t.Fatal("pending must start the spinner tick loop")
	}
	if !strings.Contains(m.View(), "thinking…") {
		t.Error("view missing the pending spinner line")
	}
	if sounds.sends != 1 {
		t.Errorf("send sound played %d times, want 1", sounds.sends)
	}

	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventAppend,
		Event: api.Event{ID: 10, TurnID: 9, Kind: "echo", Payload: []byte(`{"text":"do the thing"}`)},
	}})
	if len(m.pending) != 0 {
		t.Error("first event of the turn must clear pending")
	}
	if sounds.receives != 1 {
		t.Errorf("receive sound played %d times, want 1", sounds.receives)
	}
	if strings.Contains(m.View(), "thinking…") {
		t.Error("spinner line must disappear with pending")
	}
}

func TestEventReplaceRewritesViewport(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())

	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventAppend,
		Event: api.Event{ID: 3, TurnID: 8, Kind: "confirmation", Payload: []byte(`{"body":"Sure?","reply_handle":"@confirm-1"}`)},
	}})
	if !strings.Contains(m.View(), "Sure?") {
		t.Fatal("appended confirmation missing")
	}
	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventReplace,
		Event: api.Event{ID: 3, TurnID: 8, Kind: "confirmation", Payload: []byte(`{"body":"Sure?","resolved":true,"outcome_text":"Done."}`)},
	}})
	view := m.View()
	if !strings.Contains(view, "Done.") || strings.Contains(view, "@confirm-1") {
		t.Errorf("replace did not rewrite the block:\n%s", view)
	}
}

func TestReconnectTriggersResyncMerge(t *testing.T) {
	var served int
	mux := http.NewServeMux()
	mux.HandleFunc("GET /chat/u1.json", func(w http.ResponseWriter, r *http.Request) {
		served++
		w.Header().Set("Content-Type", "application/json")
		if served == 1 {
			_, _ = w.Write([]byte(chatPageJSON))
			return
		}
		// Second fetch: one event replaced, one appended while offline.
		_, _ = w.Write([]byte(`{
			"conversation": {"uuid": "u1", "name": "release prep"},
			"events": [
				{"id": 1, "turn_id": 7, "kind": "echo", "payload": {"text": "ping"}, "created_at": "2026-07-04T11:59:00Z"},
				{"id": 2, "turn_id": 7, "kind": "system", "payload": {"text": "pong EDITED"}, "created_at": "2026-07-04T11:59:01Z"},
				{"id": 3, "turn_id": 8, "kind": "system", "payload": {"text": "missed while offline"}, "created_at": "2026-07-04T12:00:30Z"}
			]
		}`))
	})
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())

	m, cmd := driveCmd(m, ConnStateMsg{State: cable.StateDisconnected})
	if cmd != nil {
		t.Fatal("disconnect alone must not refetch")
	}
	if !strings.Contains(m.View(), "disconnected") {
		t.Error("banner missing while disconnected")
	}

	m, cmd = driveCmd(m, ConnStateMsg{State: cable.StateConnected})
	if cmd == nil {
		t.Fatal("reconnect must trigger the resync fetch")
	}
	m = drive(m, cmd())

	view := m.View()
	if !strings.Contains(view, "pong EDITED") || !strings.Contains(view, "missed while offline") {
		t.Errorf("resync merge incomplete:\n%s", view)
	}
	if strings.Contains(m.View(), "disconnected — reconnecting") {
		t.Error("banner must clear once connected")
	}
}

func TestSessionExpiredShowsBanner(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /chat/u1.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())

	if !strings.Contains(m.View(), "session expired") {
		t.Errorf("view missing the session-expired banner:\n%s", m.View())
	}
}

func TestScrollKeysAndFollow(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())

	if !m.follow {
		t.Fatal("follow must start true")
	}
	m = drive(m, key("g"))
	if m.follow {
		t.Error("g (top) must break follow")
	}
	m = drive(m, key("G"))
	if !m.follow {
		t.Error("G must re-enable follow")
	}

	// With text in the prompt, letters belong to the input.
	m.input.SetValue("hel")
	m = drive(m, key("j"))
	if got := m.input.Value(); got != "helj" {
		t.Errorf("input = %q — j must type, not scroll, while the prompt has text", got)
	}
}

func TestUnknownStreamTypeIgnored(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())
	before := m.transcript.Len()

	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:  "event.hologram",
		Event: api.Event{ID: 99, TurnID: 50, Kind: "system", Payload: []byte(`{}`)},
	}})
	if m.transcript.Len() != before {
		t.Error("unknown stream type must be ignored")
	}
}

func TestChatKeyBranches(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())

	// Empty enter sends nothing.
	m, cmd := driveCmd(m, key("enter"))
	if cmd != nil {
		t.Error("empty prompt must not send")
	}

	// Half-page scrolls break follow at the top.
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	m = drive(m, tea.KeyMsg{Type: tea.KeyCtrlD})
	m = drive(m, key("j"), key("k"))

	// ctrl-c quits from chat mode.
	_, cmd = driveCmd(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl-c must quit")
	}
}

func TestPickerCursorBounds(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t))
	m = sized(m)
	m = drive(m, m.fetchResumeCmd()())

	m = drive(m, key("k")) // at top already: stays
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
	m = drive(m, key("j"), key("j"), key("j"), key("j")) // past the end: clamps
	if m.cursor != len(m.rows)-1 {
		t.Errorf("cursor = %d, want %d", m.cursor, len(m.rows)-1)
	}

	// Enter on "new conversation" opens an empty chat without fetching.
	m = drive(m, key("k"), key("k"))
	m.cursor = 0
	m, cmd := driveCmd(m, key("enter"))
	if m.mode != modeChat || cmd != nil || m.conv.UUID != "" {
		t.Error("new-conversation entry must open an empty chat, no fetch")
	}

	// ctrl-c also quits from the picker.
	_, cmd = driveCmd(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("ctrl-c must quit from the picker")
	}
}

func TestPickerEnterWithNoRows(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t))
	m = sized(m)
	m, cmd := driveCmd(m, key("enter"))
	if cmd != nil || m.mode != modePicker {
		t.Error("enter before the list loads must be a no-op")
	}
}

func TestResumeErrorShowsMessage(t *testing.T) {
	mux := http.NewServeMux() // no /resume.json route → 404
	m, _ := newTestModel(t, mux)
	m = sized(m)
	m = drive(m, m.fetchResumeCmd()())
	if !strings.Contains(m.View(), "could not load conversations") {
		t.Errorf("view missing the load error:\n%s", m.View())
	}
}

func TestSendFailureBranches(t *testing.T) {
	t.Run("plain failure becomes a notice", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		m, _ := newTestModel(t, mux, WithConversation("u1"))
		m = sized(m)
		m.input.SetValue("hi")
		m, cmd := driveCmd(m, key("enter"))
		m = drive(m, cmd())
		if !strings.Contains(m.View(), "send failed") {
			t.Errorf("view missing the send-failed notice:\n%s", m.View())
		}
	})

	t.Run("401 flips the session-expired banner", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
		m, _ := newTestModel(t, mux, WithConversation("u1"))
		m = sized(m)
		m.input.SetValue("hi")
		m, cmd := driveCmd(m, key("enter"))
		m = drive(m, cmd())
		if !strings.Contains(m.View(), "session expired") {
			t.Errorf("view missing the expiry banner:\n%s", m.View())
		}
	})
}

func TestNoticeCapKeepsLastThree(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	for i := range 5 {
		m.pushNotice(strings.Repeat("x", i+1))
	}
	if len(m.notices) != maxNotices {
		t.Errorf("notices = %d, want %d", len(m.notices), maxNotices)
	}
	if m.notices[0] != "xxx" {
		t.Errorf("oldest kept notice = %q, want the third", m.notices[0])
	}
}

func TestSpinnerTickAfterPendingDrainedStopsLoop(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	m.pending[9] = true
	m, cmd := driveCmd(m, m.spin.Tick())
	if cmd == nil {
		t.Fatal("tick with pending must continue the loop")
	}
	delete(m.pending, 9)
	_, cmd = driveCmd(m, m.spin.Tick())
	if cmd != nil {
		t.Error("tick with pending drained must not re-arm")
	}
}

func TestRelativeTime(t *testing.T) {
	cases := map[string]string{
		"30s": "just now",
		"5m":  "5m ago",
		"3h":  "3h ago",
		"72h": "3d ago",
	}
	for in, want := range cases {
		d, err := time.ParseDuration(in)
		if err != nil {
			t.Fatal(err)
		}
		if got := relativeTime(fixedNow.Add(-d), fixedNow); got != want {
			t.Errorf("relativeTime(-%s) = %q, want %q", in, got, want)
		}
	}
}
