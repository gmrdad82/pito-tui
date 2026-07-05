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
	time.Local = time.UTC // deterministic HH:MM stamps in golden frames
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
  "conversation": {"uuid": "u1", "title": "release prep", "display_name": "release prep"},
  "events": [
    {"id": 1, "turn_id": 7, "kind": "echo", "payload": {"text": "ping"}, "created_at": "2026-07-04T11:59:00Z"},
    {"id": 2, "turn_id": 7, "kind": "system", "payload": {"text": "pong"}, "created_at": "2026-07-04T11:59:01Z"}
  ]
}`

const resumeJSON = `{
  "recent": [{"uuid": "u1", "title": "release prep", "display_name": "release prep", "last_activity_at": "2026-07-04T11:58:00Z"}],
  "older":  [{"uuid": "u2", "title": "thumbnail ideas", "display_name": "thumbnail ideas", "last_activity_at": "2026-06-28T20:30:00Z"}]
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

	if m.conv.UUID != "u1" || m.conv.Label() != "release prep" {
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
		_, _ = w.Write([]byte(`{"uuid":"fresh","turn_id":7}`))
	})
	mux.HandleFunc("GET /chat/fresh.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"conversation":{"uuid":"fresh","title":""},"events":[]}`))
	})
	m, rec := newTestModel(t, mux, WithNewConversation())
	m = sized(m)

	m.input.SetValue("hello there")
	m, cmd := driveCmd(m, key("enter"))
	if cmd == nil {
		t.Fatal("enter with input must send")
	}
	m, cmd = driveCmd(m, cmd()) // SendResultMsg{CreatedUUID, TurnID}
	if m.conv.UUID != "fresh" {
		t.Fatalf("conv uuid = %q, want fresh", m.conv.UUID)
	}
	if !m.pending[7] {
		t.Error("the creating turn must be pending until its events arrive")
	}
	if cmd == nil {
		t.Fatal("created-uuid reply must trigger a scrollback fetch")
	}
	m = runCmd(m, cmd) // batch: fetch + spinner tick
	if got := rec.calls(); len(got) != 1 || got[0] != "fresh" {
		t.Errorf("connect calls = %v, want [fresh]", got)
	}
	if !m.pending[7] {
		t.Error("an empty fetch must not clear the still-in-flight turn")
	}
}

// runCmd executes a command, expanding batches, and drives every produced
// message into the model (one level — commands returned by Update are not
// re-executed).
func runCmd(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, inner := range msg {
			m = runCmd(m, inner)
		}
		return m
	default:
		next, _ := m.Update(msg)
		return next.(Model)
	}
}

func TestWebOnlyNoticeRendersDim(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"web_only","message":"That command wears a mouse cursor. Wrong outfit for here."}`))
	})
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m)

	m.input.SetValue("/themes")
	m, cmd := driveCmd(m, key("enter"))
	m = drive(m, cmd())

	if view := m.View(); !strings.Contains(view, "wears a mouse cursor") {
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
			"conversation": {"uuid": "u1", "title": "release prep", "display_name": "release prep"},
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

func TestExpiredSessionAsksForInAppLogin(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /chat/u1.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())

	if !strings.Contains(m.View(), "send /login") {
		t.Errorf("view missing the login banner:\n%s", m.View())
	}
}

func TestLoginRequiredFlowEndsWithChat(t *testing.T) {
	// The /login send goes through the normal chat pipeline: the server
	// creates the conversation, mints the cookie, and the banner clears.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "pito_session", Value: "minted", Path: "/"})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"uuid":"fresh","turn_id":9}`))
	})
	mux.HandleFunc("GET /chat/fresh.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"conversation":{"uuid":"fresh","title":""},"events":[]}`))
	})
	m, rec := newTestModel(t, mux, WithLoginRequired())
	m = sized(m)

	if !strings.Contains(m.View(), "send /login") {
		t.Fatalf("unauthenticated start missing the login banner:\n%s", m.View())
	}
	m.input.SetValue("/login 123456")
	m, cmd := driveCmd(m, key("enter"))
	m, cmd = driveCmd(m, cmd()) // SendResultMsg: created + minted
	m = runCmd(m, cmd)          // fetch succeeds — the auth-gated proof
	if strings.Contains(m.View(), "send /login") {
		t.Error("banner must clear once an auth-gated fetch succeeds")
	}
	if got := rec.calls(); len(got) != 1 || got[0] != "fresh" {
		t.Errorf("connect calls = %v, want [fresh]", got)
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

	t.Run("401 asks for in-app login", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
		m, _ := newTestModel(t, mux, WithConversation("u1"))
		m = sized(m)
		m.input.SetValue("hi")
		m, cmd := driveCmd(m, key("enter"))
		m = drive(m, cmd())
		if !strings.Contains(m.View(), "send /login") {
			t.Errorf("view missing the login banner:\n%s", m.View())
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

func TestUnknownConversationFallsBackToPicker(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /chat/gone.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found"}`))
	})
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resumeJSON))
	})
	m, _ := newTestModel(t, mux, WithConversation("gone"))
	m = sized(m)

	m, cmd := driveCmd(m, m.fetchChatCmd("gone", false)())
	if m.mode != modePicker {
		t.Fatal("a 404 conversation must fall back to the picker")
	}
	if cmd == nil {
		t.Fatal("the fallback must fetch the resume list")
	}
	m = drive(m, cmd())
	view := m.View()
	if !strings.Contains(view, "does not exist anymore") || !strings.Contains(view, "release prep") {
		t.Errorf("picker fallback view wrong:\n%s", view)
	}
}

func TestAnonymousSendDoesNotClearLoginBanner(t *testing.T) {
	// api.md: unauthenticated sends are ACCEPTED (echo + error arrive as
	// events) — a 201 is NOT proof of authentication. Only an auth-gated
	// fetch clears the banner.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"uuid":"anon","turn_id":3}`))
	})
	mux.HandleFunc("GET /chat/anon.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized) // still a guest
	})
	m, _ := newTestModel(t, mux, WithLoginRequired())
	m = sized(m)

	m.input.SetValue("ls games")
	m, cmd := driveCmd(m, key("enter"))
	m, cmd = driveCmd(m, cmd())
	m = runCmd(m, cmd)
	if !strings.Contains(m.View(), "send /login") {
		t.Errorf("banner must survive an anonymous send:\n%s", m.View())
	}
}

func TestPickerWindowsLongLists(t *testing.T) {
	rows := `{"recent": [`
	for i := 1; i <= 60; i++ {
		if i > 1 {
			rows += ","
		}
		rows += `{"uuid": "u` + string(rune('0'+i%10)) + `", "title": "Unnamed ` + string(rune('0'+i%10)) + `", "last_activity_at": "2026-07-04T11:00:00Z"}`
	}
	rows += `], "older": []}`
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rows))
	})
	m, _ := newTestModel(t, mux)
	m = sized(m) // 80×24
	m = drive(m, m.fetchResumeCmd()())

	view := m.View()
	if lines := strings.Count(view, "\n"); lines > 24 {
		t.Errorf("picker view has %d lines for a 24-row terminal", lines)
	}
	if !strings.Contains(view, "conversations") {
		t.Error("title must stay visible with long lists")
	}
	if !strings.Contains(view, "start a new conversation") {
		t.Error("cursor row (new conversation) must stay visible at the top")
	}
	if !strings.Contains(view, "more") {
		t.Error("overflow indicator missing")
	}

	// Cursor at the bottom: the tail must scroll into view.
	for range 70 {
		m = drive(m, key("j"))
	}
	view = m.View()
	if !strings.Contains(view, "↑") {
		t.Errorf("scrolled-down view missing the top overflow indicator:\n%s", view)
	}
	if lines := strings.Count(view, "\n"); lines > 24 {
		t.Errorf("scrolled picker view has %d lines", lines)
	}
}

func TestAckArrivingAfterEventsDoesNotStrandTheSpinner(t *testing.T) {
	// Live-observed race: dev dispatches fast enough that the turn's
	// events arrive over the cable BEFORE the HTTP ack returns.
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())

	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventAppend,
		Event: api.Event{ID: 30, TurnID: 12, Kind: "echo", Payload: []byte(`{"text":"hi"}`)},
	}})
	m, cmd := driveCmd(m, SendResultMsg{Res: &api.SendResult{TurnID: 12}})
	if len(m.pending) != 0 {
		t.Error("ack for an already-rendered turn must not pend")
	}
	if cmd != nil {
		t.Error("no spinner loop for an already-rendered turn")
	}
	if strings.Contains(m.View(), "thinking…") {
		t.Errorf("stranded spinner:\n%s", m.View())
	}
}

func TestReplaceClearsPendingToo(t *testing.T) {
	// The thinking resolve arrives as event.replace and is the turn-done
	// signal — pending must not survive it.
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())
	m.pending[7] = true // turn 7's appends were merged via fetch earlier

	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventReplace,
		Event: api.Event{ID: 2, TurnID: 7, Kind: "thinking", Payload: []byte(`{"resolved":true,"elapsed_seconds":0.7}`)},
	}})
	if len(m.pending) != 0 {
		t.Error("event.replace must clear the turn's pending state")
	}
}

type imageRecorder struct {
	shows  int
	clears int
	last   []byte
}

func (r *imageRecorder) Show(data []byte, row, col, cols, rows int) {
	r.shows++
	r.last = data
}
func (r *imageRecorder) Clear() { r.clears++ }

func TestDetailCardThumbnailShowsOnKittyTerminals(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /thumb.jpg", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("jpeg-bytes"))
	})
	rec := &imageRecorder{}
	m, _ := newTestModel(t, mux, WithConversation("u1"), WithImages(rec))
	m = sized(m)

	body := `{"body":"<div><img class=\"pito-channel-tiny-avatar\" src=\"/avatar.jpg\"/><img alt=\"x\" src=\"/thumb.jpg\"/></div>","html":true}`
	m, cmd := driveCmd(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventAppend,
		Event: api.Event{ID: 9, TurnID: 4, Kind: "system", Payload: []byte(body)},
	}})
	if cmd == nil {
		t.Fatal("card with a thumbnail must fetch it")
	}
	_ = drive(m, cmd()) // ImageFetchedMsg → Show
	if rec.shows != 1 || string(rec.last) != "jpeg-bytes" {
		t.Errorf("Show calls = %d, data = %q — thumbnail must pin (avatar skipped)", rec.shows, rec.last)
	}
}

func TestNoImageDisplayMeansNoFetch(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	body := `{"body":"<img src=\"/thumb.jpg\"/>","html":true}`
	_, cmd := driveCmd(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventAppend,
		Event: api.Event{ID: 9, TurnID: 4, Kind: "system", Payload: []byte(body)},
	}})
	if cmd != nil {
		t.Error("plain terminals must not fetch thumbnails")
	}
}

func TestShimmerAnimatesWhileFreshThenSettles(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"), WithTruecolor(true))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())

	body := `{"body":"<span class=\"pito-subject-shimmer\">13</span> games.","html":true}`
	m, cmd := driveCmd(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventAppend,
		Event: api.Event{ID: 40, TurnID: 15, Kind: "system", Payload: []byte(body)},
	}})
	if len(m.shimmer) != 1 {
		t.Fatal("fresh shimmer event must register for animation")
	}
	if cmd == nil {
		t.Fatal("shimmer must start the animation loop")
	}

	// Ticks advance the phase and keep the loop alive while fresh.
	before := m.phase
	m, cmd = driveCmd(m, AnimTickMsg{})
	if m.phase == before || cmd == nil {
		t.Error("tick must advance phase and re-arm")
	}

	// Expire the shimmer: the loop dies.
	m.shimmer[15] = time.Now().Add(-shimmerLife - time.Second)
	m, cmd = driveCmd(m, AnimTickMsg{})
	if len(m.shimmer) != 0 || m.animating {
		t.Error("expired shimmer must settle")
	}
	if cmd != nil {
		t.Error("settled shimmer must not re-arm the loop")
	}
}

func TestNoShimmerTrackingWithoutTruecolor(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	body := `{"body":"<span class=\"pito-subject-shimmer\">13</span>","html":true}`
	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventAppend,
		Event: api.Event{ID: 41, TurnID: 16, Kind: "system", Payload: []byte(body)},
	}})
	if len(m.shimmer) != 0 {
		t.Error("256-color terminals must not run the animation loop")
	}
}
