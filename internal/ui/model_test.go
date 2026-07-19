package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/cable"
	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

func TestMain(m *testing.M) {
	// Lip Gloss v2 has no global color profile to force anymore (see
	// render.TestMain); goldenFrame strips ANSI explicitly for the golden
	// comparisons instead, and every other assertion here checks plain
	// substrings that survive being wrapped in color codes.
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
	// WithSplash(false): the startup splash (chrome.go/splash.go) defaults
	// on in NewModel — every test wants a clean first frame, so the
	// harness forces it off here, mirroring WithPlainRender's own
	// always-prepended shape. A test that wants to exercise the splash
	// itself passes WithSplash(true) as one of opts below, which — Option
	// application being plain in-order last-writer-wins — overrides this.
	opts = append([]Option{WithPlainRender(), WithNow(func() time.Time { return fixedNow }), WithSplash(false), WithStarSky(false)}, opts...)
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

// driveAnim ticks the animation loop directly, maxTicks times at most,
// until the model reports nothing left animating (m.animating false) —
// standing in for the real tea.Tick loop without its 40ms wall-clock
// wait. A deferred overlay close (spring.go's stepOverlays) surfaces its
// follow-on command — e.g. the picker's fetchChatCmd, fired only once
// the slide-out spring actually settles — bundled alongside that tick's
// recurring animTick; the ONE tick where closeAction flips from set to
// nil is the only one whose command is actually run (via runCmd, which
// also re-drives the bundled animTick — harmless, just one extra real
// tick). Every other tick's command is the bare recurring animTick and
// is safe to discard untouched.
func driveAnim(t *testing.T, m Model, maxTicks int) Model {
	t.Helper()
	for i := 0; i < maxTicks; i++ {
		next, _ := m.Update(AnimTickMsg{})
		m = next.(Model)
		if !m.animating {
			return m
		}
	}
	t.Fatalf("animation did not settle within %d ticks", maxTicks)
	return m
}

// key builds a synthetic key press for tests. Named keys map onto their v2
// Code constant; anything else is treated as printable text (Code set to
// its first rune so msg.String() still matches single-letter cases like
// "j"/"k", and multi-rune labels like "down"/"up" happen to stringify back
// to themselves too — see [tea.Key.String]).
func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	default:
		r := []rune(s)
		return tea.KeyPressMsg{Code: r[0], Text: s}
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

	// j moves past "new conversation" to the first resume row; enter
	// starts the picker's slide-out close — the actual switch to chat
	// (and its scrollback fetch) waits for the close spring to settle
	// (drawer springs purged 2026-07-12: the close is immediate.)
	m = drive(m, key("j"))
	m, cmd = driveCmd(m, key("enter"))
	if m.mode != modeChat || cmd == nil {
		t.Fatal("enter on a resume row must switch straight into chat (springs purged)")
	}
	m = drive(m, cmd()) // the close returned the scrollback fetch — deliver it

	if m.conv.UUID != "u1" || m.conv.Label() != "release prep" {
		t.Errorf("conversation = %+v", m.conv)
	}
	if got := rec.calls(); len(got) != 1 || got[0] != "u1" {
		t.Errorf("connect calls = %v, want [u1]", got)
	}
	if view := m.viewContent(); !strings.Contains(view, "pong") {
		t.Errorf("scrollback missing from view:\n%s", view)
	}
}

func TestResumeHintExposesActiveConversation(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t))
	m = sized(m)
	m, _ = driveCmd(m, nil)
	resumeMsg := m.fetchResumeCmd()()
	m = drive(m, resumeMsg)
	m = drive(m, key("j"))
	m, cmd := driveCmd(m, key("enter"))
	m = drive(m, cmd())

	uuid, label, ok := m.ResumeHint()
	if !ok || uuid != "u1" || label != "release prep" {
		t.Errorf("ResumeHint() = %q, %q, %v, want \"u1\", \"release prep\", true", uuid, label, ok)
	}
}

func TestResumeHintEmptyForFreshUnsentConversation(t *testing.T) {
	// WithNewConversation opens a blank-uuid chat: nothing exists server-
	// side until the first send, so there is nothing to resume yet.
	m, _ := newTestModel(t, http.NewServeMux(), WithNewConversation())

	if _, _, ok := m.ResumeHint(); ok {
		t.Error("ResumeHint() ok = true before any message created the conversation")
	}
}

func TestResumeHintEmptyWhenLoginRequired(t *testing.T) {
	m, _ := newTestModel(t, http.NewServeMux(), WithLoginRequired())

	if _, _, ok := m.ResumeHint(); ok {
		t.Error("ResumeHint() ok = true on an unauthenticated session")
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

	if view := m.viewContent(); !strings.Contains(view, "wears a mouse cursor") {
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
	if !strings.Contains(m.viewContent(), "●") {
		t.Error("view missing the pending comet line (web post_command_dots parity — no caption)")
	}
	if strings.Contains(m.viewContent(), "thinking…") {
		t.Error("the client must not invent a 'thinking…' caption (owner smoke 2026-07-12)")
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
	if strings.Contains(m.viewContent(), "●∙") || strings.Contains(m.viewContent(), "∙●") {
		t.Error("comet line must disappear with pending")
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
	if !strings.Contains(m.viewContent(), "Sure?") {
		t.Fatal("appended confirmation missing")
	}
	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventReplace,
		Event: api.Event{ID: 3, TurnID: 8, Kind: "confirmation", Payload: []byte(`{"body":"Sure?","resolved":true,"outcome_text":"Done."}`)},
	}})
	view := m.viewContent()
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
	if !strings.Contains(m.viewContent(), "disconnected") {
		t.Error("banner missing while disconnected")
	}

	m, cmd = driveCmd(m, ConnStateMsg{State: cable.StateConnected})
	if cmd == nil {
		t.Fatal("reconnect must trigger the resync fetch")
	}
	m = drive(m, cmd())

	view := m.viewContent()
	if !strings.Contains(view, "pong EDITED") || !strings.Contains(view, "missed while offline") {
		t.Errorf("resync merge incomplete:\n%s", view)
	}
	if strings.Contains(m.viewContent(), "disconnected — reconnecting") {
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

	if !strings.Contains(m.viewContent(), "send /login") {
		t.Errorf("view missing the login banner:\n%s", m.viewContent())
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

	if !strings.Contains(m.viewContent(), "send /login") {
		t.Fatalf("unauthenticated start missing the login banner:\n%s", m.viewContent())
	}
	m.input.SetValue("/login 123456")
	m, cmd := driveCmd(m, key("enter"))
	m, cmd = driveCmd(m, cmd()) // SendResultMsg: created + minted
	m = runCmd(m, cmd)          // fetch succeeds — the auth-gated proof
	if strings.Contains(m.viewContent(), "send /login") {
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
	// A tall transcript so the viewport can actually scroll.
	for i := int64(100); i <= 180; i++ {
		m.transcript.Append(api.Event{ID: i, TurnID: i, Kind: "system",
			Payload: []byte(`{"text":"row"}`)})
	}
	m.refreshViewport()
	m.sc.GotoBottom()
	// The vim scroll letters are gone (2.0.0 commands start with them —
	// "glance", "jobs"): every letter types, even at an empty prompt.
	m = drive(m, key("g"))
	if got := m.input.Value(); got != "g" {
		t.Errorf("input = %q — g must type at an empty prompt, not scroll", got)
	}
	if !m.follow {
		t.Error("typing must not touch follow")
	}
	m.input.Reset()

	// Scrolling still breaks/restores follow via the chorded keys.
	m = drive(m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if m.follow {
		t.Error("ctrl+u (up) must break follow")
	}
	for i := 0; i < 50; i++ {
		m = drive(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	}
	if !m.follow {
		t.Error("ctrl+d back to the bottom must restore follow")
	}

	// With text in the prompt, letters belong to the input.
	m.input.SetValue("hel")
	m = drive(m, key("j"))
	if got := m.input.Value(); got != "helj" {
		t.Errorf("input = %q — j must type while the prompt has text", got)
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
	m = drive(m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = drive(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})

	// ctrl-c quits from chat mode.
	_, cmd = driveCmd(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
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

	// ctrl-c quits from the picker.
	_, cmd := driveCmd(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Error("ctrl-c must quit from the picker")
	}

	// Enter on "new conversation" lands straight in an empty chat, no
	// fetch (drawer springs purged 2026-07-12).
	m = drive(m, key("k"), key("k"))
	m.cursor = 0
	m, _ = driveCmd(m, key("enter"))
	if m.mode != modeChat || m.conv.UUID != "" {
		t.Error("new-conversation entry must land in an empty chat, no fetch")
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

// ── n rename / dd delete — the picker's web-parity keys ─────────────────
// (resume_controller.js's "n" and dd chord; conversations_controller.rb's
// PATCH/DELETE /chat/:uuid).

// resumeMux is a fresh mux serving GET /resume.json (resumeJSON: u1
// "release prep" recent, u2 "thumbnail ideas" older), plus whatever the
// caller registers on top — a fresh *http.ServeMux per test, since
// chatServer's is returned as a bare http.Handler and can't be extended.
func resumeMux(t *testing.T) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resumeJSON))
	})
	return mux
}

func TestPickerDeleteChordArmsThenConfirms(t *testing.T) {
	deletes := 0
	mux := resumeMux(t)
	mux.HandleFunc("DELETE /chat/u1", func(w http.ResponseWriter, r *http.Request) {
		deletes++
		w.WriteHeader(http.StatusNoContent)
	})
	m, _ := newTestModel(t, mux)
	m = sized(m)
	m = drive(m, m.fetchResumeCmd()())
	m = drive(m, key("j")) // cursor -> u1 (recent)

	// A lone `d` arms the row without deleting.
	m = drive(m, key("d"))
	if m.pickerDeleteArmed <= 0 {
		t.Fatal("a first d must arm the highlighted row")
	}
	if deletes != 0 {
		t.Fatal("a first d must not delete")
	}

	// A second `d` within the window confirms the delete.
	m, cmd := driveCmd(m, key("d"))
	if m.pickerDeleteArmed != 0 {
		t.Error("the confirming d must disarm")
	}
	if cmd == nil {
		t.Fatal("the second d within the window must fire the delete")
	}
	m = drive(m, cmd())
	if deletes != 1 {
		t.Errorf("DELETE calls = %d, want 1", deletes)
	}
	for _, row := range m.rows {
		if row.uuid == "u1" {
			t.Error("the deleted row must be removed from the picker")
		}
	}
}

func TestPickerDeleteChordAutoDisarmsAfterWindow(t *testing.T) {
	mux := resumeMux(t)
	mux.HandleFunc("DELETE /chat/u1", func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not delete once the arm window has expired")
	})
	m, _ := newTestModel(t, mux)
	m = sized(m)
	m = drive(m, m.fetchResumeCmd()())
	m = drive(m, key("j"))

	m = drive(m, key("d"))
	if m.pickerDeleteArmed <= 0 {
		t.Fatal("d must arm the row")
	}
	m = driveAnim(t, m, int(pickerDeleteArmTicks)+5)
	if m.pickerDeleteArmed != 0 {
		t.Fatal("the arm window must expire on its own, like resume_controller.js's 500ms setTimeout")
	}

	// A `d` after expiry is a FIRST press again, not a confirming second —
	// it re-arms (the DELETE handler above fails the test if this instead
	// fires a delete).
	m = drive(m, key("d"))
	if m.pickerDeleteArmed <= 0 {
		t.Error("d after expiry must re-arm the row")
	}
}

func TestPickerDeleteChordDisarmsOnMove(t *testing.T) {
	mux := resumeMux(t)
	mux.HandleFunc("DELETE /chat/u1", func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not delete after the highlight moved away (web parity)")
	})
	m, _ := newTestModel(t, mux)
	m = sized(m)
	m = drive(m, m.fetchResumeCmd()())
	m = drive(m, key("j"))

	m = drive(m, key("d"))
	if m.pickerDeleteArmed <= 0 {
		t.Fatal("d must arm the row")
	}
	m = drive(m, key("j")) // move onto u2 — disarms
	if m.pickerDeleteArmed != 0 {
		t.Error("moving the highlight must disarm dd")
	}
	m, cmd := driveCmd(m, key("d"))
	if cmd != nil {
		t.Error("d right after a move must re-arm, not delete")
	}
}

func TestPickerDeleteChordDisarmsOnEscWithoutClosing(t *testing.T) {
	mux := resumeMux(t)
	// Opened over an existing conversation, so a bare Esc (nothing armed)
	// would otherwise close the picker back to chat — the armed Esc must
	// NOT take that path.
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m)
	m.mode = modePicker
	m = drive(m, m.fetchResumeCmd()())
	m = drive(m, key("j"))

	m = drive(m, key("d"))
	if m.pickerDeleteArmed <= 0 {
		t.Fatal("d must arm the row")
	}
	m, _ = driveCmd(m, key("esc"))
	if m.pickerDeleteArmed != 0 {
		t.Error("esc must disarm the row (resume_controller.js: armedRow -> disarm)")
	}
	if m.mode != modePicker {
		t.Error("esc while a row is armed must not close the picker")
	}
}

func TestPickerDeleteIgnoredOnNewConversationRow(t *testing.T) {
	mux := resumeMux(t)
	m, _ := newTestModel(t, mux)
	m = sized(m)
	m = drive(m, m.fetchResumeCmd()()) // cursor rests on the "new conversation" row

	m, cmd := driveCmd(m, key("d"))
	if m.pickerDeleteArmed != 0 || cmd != nil {
		t.Error("d on the new-conversation row must be a no-op — there is nothing to delete")
	}
}

func TestPickerDeleteCurrentConversationClearsConv(t *testing.T) {
	mux := resumeMux(t)
	mux.HandleFunc("DELETE /chat/u1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m)
	m.mode = modePicker
	m = drive(m, m.fetchResumeCmd()())
	m = drive(m, key("j")) // cursor -> u1, the conversation the picker is open over

	m = drive(m, key("d"))
	m, cmd := driveCmd(m, key("d"))
	if cmd == nil {
		t.Fatal("second d must fire the delete")
	}
	m = drive(m, cmd())
	if m.conv.UUID != "" {
		t.Error("deleting the active conversation must clear conv — the web's own post-delete rule (navigate home)")
	}
}

func TestPickerDeleteOtherConversationKeepsCurrentConv(t *testing.T) {
	mux := resumeMux(t)
	mux.HandleFunc("DELETE /chat/u2", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m)
	m.mode = modePicker
	m = drive(m, m.fetchResumeCmd()())
	m = drive(m, key("j"), key("j")) // cursor -> u2 (older), not the active conversation

	m = drive(m, key("d"))
	m, cmd := driveCmd(m, key("d"))
	if cmd == nil {
		t.Fatal("second d must fire the delete")
	}
	m = drive(m, cmd())
	if m.conv.UUID != "u1" {
		t.Error("deleting a DIFFERENT row must leave the active conversation untouched")
	}
	if len(m.rows) != 2 { // new + u1 only, u2 removed
		t.Errorf("rows = %d, want 2 (new-conversation + u1)", len(m.rows))
	}
}

func TestPickerRenameSubmitPatchesAndUpdatesRow(t *testing.T) {
	var gotTitle string
	mux := resumeMux(t)
	mux.HandleFunc("PATCH /chat/u1", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Title string `json:"title"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotTitle = body.Title
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"` + body.Title + `"}`))
	})
	m, _ := newTestModel(t, mux)
	m = sized(m)
	m = drive(m, m.fetchResumeCmd()())
	m = drive(m, key("j")) // cursor -> u1

	m, cmd := driveCmd(m, key("n"))
	if m.pickerRenaming != "u1" {
		t.Fatal("n must open inline rename on the highlighted row")
	}
	if cmd == nil {
		t.Error("opening rename must start the cursor blink")
	}
	if got := m.pickerRenameInput.Value(); got != "release prep" {
		t.Errorf("rename input seed = %q, want %q", got, "release prep")
	}

	m.pickerRenameInput.SetValue("renamed chat")
	m, cmd = driveCmd(m, key("enter"))
	if m.pickerRenaming != "" {
		t.Error("enter must close the rename input")
	}
	if m.rows[1].title != "renamed chat" {
		t.Errorf("optimistic row title = %q, want %q", m.rows[1].title, "renamed chat")
	}
	if cmd == nil {
		t.Fatal("enter must PATCH the rename")
	}
	m = drive(m, cmd())
	if gotTitle != "renamed chat" {
		t.Errorf("PATCH title = %q, want %q", gotTitle, "renamed chat")
	}
	if m.rows[1].title != "renamed chat" {
		t.Errorf("row title after the PATCH lands = %q", m.rows[1].title)
	}
}

func TestPickerRenameUpdatesConvDisplayNameWhenCurrent(t *testing.T) {
	mux := resumeMux(t)
	mux.HandleFunc("PATCH /chat/u1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"renamed chat"}`))
	})
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m)
	m.mode = modePicker
	m = drive(m, m.fetchResumeCmd()())
	m = drive(m, key("j"))

	m = drive(m, key("n"))
	m.pickerRenameInput.SetValue("renamed chat")
	m, cmd := driveCmd(m, key("enter"))
	m = drive(m, cmd())
	if m.conv.DisplayName != "renamed chat" {
		t.Errorf("conv.DisplayName = %q, want %q", m.conv.DisplayName, "renamed chat")
	}
}

func TestPickerRenameEscCancelsWithoutPatch(t *testing.T) {
	mux := resumeMux(t)
	mux.HandleFunc("PATCH /chat/u1", func(w http.ResponseWriter, r *http.Request) {
		t.Error("esc must not PATCH")
	})
	m, _ := newTestModel(t, mux)
	m = sized(m)
	m = drive(m, m.fetchResumeCmd()())
	m = drive(m, key("j"))

	m = drive(m, key("n"))
	m.pickerRenameInput.SetValue("scratch")
	m, cmd := driveCmd(m, key("esc"))
	if m.pickerRenaming != "" {
		t.Error("esc must close the rename input")
	}
	if cmd != nil {
		t.Error("esc must not fire a network call")
	}
	if m.rows[1].title != "release prep" {
		t.Errorf("row title must be unchanged by a cancelled rename, got %q", m.rows[1].title)
	}
}

func TestPickerRenameBlankSubmitCancelsWithoutPatch(t *testing.T) {
	mux := resumeMux(t)
	mux.HandleFunc("PATCH /chat/u1", func(w http.ResponseWriter, r *http.Request) {
		t.Error("a blank title must not PATCH")
	})
	m, _ := newTestModel(t, mux)
	m = sized(m)
	m = drive(m, m.fetchResumeCmd()())
	m = drive(m, key("j"))

	m = drive(m, key("n"))
	m.pickerRenameInput.SetValue("   ")
	m, cmd := driveCmd(m, key("enter"))
	if m.pickerRenaming != "" {
		t.Error("enter on a blank value must close the rename input")
	}
	if cmd != nil {
		t.Error("a blank title must not fire a network call (web's #commitRename parity)")
	}
	if m.rows[1].title != "release prep" {
		t.Errorf("row title must be unchanged by a blank rename, got %q", m.rows[1].title)
	}
}

func TestPickerRenameIgnoredOnNewConversationRow(t *testing.T) {
	mux := resumeMux(t)
	m, _ := newTestModel(t, mux)
	m = sized(m)
	m = drive(m, m.fetchResumeCmd()()) // cursor rests on the "new conversation" row

	m, cmd := driveCmd(m, key("n"))
	if m.pickerRenaming != "" || cmd != nil {
		t.Error("n on the new-conversation row must be a no-op — there is nothing to rename")
	}
}

func TestResumeErrorShowsMessage(t *testing.T) {
	mux := http.NewServeMux() // no /resume.json route → 404
	m, _ := newTestModel(t, mux)
	m = sized(m)
	m = drive(m, m.fetchResumeCmd()())
	if !strings.Contains(m.viewContent(), "could not load conversations") {
		t.Errorf("view missing the load error:\n%s", m.viewContent())
	}
}

// TestPickerPaginationAndDuplicateGuard drives the picker's full
// three-page /resume.json flow (tui-needs ask 9a): page 1 arrives
// grouped {recent, older} with a cursor, page 2 arrives flat {rows}
// carrying an ai-flagged row, page 3 arrives flat and exhausted
// (next_cursor null). It pins the same contract
// TestNotificationsPaginationAndDuplicateGuard pins for the
// notifications panel: scrolling to the last loaded row fires exactly
// one fetch, a repeated nudge while it's in flight is a no-op (duplicate
// guard), the loader row shows only while fetching, appended rows render
// with their ✦ badge intact, and exhaustion stops fetching for good.
func TestPickerPaginationAndDuplicateGuard(t *testing.T) {
	var calls []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		after := r.URL.Query().Get("after")
		calls = append(calls, after)
		w.Header().Set("Content-Type", "application/json")
		switch after {
		case "":
			_, _ = w.Write([]byte(`{
				"recent": [{"uuid":"u1","title":"first","display_name":"first","last_activity_at":"2026-07-04T11:58:00Z"}],
				"older":  [{"uuid":"u2","title":"second","display_name":"second","last_activity_at":"2026-06-28T20:30:00Z"}],
				"next_cursor":"c2"
			}`))
		case "c2":
			_, _ = w.Write([]byte(`{"rows":[
				{"uuid":"u3","title":"third","display_name":"third","last_activity_at":"2026-06-20T09:00:00Z","ai":true}
			],"next_cursor":"c3"}`))
		case "c3":
			_, _ = w.Write([]byte(`{"rows":[
				{"uuid":"u4","title":"fourth","display_name":"fourth","last_activity_at":"2026-06-10T09:00:00Z"}
			],"next_cursor":null}`))
		default:
			t.Errorf("unexpected fetch with after=%q", after)
			_, _ = w.Write([]byte(`{"rows":[],"next_cursor":null}`))
		}
	})
	m, _ := newTestModel(t, mux)
	m = sized(m)

	// Page 1 lands (new + first + second = 3 rows, last index 2).
	m = drive(m, m.fetchResumeCmd()())
	if len(m.rows) != 3 || m.pickerNext != "c2" || m.pickerFetching {
		t.Fatalf("page 1 not absorbed: rows=%d next=%q fetching=%v", len(m.rows), m.pickerNext, m.pickerFetching)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %v, want exactly 1 after page 1", calls)
	}

	// "j" to row 1: not the last row yet, must not fetch.
	m, cmd := driveCmd(m, key("j"))
	if m.cursor != 1 || cmd != nil || m.pickerFetching {
		t.Fatalf("moving off the last row must not fetch: cursor=%d fetching=%v cmd=%v", m.cursor, m.pickerFetching, cmd)
	}

	// "j" to row 2 (the last loaded row): fires page 2.
	m, cmd = driveCmd(m, key("j"))
	if m.cursor != 2 || !m.pickerFetching || cmd == nil {
		t.Fatalf("reaching the last row must trigger page 2: cursor=%d fetching=%v cmd=%v", m.cursor, m.pickerFetching, cmd)
	}
	if !strings.Contains(m.viewContent(), loaderDots) {
		t.Errorf("loader row missing mid-fetch:\n%s", m.viewContent())
	}

	// A duplicate nudge while the fetch is in flight must NOT re-request.
	m, cmd = driveCmd(m, key("j"))
	if cmd != nil {
		t.Fatal("a nudge while fetching must be a no-op (duplicate-fetch guard)")
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %v, want still 1 — the guard must have blocked the duplicate", calls)
	}

	// Land page 2 — appends below the existing rows (new + first + second
	// + third = 4 rows, last index 3), carries the ai flag.
	m = drive(m, m.fetchResumeMoreCmd(m.pickerNext)())
	if len(m.rows) != 4 || m.pickerNext != "c3" || m.pickerFetching {
		t.Fatalf("page 2 not absorbed: rows=%d next=%q fetching=%v", len(m.rows), m.pickerNext, m.pickerFetching)
	}
	if !m.rows[3].ai || m.rows[3].title != "third" {
		t.Fatalf("appended row wrong: %+v", m.rows[3])
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want exactly 2 total", calls)
	}
	var thirdLine, secondLine string
	for _, l := range strings.Split(m.viewContent(), "\n") {
		switch {
		case strings.Contains(l, "third"):
			thirdLine = l
		case strings.Contains(l, "second"):
			secondLine = l
		}
	}
	if !strings.Contains(thirdLine, aiSparkle) {
		t.Errorf("appended ai-flagged row missing its badge: %q", thirdLine)
	}
	if strings.Contains(secondLine, aiSparkle) {
		t.Errorf("unflagged row must not show the badge: %q", secondLine)
	}

	// "j" to row 3 (the new last row): fires page 3.
	m, cmd = driveCmd(m, key("j"))
	if m.cursor != 3 || !m.pickerFetching || cmd == nil {
		t.Fatalf("reaching the new last row must trigger page 3: cursor=%d fetching=%v cmd=%v", m.cursor, m.pickerFetching, cmd)
	}
	if !strings.Contains(m.viewContent(), loaderDots) {
		t.Errorf("loader row missing mid-fetch for page 3:\n%s", m.viewContent())
	}

	// Land page 3 — exhausted (next_cursor null).
	m = drive(m, m.fetchResumeMoreCmd(m.pickerNext)())
	if len(m.rows) != 5 || m.pickerNext != "" || m.pickerFetching {
		t.Fatalf("page 3 not absorbed: rows=%d next=%q fetching=%v", len(m.rows), m.pickerNext, m.pickerFetching)
	}
	if len(calls) != 3 {
		t.Fatalf("calls = %v, want exactly 3 total", calls)
	}
	if strings.Contains(m.viewContent(), loaderDots) {
		t.Errorf("exhausted list must never show the loader:\n%s", m.viewContent())
	}

	// Scrolling to the new last row (now exhausted) must not fetch again.
	m, cmd = driveCmd(m, key("j"))
	if cmd != nil || m.pickerFetching {
		t.Fatal("an exhausted list must never fetch again")
	}
	if len(calls) != 3 {
		t.Fatalf("calls = %v, want still 3 after exhaustion", calls)
	}
}

// TestPickerCursorlessServerNeverFetchesMore covers old-server tolerance
// (tui-needs ask 9a): a /resume.json reply with no next_cursor at all —
// today's pre-pagination shape, and what resumeJSON's fixture in
// chatServer serves — must never grow a loader row or schedule a
// follow-on fetch, no matter how far the cursor scrolls. This is the
// byte-identical path TestGoldenPicker also pins.
func TestPickerCursorlessServerNeverFetchesMore(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t))
	m = sized(m)
	m = drive(m, m.fetchResumeCmd()())
	if m.pickerNext != "" {
		t.Fatalf("setup: pickerNext = %q, want \"\" for a cursorless server", m.pickerNext)
	}

	var cmd tea.Cmd
	for range len(m.rows) + 2 { // past the end and back — still nothing to fetch
		m, cmd = driveCmd(m, key("j"))
		if cmd != nil || m.pickerFetching {
			t.Fatalf("cursorless server must never schedule a fetch: cursor=%d fetching=%v", m.cursor, m.pickerFetching)
		}
	}
	if strings.Contains(m.viewContent(), loaderDots) {
		t.Errorf("cursorless server must never show the loader row:\n%s", m.viewContent())
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
		if !strings.Contains(m.viewContent(), "send failed") {
			t.Errorf("view missing the send-failed notice:\n%s", m.viewContent())
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
		if !strings.Contains(m.viewContent(), "send /login") {
			t.Errorf("view missing the login banner:\n%s", m.viewContent())
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

// TestRelativeTime pins relativeTime tier-by-tier against pito's
// Pito::Formatter::CompactTimeAgo (lib/pito/formatter/compact_time_ago.rb) —
// every boundary below is read off the Ruby's thresholds, not carried over
// from the Go's old "just now"/no-"~" phrasing.
func TestRelativeTime(t *testing.T) {
	cases := []struct {
		name    string
		seconds int64
		want    string
	}{
		{"zero seconds", 0, "~0s ago"},
		{"top of the seconds tier", 59, "~59s ago"},
		{"bottom of the minutes tier", 60, "~1m ago"},
		{"top of the minutes tier", 3_599, "~59m ago"},
		{"bottom of the hours tier", 3_600, "~1h ago"},
		{"top of the hours tier", 86_399, "~23h ago"},
		{"bottom of the days tier", 86_400, "~1d ago"},
		{"top of the days tier", 2_591_999, "~29d ago"},
		{"bottom of the months tier", 2_592_000, "~1mo ago"},
		{"top of the months tier", 31_535_999, "~12mo ago"},
		{"bottom of the years tier", 31_536_000, "~1yr ago"},
		{"deep into the years tier", 63_072_000, "~2yr ago"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := time.Duration(c.seconds) * time.Second
			if got := relativeTime(fixedNow.Add(-d), fixedNow); got != c.want {
				t.Errorf("relativeTime(-%ds) = %q, want %q", c.seconds, got, c.want)
			}
		})
	}

	t.Run("future timestamp clamps to ~0s ago", func(t *testing.T) {
		if got := relativeTime(fixedNow.Add(5*time.Second), fixedNow); got != "~0s ago" {
			t.Errorf("relativeTime(future) = %q, want %q", got, "~0s ago")
		}
	})

	t.Run("zero time.Time is never", func(t *testing.T) {
		if got := relativeTime(time.Time{}, fixedNow); got != "never" {
			t.Errorf("relativeTime(zero) = %q, want %q", got, "never")
		}
	})
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
	// The fallback batches the resume fetch with the picker's slide-in
	// spring kickoff (tea.Batch(fetchResumeCmd, animate)) — runCmd
	// expands the batch, unlike drive's single-message Update.
	m = runCmd(m, cmd)
	m = driveAnim(t, m, 40) // let the slide-in settle before reading the (clipped) view
	view := m.viewContent()
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
	if !strings.Contains(m.viewContent(), "send /login") {
		t.Errorf("banner must survive an anonymous send:\n%s", m.viewContent())
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

	view := m.viewContent()
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
	view = m.viewContent()
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
	if strings.Contains(m.viewContent(), "●∙") || strings.Contains(m.viewContent(), "∙●") {
		t.Errorf("stranded comet spinner:\n%s", m.viewContent())
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

func TestShimmerAnimatesIndefinitely(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"), WithTruecolor(true))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())

	body := `{"body":"<span class=\"pito-subject-shimmer\">13</span> games.","html":true}`
	m, cmd := driveCmd(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventAppend,
		Event: api.Event{ID: 40, TurnID: 15, Kind: "system", Payload: []byte(body)},
	}})
	if len(m.shimmer) != 1 {
		t.Fatal("shimmer event must register for animation")
	}
	if cmd == nil {
		t.Fatal("shimmer must start the animation loop")
	}

	// Ticks advance the phase and re-arm forever (owner call: the web
	// shimmers indefinitely, so does the terminal).
	before := m.phase
	for i := 0; i < 3; i++ {
		var tick tea.Cmd
		m, tick = driveCmd(m, AnimTickMsg{})
		if tick == nil {
			t.Fatalf("tick %d must re-arm — shimmer never settles", i)
		}
	}
	if m.phase == before {
		t.Error("phase must advance")
	}
	if len(m.shimmer) != 1 {
		t.Error("shimmer set must persist")
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

func suggestionsServer(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /chat/u1.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatPageJSON))
	})
	mux.HandleFunc("POST /suggestions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input  string `json:"input"`
			Cursor int    `json:"cursor"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		if body.Input == "/co" {
			_, _ = w.Write([]byte(`{"mode":"slash","stage":"verb","ghost":{"complete_current":"","next_hint":""},
				"menu_items":[{"label":"/config","description":"Read or write credentials","insert":"/config"},
				              {"label":"/connect","description":"Connect a channel","insert":"/connect"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"mode":"free","stage":"free","ghost":{"complete_current":"","next_hint":""},"menu_items":[]}`))
	})
	return mux
}

func TestPaletteFetchesPerKeystrokeAndRenders(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m)

	m.input.SetValue("/c")
	m, cmd := driveCmd(m, key("o")) // typing 'o' → "/co" changed → fetch
	if cmd == nil {
		t.Fatal("typing must fetch suggestions")
	}
	m = runCmd(m, cmd)
	if m.suggest == nil || len(m.suggest.MenuItems) != 2 {
		t.Fatalf("suggestions not set: %+v", m.suggest)
	}
	view := m.viewContent()
	if !strings.Contains(view, "/config") || !strings.Contains(view, "Read or write credentials") {
		t.Errorf("palette missing from view:\n%s", view)
	}
	if !strings.Contains(view, "tab complete") {
		t.Errorf("palette footer missing:\n%s", view)
	}
}

func TestPaletteStaleRepliesLose(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m)
	m.input.SetValue("/co")
	m.suggestSeq = 5
	m = drive(m, SuggestionsMsg{Seq: 3, S: &api.Suggestions{MenuItems: []api.Suggestion{{Label: "stale"}}}})
	if m.suggest != nil {
		t.Error("stale seq must be discarded")
	}
	m = drive(m, SuggestionsMsg{Seq: 5, S: &api.Suggestions{MenuItems: []api.Suggestion{{Label: "fresh"}}}})
	if m.suggest == nil || m.suggest.MenuItems[0].Label != "fresh" {
		t.Error("current seq must land")
	}
}

func TestPaletteNavigationAcceptDismiss(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m)
	m.input.SetValue("/co")
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{
		{Label: "/config", Insert: "/config"},
		{Label: "/connect", Insert: "/connect"},
	}}

	m = drive(m, key("down"))
	if m.suggestSel != 1 {
		t.Errorf("sel = %d, want 1", m.suggestSel)
	}
	m = drive(m, key("up"))
	if m.suggestSel != 0 {
		t.Errorf("sel = %d, want 0", m.suggestSel)
	}

	m, _ = driveCmd(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := m.input.Value(); got != "/config " {
		t.Errorf("tab accept → %q, want %q", got, "/config ")
	}
	if m.suggest != nil {
		t.Error("accept must dismiss the menu")
	}

	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{{Label: "x"}}}
	m = drive(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.suggest != nil {
		t.Error("esc must dismiss")
	}
}

func TestPaletteTokenReplacement(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m)
	m.input.SetValue("ls cha")
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{{Label: "channels", Insert: "channels"}}}
	m, _ = driveCmd(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := m.input.Value(); got != "ls channels " {
		t.Errorf("token replace → %q, want %q", got, "ls channels ")
	}
}

func TestClearingInputDismissesPalette(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m)
	m.input.SetValue("x")
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{{Label: "y"}}}
	m = drive(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.suggest != nil {
		t.Error("empty input must clear the palette")
	}
}

// The palette used to SHRINK the viewport; that contract inverted on
// 2026-07-12 (owner: no layout shift) — it overlays now. The frame must
// still never overflow the terminal, open or closed.
func TestPaletteNeverResizesTheViewport(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m)
	tall := m.sc.height
	m.input.SetValue("/co")
	m.suggestSeq = 1
	m = drive(m, SuggestionsMsg{Seq: 1, S: &api.Suggestions{MenuItems: []api.Suggestion{
		{Label: "/config"}, {Label: "/connect"},
	}}})
	if m.sc.height != tall {
		t.Errorf("palette must not resize the viewport: %d → %d", tall, m.sc.height)
	}
	if lines := strings.Count(m.viewContent(), "\n") + 1; lines > 24 {
		t.Errorf("frame overflows the terminal: %d lines", lines)
	}
	m = drive(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.sc.height != tall {
		t.Errorf("dismiss must leave the viewport untouched: %d → %d", tall, m.sc.height)
	}
}

// A description too long for its column wraps onto a continuation row
// indented to the description column (2 marker cells + labelWidth + the
// "  " gap) — not the marker column, and it carries no marker/label of
// its own.
func TestPaletteWrapsLongDescriptionToContinuationRow(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m) // width 80
	longDesc := strings.Repeat("word ", 30)
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{
		{Label: "a", Description: "short"},
		{Label: "channels", Description: longDesc},
	}}
	m.suggestSel = 0 // "channels" is NOT selected — plain-space indent, easiest to assert exactly

	view := ansi.Strip(m.paletteView())
	lines := strings.Split(view, "\n")
	if len(lines) < 4 {
		t.Fatalf("expected item0 row + item1 first row + >=1 continuation + footer, got %d lines:\n%s", len(lines), view)
	}
	if !strings.Contains(lines[1], "channels") {
		t.Fatalf("lines[1] should be the wrapped item's first row, got %q", lines[1])
	}
	// labelWidth = len("channels") = 8 → descOffset = 2 + 8 + 2 = 12.
	const descOffset = 12
	cont := lines[2]
	indent := strings.Repeat(" ", descOffset)
	if !strings.HasPrefix(cont, indent) {
		t.Errorf("continuation row not indented to the description column (%d cells): %q", descOffset, cont)
	}
	if rest := strings.TrimPrefix(cont, indent); rest == "" || strings.HasPrefix(rest, " ") {
		t.Errorf("continuation row should carry no marker/label — text starts right at the description column: %q", cont)
	}
	if strings.Contains(cont, "channels") {
		t.Errorf("continuation row must carry no label of its own: %q", cont)
	}
}

// The selected item's block — including every wrapped continuation
// row — must survive the row-budget trim even when other items get
// dropped to make room.
func TestPaletteSelectedLongItemStaysFullyVisible(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m) // width 80, height 24 → chatViewportHeight() well above paletteMax+1
	items := make([]api.Suggestion, 6)
	for i := range items {
		items[i] = api.Suggestion{Label: fmt.Sprintf("zero%d", i)}
	}
	// The selected item's description is long enough to wrap several
	// rows, and its LAST word is a unique tail marker — if the tail
	// marker survives, the whole wrapped block survived.
	items[5].Label = "selected"
	items[5].Description = strings.Repeat("word ", 40) + "TAILMARKER"
	m.suggest = &api.Suggestions{MenuItems: items}
	m.suggestSel = 5

	view := m.paletteView()
	if !strings.Contains(view, "selected") {
		t.Fatal("selected item's label must render")
	}
	if !strings.Contains(view, "TAILMARKER") {
		t.Error("selected item's LAST wrapped row was dropped — the block was truncated instead of trimming other items")
	}
	if !strings.Contains(view, "tab complete") {
		t.Error("footer must still render alongside the selected item's wrapped block")
	}
}

// Row budget: items + continuations + the footer must never exceed the
// viewport the overlay paints over, even with 6 items all wrapping —
// fewer items get shown rather than clipped rows.
func TestPaletteRowBudgetCapsWithSixLongItems(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m) // width 80, height 24
	longDesc := strings.Repeat("word ", 20)
	items := make([]api.Suggestion, 6)
	for i := range items {
		items[i] = api.Suggestion{Label: fmt.Sprintf("item%d", i), Description: longDesc}
	}
	m.suggest = &api.Suggestions{MenuItems: items}
	m.suggestSel = 3

	view := ansi.Strip(m.paletteView())
	lines := strings.Split(view, "\n")
	if got := len(lines); got > paletteMax+1 {
		t.Errorf("palette emitted %d rows, want <= paletteMax+1 (%d)", got, paletteMax+1)
	}
	if got := len(lines); got > m.chatViewportHeight() {
		t.Fatalf("palette rows (%d) exceed the viewport (%d) — overlayBottom would clip its own top", got, m.chatViewportHeight())
	}
	if !strings.Contains(view, "item3") {
		t.Error("selected item must stay visible even under a tight row budget")
	}
	if shown := strings.Count(view, "item"); shown >= len(items) {
		t.Errorf("expected the row budget to trim below all %d items when every one wraps, got %d shown", len(items), shown)
	}
	if !strings.Contains(view, "tab complete") {
		t.Error("footer must still render")
	}
}

// A description column under ~10 cells (a tiny terminal) makes word-wrap
// pathological — fall back to today's single hard-clipped line instead
// of wrapping into single-letter rows.
func TestPaletteNarrowWidthFallsBackToSingleLineClip(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = drive(m, tea.WindowSizeMsg{Width: 20, Height: 24})
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{
		{Label: "channels", Description: strings.Repeat("word ", 30)},
	}}
	m.suggestSel = 0

	view := ansi.Strip(m.paletteView())
	lines := strings.Split(view, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 1 clipped item row + footer, got %d lines:\n%s", len(lines), view)
	}
	if w := lipgloss.Width(lines[0]); w > m.contentWidth() {
		t.Errorf("item row not clipped to contentWidth: %d > %d", w, m.contentWidth())
	}
}

// The model-mention wire contract's TUI half: the @ai item's Model
// substring inside its already-interpolated Description paints orange
// (render.ColorOrange, 256-color "215" off truecolor) — a fragment
// composed alongside the surrounding dim description text, not a
// blanket restyle.
const paletteOrangeOpen = "\x1b[38;5;215m"

func TestPaletteModelMentionPaintedOrange(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m)
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{
		{Label: "@ai", Description: "ask claude-sonnet-5 anything", Model: "claude-sonnet-5"},
	}}
	m.suggestSel = 0 // unselected — the "channels" test's easiest-to-assert row

	view := m.paletteView()
	want := paletteOrangeOpen + "claude-sonnet-5\x1b[m"
	if !strings.Contains(view, want) {
		t.Errorf("model mention not painted orange:\nwant substring: %q\ngot view:\n%s", want, view)
	}
	if got := ansi.Strip(view); !strings.Contains(got, "ask claude-sonnet-5 anything") {
		t.Errorf("stripped view lost description text: %q", got)
	}
}

// No Model field (the every-other-item, and the no-model-configured @ai
// fallback, cases) must render BYTE-IDENTICAL to the pre-feature
// rendering: plain statusStyle over the whole description, no orange
// anywhere.
func TestPaletteNoModelFieldByteIdenticalToPlainRendering(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m)
	desc := "ask the assistant anything"
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{
		{Label: "@ai", Description: desc}, // Model left zero-valued ("")
	}}
	m.suggestSel = 1 // nothing selected among the one item

	got := m.paletteView()
	if strings.Contains(got, paletteOrangeOpen) {
		t.Errorf("no-model item must never paint orange: %s", got)
	}
	labelWidth := lipgloss.Width("@ai")
	first := "  " + "@ai" + strings.Repeat(" ", labelWidth-lipgloss.Width("@ai"))
	want := lipgloss.NewStyle().MaxWidth(m.contentWidth()).Render(first+statusStyle.Render("  "+desc)) + "\n" +
		statusStyle.Render("tab complete · ↑/↓ move · esc dismiss")
	if got != want {
		t.Errorf("no-model rendering diverged from plain statusStyle rendering:\n got: %q\nwant: %q", got, want)
	}
}

// A Model that doesn't actually occur in Description (a mismatched or
// stale value) must render plain — no crash, no partial/garbled paint.
func TestPaletteModelSubstringMissingRendersPlain(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m)
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{
		{Label: "@ai", Description: "ask the assistant anything", Model: "claude-sonnet-5"},
	}}
	m.suggestSel = 0

	view := m.paletteView()
	if strings.Contains(view, paletteOrangeOpen) {
		t.Errorf("Model with no match in Description must not paint orange: %s", view)
	}
	if got := ansi.Strip(view); !strings.Contains(got, "ask the assistant anything") {
		t.Errorf("description text lost: %q", got)
	}
}

// The selected row restyles the marker/label with the accent, but the
// description column keeps wearing its own style (statusStyle, dim) —
// today's behavior, unchanged by this feature — so the model span must
// still carve out into orange on a selected row exactly as it does on an
// unselected one; the accent bar must not swallow it.
func TestPaletteModelMentionSelectedRowStaysOrange(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m)
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{
		{Label: "@ai", Description: "ask claude-sonnet-5 anything", Model: "claude-sonnet-5"},
	}}
	m.suggestSel = 0 // the only item — selected

	view := m.paletteView()
	want := paletteOrangeOpen + "claude-sonnet-5\x1b[m"
	if !strings.Contains(view, want) {
		t.Errorf("selected row must keep the model span orange:\nwant substring: %q\ngot view:\n%s", want, view)
	}
}

// A description long enough to wrap, where the model id lands on a
// CONTINUATION line rather than the first — per-line substring matching
// (style after wrap) must still find and paint it there.
func TestPaletteModelMentionOnContinuationLine(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m) // width 80
	desc := strings.Repeat("filler ", 20) + "claude-sonnet-5 is the active model"
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{
		{Label: "@ai", Description: desc, Model: "claude-sonnet-5"},
	}}
	m.suggestSel = 0

	view := m.paletteView()
	lines := strings.Split(view, "\n")
	firstOrange := -1
	for i, line := range lines {
		if strings.Contains(line, paletteOrangeOpen) {
			firstOrange = i
			break
		}
	}
	if firstOrange < 0 {
		t.Fatalf("model mention never painted orange across wrapped lines:\n%s", view)
	}
	if firstOrange == 0 {
		t.Errorf("expected the model id to land on a continuation line (filler pushed it past line 0), got line %d", firstOrange)
	}
	if got := ansi.Strip(view); !strings.Contains(got, "claude-sonnet-5 is the active model") {
		t.Errorf("continuation text lost: %q", got)
	}
}

// The label-parens contract: the model-mention wire contract's substring
// moved into the LABEL ("@ai(<model>)"); Description is back to the plain
// sentence with no model in it. paletteAccentSpan is the same mechanism
// TestPaletteModelMentionPaintedOrange already proved on Description —
// these four cover it on Label.

func TestPaletteLabelModelMentionPaintedOrange(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m)
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{
		{Label: "@ai(claude-sonnet-5)", Description: "ask the assistant anything", Model: "claude-sonnet-5"},
	}}
	m.suggestSel = 1 // nothing selected — the label's plain (unstyled) branch

	view := m.paletteView()
	want := paletteOrangeOpen + "claude-sonnet-5\x1b[m"
	if !strings.Contains(view, want) {
		t.Errorf("label model mention not painted orange:\nwant substring: %q\ngot view:\n%s", want, view)
	}
	if n := strings.Count(view, paletteOrangeOpen); n != 1 {
		t.Errorf("expected exactly one orange span (the label's — the plain description carries no model), got %d: %s", n, view)
	}
	if got := ansi.Strip(view); !strings.Contains(got, "@ai(claude-sonnet-5)") || !strings.Contains(got, "ask the assistant anything") {
		t.Errorf("label or description text lost: %q", got)
	}
}

// The selected row restyles the label with the accent bar's bold sel
// style, but the model span must still carve out into orange — same
// rule TestPaletteModelMentionSelectedRowStaysOrange proved for
// Description — so the accent bar never swallows the mention.
func TestPaletteLabelModelMentionSelectedRowStaysOrange(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m)
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{
		{Label: "@ai(claude-sonnet-5)", Description: "ask the assistant anything", Model: "claude-sonnet-5"},
	}}
	m.suggestSel = 0 // the only item — selected

	view := m.paletteView()
	want := paletteOrangeOpen + "claude-sonnet-5\x1b[m"
	if !strings.Contains(view, want) {
		t.Errorf("selected row must keep the label's model span orange:\nwant substring: %q\ngot view:\n%s", want, view)
	}
	if got := ansi.Strip(view); !strings.Contains(got, "@ai(claude-sonnet-5)") {
		t.Errorf("stripped view lost the label text: %q", got)
	}
}

// An old server that still sends the plain "@ai" label (whether or not
// it also sends Model, e.g. mid-migration or a server still painting the
// model into Description per the earlier contract) must render the
// label byte-identical to today: "claude-sonnet-5" never occurs inside
// "@ai", so paletteAccentSpan's idx < 0 branch falls straight through to
// plain, unstyled label rendering.
func TestPaletteOldServerPlainLabelByteIdentical(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m)
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{
		{Label: "@ai", Description: "ask claude-sonnet-5 anything", Model: "claude-sonnet-5"},
	}}
	m.suggestSel = 1 // nothing selected — the label's plain (unstyled) branch

	got := m.paletteView()
	labelWidth := lipgloss.Width("@ai")
	wantLabel := "  " + "@ai" + strings.Repeat(" ", labelWidth-lipgloss.Width("@ai"))
	if !strings.HasPrefix(got, wantLabel) {
		t.Errorf("plain \"@ai\" label rendering diverged from today's:\ngot view:\n%s\nwant prefix: %q", got, wantLabel)
	}
}

// labelWidth (the padding every row's description column aligns to) must
// account for the FULL label text, parens and model id included — a
// shorter item's label pads out to the longer "@ai(<model>)" label's
// width, not the old bare "@ai" width.
func TestPaletteLabelWidthAccountsForModelParens(t *testing.T) {
	m, _ := newTestModel(t, suggestionsServer(t), WithConversation("u1"))
	m = sized(m)
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{
		{Label: "@ai(claude-sonnet-5)", Description: "ask the assistant anything", Model: "claude-sonnet-5"},
		{Label: "channels", Description: "list your channels"},
	}}
	m.suggestSel = 0

	got := ansi.Strip(m.paletteView())
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 item lines: %v", lines)
	}
	longLabel := "@ai(claude-sonnet-5)"
	labelWidth := lipgloss.Width(longLabel)
	wantLine := "  channels" + strings.Repeat(" ", labelWidth-lipgloss.Width("channels")) + "  list your channels"
	if lines[1] != wantLine {
		t.Errorf("shorter label not padded to the longer (paren-inclusive) label width:\n got: %q\nwant: %q", lines[1], wantLine)
	}
}

func TestAvatarCellsVanishEverywhere(t *testing.T) {
	// Owner call: no stand-in glyphs, no phantom column — avatar cells
	// disappear from tables on every terminal.
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	payload := `{"body":"x","html":true,"table_rows":[{"cells":[
		{"html": true, "text": "<img class=\"pito-channel-tiny-avatar\" src=\"/a.jpg\"/>", "class": "pito-cell-avatar"},
		{"text": "@x", "class": ""}]}]}`
	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventAppend,
		Event: api.Event{ID: 62, TurnID: 32, Kind: "system", Payload: []byte(payload)},
	}})
	view := m.viewContent()
	if strings.Contains(view, "◉") {
		t.Errorf("no stand-in glyphs:\n%s", view)
	}
	if !strings.Contains(view, "@x") {
		t.Errorf("remaining columns must render:\n%s", view)
	}
}

func TestMutationReplyEchoesAndResyncs(t *testing.T) {
	var fetches int
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"uuid":"u1","turn_id":null}`))
	})
	mux.HandleFunc("GET /chat/u1.json", func(w http.ResponseWriter, r *http.Request) {
		fetches++
		w.Header().Set("Content-Type", "application/json")
		if fetches == 1 {
			_, _ = w.Write([]byte(`{"conversation":{"uuid":"u1","title":"t"},"events":[
				{"id": 5, "turn_id": 2, "kind": "system", "payload": {"text": "SORTED OLD"}, "created_at": "2026-07-05T10:00:00Z"}]}`))
			return
		}
		// The mutation edited event 5 in place server-side.
		_, _ = w.Write([]byte(`{"conversation":{"uuid":"u1","title":"t"},"events":[
			{"id": 5, "turn_id": 2, "kind": "system", "payload": {"text": "SORTED NEW"}, "created_at": "2026-07-05T10:00:00Z"}]}`))
	})
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())

	m.input.SetValue("#gamma-5324 sort title desc")
	m, cmd := driveCmd(m, key("enter"))
	m, cmd = driveCmd(m, cmd()) // SendResultMsg{turn_id null}

	view := m.viewContent()
	if !strings.Contains(view, "#gamma-5324 sort title desc") {
		t.Errorf("mutation reply must echo locally:\n%s", view)
	}
	if cmd == nil {
		t.Fatal("mutation ack must schedule the resync safety net")
	}
	// cmd is a tea.Tick — execute it to get the deferred resync, then run it.
	deferred, ok := cmd().(resyncNowMsg)
	if !ok {
		t.Fatalf("expected resyncNowMsg, got %T", cmd())
	}
	m, cmd = driveCmd(m, deferred)
	m = drive(m, cmd())
	if view := m.viewContent(); !strings.Contains(view, "SORTED NEW") || strings.Contains(view, "SORTED OLD") {
		t.Errorf("resync must merge the in-place mutation:\n%s", view)
	}
}

func TestContextMeterAndMiniStatusFlow(t *testing.T) {
	page := `{"conversation":{"uuid":"u-1","title":"t","display_name":"t",
		"context":{"pct":2.0,"count":2,"threshold":100}},
		"notifications":{"unread":28},"events":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()
	m, _ := newTestModel(t, nil, WithConversation("u-1"))
	client, err := api.New(srv.URL, filepath.Join(t.TempDir(), "cookies.json"))
	if err != nil {
		t.Fatal(err)
	}
	m.client = client
	m = sized(m)

	fetched := m.fetchChatCmd("u-1", false)()
	next, _ := m.Update(fetched)
	m = next.(Model)
	if m.meterCtx == nil || m.meterCtx.Pct != 2.0 {
		t.Fatalf("meter not absorbed from fetch: %+v", m.meterCtx)
	}
	if m.unread != 28 {
		t.Fatalf("unread not absorbed: %d", m.unread)
	}

	// The meter line renders above the input; status = dot + tag + unread.
	view := m.viewContent()
	if !strings.Contains(view, "2%") {
		t.Errorf("meter counter missing from view:\n%s", view)
	}
	// pito fat-cut 2026-07-12: the identity after the dot is the SERVER
	// tag alone — the httptest host is 127.0.0.1, a dev host, so "dev";
	// the nickname, host, and state word are all gone.
	// ctrl+/ and the count+flag piece are two separate style.Render calls
	// (statusLine, model.go) — ANSI-strip before matching the joined
	// text, or the reset/color-start codes between them break the
	// substring.
	if !strings.Contains(view, "dev") || !strings.Contains(ansi.Strip(view), "ctrl+/ 28 ⚑") {
		t.Errorf("mini status missing tag/unread:\n%s", view)
	}
	for _, gone := range []string{"connected", "gmrdad82@", "127.0.0.1"} {
		if strings.Contains(view, gone) {
			t.Errorf("%q must be gone from the status bar:\n%s", gone, view)
		}
	}
	// Action affordances cluster right (owner ruling 2026-07-13): ctrl+f
	// footage · ctrl+k commands tail the right side, AFTER the
	// dot/tag/unread pieces, while ctrl+c Quit stays alone on the left.
	line := ansi.Strip(m.statusLine())
	for _, want := range []string{"ctrl+k", "commands", "ctrl+f", "footage"} {
		if !strings.Contains(line, want) {
			t.Errorf("status line missing action hint %q:\n%s", want, line)
		}
	}
	if i := strings.Index(line, "ctrl+/ 28 ⚑"); i == -1 || i > strings.Index(line, "ctrl+k") {
		t.Errorf("action hints must trail the dot/tag/unread cluster, not lead it:\n%s", line)
	}
	// ctrl+c (Quit) sits ALONE on the left, ahead of the whole right
	// cluster — a single occurrence, positioned before ctrl+k/ctrl+f.
	if n := strings.Count(line, "ctrl+c"); n != 1 {
		t.Errorf("ctrl+c must appear exactly once (left, alone), got %d:\n%s", n, line)
	}
	if strings.Index(line, "ctrl+c") > strings.Index(line, "ctrl+k") {
		t.Errorf("ctrl+c (left) must come before the right cluster's ctrl+k:\n%s", line)
	}
	// Unauthenticated sessions read as pito's own anonymous word, and the
	// action-hint cluster hides — mirrors pito web's mini_status_component
	// gating its own ctrl+k hint on @state (authenticated only).
	loggedOut := m
	loggedOut.needsLogin = true
	if !strings.Contains(loggedOut.viewContent(), "tarnished") {
		t.Errorf("unauthenticated status must read pito's anonymous word:\n%s", loggedOut.viewContent())
	}
	if loggedOutLine := ansi.Strip(loggedOut.statusLine()); strings.Contains(loggedOutLine, "ctrl+k") || strings.Contains(loggedOutLine, "ctrl+f") {
		t.Errorf("action hints must hide for an unauthenticated session:\n%s", loggedOutLine)
	}

	// conversation.update patches both live.
	next, _ = m.Update(CableEventMsg{M: cable.StreamMessage{
		Type:          cable.TypeConversationUpdate,
		Context:       &api.ContextMeter{Pct: 43, Count: 43, Threshold: 100},
		Notifications: &api.NotifCount{Unread: 3},
	}})
	m = next.(Model)
	if m.meterCtx.Pct != 43 || m.unread != 3 {
		t.Fatalf("conversation.update not applied: %+v unread=%d", m.meterCtx, m.unread)
	}
	if view := m.viewContent(); !strings.Contains(view, "43%") || !strings.Contains(ansi.Strip(view), "ctrl+/ 3 ⚑") {
		t.Errorf("patched values not rendered:\n%s", view)
	}
}

func TestShiftArrowsScrollEvenWhileTyping(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	// A tall transcript so the viewport can actually scroll.
	for i := int64(1); i <= 80; i++ {
		m.transcript.Append(api.Event{ID: i, TurnID: i, Kind: "system",
			Payload: []byte(`{"text":"row"}`)})
	}
	m.refreshViewport()
	m.sc.GotoBottom()
	m.follow = true

	// Mid-typing: text in the prompt, shift+up still scrolls.
	m.input.SetValue("ls games")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})
	m = next.(Model)
	if m.sc.AtBottom() {
		t.Fatal("shift+up must scroll the viewport even while typing")
	}
	if m.follow {
		t.Fatal("scrolling up must release follow")
	}
	if m.input.Value() != "ls games" {
		t.Fatalf("input must be untouched, got %q", m.input.Value())
	}
	// And shift+down heads back toward the bottom.
	for i := 0; i < 100; i++ {
		next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
		m = next.(Model)
	}
	if !m.sc.AtBottom() || !m.follow {
		t.Fatal("shift+down to the bottom must restore follow")
	}
}

func TestMouseWheelScrollsConversationAndRestoresFollow(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	for i := int64(1); i <= 80; i++ {
		m.transcript.Append(api.Event{ID: i, TurnID: i, Kind: "system",
			Payload: []byte(`{"text":"row"}`)})
	}
	m.refreshViewport()
	m.sc.GotoBottom()
	m.follow = true

	// The View must actually ask the terminal for wheel events.
	if m.View().MouseMode == tea.MouseModeNone {
		t.Fatal("View must enable a mouse mode or wheel events never arrive")
	}

	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m = next.(Model)
	if m.sc.AtBottom() {
		t.Fatal("wheel up must scroll the viewport")
	}
	if m.follow {
		t.Fatal("wheel up must release follow")
	}
	for i := 0; i < 100; i++ {
		next, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
		m = next.(Model)
	}
	if !m.sc.AtBottom() || !m.follow {
		t.Fatal("wheel down to the bottom must restore follow")
	}
}

func TestMouseWheelMovesNotificationsCursor(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	m.mode = modeNotifications
	for i := 0; i < 5; i++ {
		m.notif.rows = append(m.notif.rows, api.NotificationRow{Message: "n"})
	}
	m.notif.fetched = true

	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = next.(Model)
	if m.notif.cursor != 1 {
		t.Fatalf("wheel down in notifications must move the cursor, got %d", m.notif.cursor)
	}
	next, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m = next.(Model)
	if m.notif.cursor != 0 {
		t.Fatalf("wheel up in notifications must move the cursor back, got %d", m.notif.cursor)
	}
}

// The web contract under test: history_controller.js — oh-my-zsh prefix
// recall on ↑/↓, snapshot draft at index -1, no wrap, edits end the walk.
func TestInputHistoryPrefixRecall(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	m.histEntries = []string{"/config google", "list vids", "/config", "list games"} // newest first

	up := func() { next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp}); m = next.(Model) }
	down := func() { next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown}); m = next.(Model) }

	// Empty buffer: ↑ walks everything, newest first.
	up()
	if got := m.input.Value(); got != "/config google" {
		t.Fatalf("first ↑ must recall the newest entry, got %q", got)
	}
	up()
	if got := m.input.Value(); got != "list vids" {
		t.Fatalf("second ↑ must recall the next older entry, got %q", got)
	}
	// ↓ returns to the newest, then to the (empty) draft.
	down()
	if got := m.input.Value(); got != "/config google" {
		t.Fatalf("↓ must walk back newer, got %q", got)
	}
	down()
	if got := m.input.Value(); got != "" {
		t.Fatalf("↓ past the newest must restore the empty draft, got %q", got)
	}
	// The prefix survived the draft restore: a further ↑ resumes the walk.
	up()
	if got := m.input.Value(); got != "/config google" {
		t.Fatalf("↑ after the draft restore must resume the walk, got %q", got)
	}

	// A real edit ends the session; the next ↑ prefix-filters on "/conf".
	m.input.SetValue("/conf")
	next, _ := m.Update(tea.KeyPressMsg{Text: "i", Code: 'i'})
	m = next.(Model)
	if m.histPrefix != nil {
		t.Fatal("a real edit must end the recall session")
	}
	m.input.SetValue("/conf")
	up()
	if got := m.input.Value(); got != "/config google" {
		t.Fatalf("prefixed ↑ must recall the newest /conf… entry, got %q", got)
	}
	up()
	if got := m.input.Value(); got != "/config" {
		t.Fatalf("prefixed ↑ again must skip non-matches, got %q", got)
	}
	// No wrap at the oldest match.
	up()
	if got := m.input.Value(); got != "/config" {
		t.Fatalf("↑ at the oldest match must hold (no wrap), got %q", got)
	}
	// ↓ back down restores the snapshot draft "/conf".
	down()
	down()
	if got := m.input.Value(); got != "/conf" {
		t.Fatalf("↓ past the newest match must restore the draft, got %q", got)
	}
}

func TestInputHistoryRecordsSendsAndSeedsFromEchoes(t *testing.T) {
	sent := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		sent <- body.Message
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"turn_id":9}`)
	})
	m, _ := newTestModel(t, mux, WithConversation("u-1"))
	m = sized(m)

	// Sending records the entry (newest first) and consecutive dupes collapse.
	for _, text := range []string{"list games", "list games", "ls vids"} {
		m.input.SetValue(text)
		next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = next.(Model)
		if cmd != nil {
			cmd() // run the send so the httptest handler sees it
			<-sent
		}
	}
	if len(m.histEntries) != 2 || m.histEntries[0] != "ls vids" || m.histEntries[1] != "list games" {
		t.Fatalf("send recording wrong: %#v", m.histEntries)
	}

	// Empty history is a no-op ↑ (fresh model, nothing seeded).
	m2, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-2"))
	m2 = sized(m2)
	next, _ := m2.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m2 = next.(Model)
	if m2.input.Value() != "" || m2.histPrefix != nil {
		t.Fatal("↑ with no history must be a no-op")
	}

	// Seeding: echo events in the transcript become entries, newest first —
	// the web's page-load sent_history analog.
	m2.transcript.Append(api.Event{ID: 1, TurnID: 1, Kind: api.KindEcho,
		Payload: []byte(`{"text":"analyze channel"}`)})
	m2.transcript.Append(api.Event{ID: 2, TurnID: 2, Kind: api.KindEcho,
		Payload: []byte(`{"text":"show game 3"}`)})
	m2.seedHistory()
	if len(m2.histEntries) != 2 || m2.histEntries[0] != "show game 3" || m2.histEntries[1] != "analyze channel" {
		t.Fatalf("seeding wrong: %#v", m2.histEntries)
	}
}

// The web contract under test: chatbox_hints_controller.js + the
// Filter/Channel components — kbd chip + plain value token, no prefix
// words, red "none" when the account has no channels.
func TestScopeHintChipAndNoneState(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)

	m.input.SetValue("list vids")
	m.channels = nil
	hint := ansi.Strip(m.scopeHintLine())
	if !strings.Contains(hint, "shift+tab") || !strings.Contains(hint, "none") {
		t.Fatalf("channel hint without channels must show the chip + red none: %q", hint)
	}
	if strings.Contains(hint, "channel:") {
		t.Fatalf("no prefix words — the web shows none: %q", hint)
	}

	m.channels = []string{"@all", "@gmrdad82"}
	m.scopeChannel = "@all"
	hint = ansi.Strip(m.scopeHintLine())
	if !strings.Contains(hint, "shift+tab") || !strings.Contains(hint, "@all") {
		t.Fatalf("channel hint = %q", hint)
	}

	m.input.SetValue("analyze")
	hint = ansi.Strip(m.scopeHintLine())
	if !strings.Contains(hint, "ctrl+space") || !strings.Contains(hint, "7d") {
		t.Fatalf("period hint = %q", hint)
	}
}

// The perf regression under test (owner 2026-07-12: "everything seems
// very very slow"): resolved thinking/confirmation events must NOT hold
// their turns on the 40ms animation loop — only genuinely animating
// events may.
func TestShimmerMarksOnlyPendingWork(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"), WithTruecolor(true))
	m = sized(m)

	// A backfill full of RESOLVED thinking lines (one per turn — the
	// real shape of a scrollback) must not mark anything.
	m = drive(m, ChatFetchedMsg{Page: &api.ChatPage{
		Conversation: api.Conversation{UUID: "u-1"},
		Events: []api.Event{
			{ID: 1, TurnID: 1, Kind: api.KindThinking, Payload: []byte(`{"resolved":true,"elapsed_seconds":1.0}`)},
			{ID: 2, TurnID: 2, Kind: api.KindThinking, Payload: []byte(`{"resolved":true,"elapsed_seconds":2.0}`)},
			{ID: 3, TurnID: 3, Kind: api.KindConfirmation, Payload: []byte(`{"resolved":true,"outcome_text":"done"}`)},
		},
	}})
	if len(m.shimmer) != 0 {
		t.Fatalf("resolved events must not join the shimmer map: %v", m.shimmer)
	}

	// A PENDING thinking event animates — and its resolve releases it.
	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventAppend,
		Event: api.Event{ID: 4, TurnID: 9, Kind: api.KindThinking, Payload: []byte(`{"resolved":false,"dictionary":"slash","order":[0]}`)},
	}})
	if !m.shimmer[9] {
		t.Fatal("a pending thinking event must animate")
	}
	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventReplace,
		Event: api.Event{ID: 4, TurnID: 9, Kind: api.KindThinking, Payload: []byte(`{"resolved":true,"word_index":0,"elapsed_seconds":1.2}`)},
	}})
	if m.shimmer[9] {
		t.Fatal("a resolved thinking event must release its turn from the animation loop")
	}
}

// The palette must OVERLAY the conversation, not reflow it (owner
// 2026-07-12: "autocomplete shifts the conversation up and down").
func TestSuggestionsPaletteOverlaysWithoutReflow(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	for i := int64(1); i <= 40; i++ {
		m.transcript.Append(api.Event{ID: i, TurnID: i, Kind: "system",
			Payload: []byte(`{"text":"row"}`)})
	}
	m.refreshViewport()
	heightBefore := m.chatViewportHeight()
	linesBefore := strings.Count(m.viewContent(), "\n")

	m.input.SetValue("/con")
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{
		{Label: "/connect", Insert: "/connect"},
		{Label: "/config", Insert: "/config"},
	}}
	if got := m.chatViewportHeight(); got != heightBefore {
		t.Fatalf("palette must not change the viewport height: %d → %d", heightBefore, got)
	}
	view := m.viewContent()
	if strings.Count(view, "\n") != linesBefore {
		t.Fatalf("palette must not change the frame's line count: %d → %d", linesBefore, strings.Count(view, "\n"))
	}
	if !strings.Contains(view, "/connect") {
		t.Fatal("palette content must still render (overlaid)")
	}
}

// /resume is client grammar now (owner bug 2026-07-12): the server
// answers it browser-only, so the TUI opens its own picker instead —
// and esc returns to the conversation it was opened over.
func TestSlashResumeOpensPickerAndEscReturns(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())

	m.input.SetValue("/resume")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.mode != modePicker {
		t.Fatal("/resume must open the conversation picker client-side")
	}
	if cmd == nil {
		t.Fatal("/resume must fire the page-1 resume fetch")
	}
	if m.input.Value() != "" {
		t.Fatal("/resume must clear the prompt")
	}
	// Deliver the page-1 fetch (the in-flight loader holds the gate
	// open until it lands), then let the open spring settle.
	m = drive(m, m.fetchResumeCmd()())
	m = driveAnim(t, m, 60)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	m = driveAnim(t, m, 60)
	if m.mode != modeChat || m.conv.UUID != "u1" {
		t.Fatalf("esc must return to the open conversation, mode=%v uuid=%q", m.mode, m.conv.UUID)
	}
}

// Cable arrivals GLIDE the following viewport to the bottom over a few
// ticks instead of snapping (owner 2026-07-12: "snaps and jumps all
// over the place"); giant jumps (backfills) still land instantly.
func TestFollowGlidesOnSmallGrowthSnapsOnBackfill(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	for i := int64(1); i <= 60; i++ {
		m.transcript.Append(api.Event{ID: i, TurnID: i, Kind: "system",
			Payload: []byte(`{"text":"row"}`)})
	}
	m.refreshViewport() // giant first paint: snaps straight to the bottom
	if !m.sc.AtBottom() {
		t.Fatal("a backfill-sized jump must land instantly")
	}

	// A small arrival: the viewport starts gliding, holds the gate open,
	// and reaches the bottom within a few ticks.
	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type: cable.TypeEventAppend,
		Event: api.Event{ID: 100, TurnID: 100, Kind: "system",
			Payload: []byte(`{"text":"one\ntwo\nthree\nfour"}`)},
	}})
	if m.sc.AtBottom() {
		t.Fatal("a small arrival must glide, not snap")
	}
	if !m.scrollEasing || !m.animGateOpen() {
		t.Fatal("a glide in flight must hold the animation gate open")
	}
	for i := 0; i < 30 && !m.sc.AtBottom(); i++ {
		m = drive(m, AnimTickMsg{})
	}
	if !m.sc.AtBottom() {
		t.Fatal("the glide must reach the bottom")
	}
	if m.scrollEasing {
		t.Fatal("a landed glide must release the gate")
	}
}

// ctrl+c confirms Crush-style (owner 2026-07-12): first press arms +
// shows the warning, second quits, any other key stands down.
func TestCtrlCArmsThenQuits(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	// 76 cols, not the usual 80 (pito-tui 3.0.0, U1.2/U1.3 single-space +
	// "footage" label shrink): statusLine's own graceful-truncation gate
	// is `contentWidth - width(left) - width(withHints) - 1 > 0` (model.go,
	// statusLine). Empirically sizing this exact model (armed left =
	// "ctrl+c" chip + "again to quit"; right = dot/tag/"ctrl+/ 0 ⚑" +
	// " · " + "ctrl+f footage · ctrl+k commands") across widths 60..82
	// shows the hints cluster present at width>=77 and gone at width<=76
	// — the tighter bar shed the doubled spacing and "update "→"" label
	// trim, so 80 cols now legitimately fits both; 76 is the tightest
	// width that still exercises the drop this test exists to prove.
	m = drive(m, tea.WindowSizeMsg{Width: 76, Height: 24})
	next, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(Model)
	if m.quitArmed == 0 {
		t.Fatal("first ctrl+c must open the confirm window, not quit")
	}
	if !strings.Contains(ansi.Strip(m.viewContent()), "again to quit") {
		t.Fatal("the armed window must say so on the status row")
	}
	// Graceful truncation (owner ruling 2026-07-13): the right cluster's
	// own ctrl+k/ctrl+f hints are the FIRST thing to drop as the bar
	// narrows — at 76 cols there isn't room for both the armed "again to
	// quit" warning AND the hints, and the warning always wins.
	if armedLine := ansi.Strip(m.statusLine()); strings.Contains(armedLine, "ctrl+k") {
		t.Errorf("action hints must drop before the armed quit warning gets clipped:\n%s", armedLine)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("second ctrl+c must quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("second ctrl+c's command must be tea.Quit")
	}

	// Any other key disarms.
	m.quitArmed = quitArmTicks
	next, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(Model)
	if m.quitArmed != 0 {
		t.Fatal("another key must stand the quit down")
	}
}

// The virtualization contract (owner 2026-07-12, launch-guarding):
// off-screen shimmer turns are NOT re-rendered by animation ticks.
func TestAnimTicksTouchOnlyVisibleShimmerTurns(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"), WithTruecolor(true))
	m = sized(m)
	// A tall transcript: turn 1 (shimmer-marked) far above a full screen
	// of filler, viewport pinned at the bottom.
	m.transcript.Append(api.Event{ID: 1, TurnID: 1, Kind: "system",
		Payload: []byte(`{"text":"<span class="pito-subject-shimmer">Old Subject</span>","html":true}`)})
	for i := int64(2); i <= 120; i++ {
		m.transcript.Append(api.Event{ID: i, TurnID: i, Kind: "system",
			Payload: []byte(`{"text":"row"}`)})
	}
	m.shimmer[1] = true
	m.refreshViewport()
	m.sc.GotoBottom()

	renders := 0
	m.transcript.SetRenderer(func(ev api.Event, width int) string {
		if ev.TurnID == 1 {
			renders++
		}
		return "x"
	})
	m.transcript.TotalLines(m.contentWidth()) // settle caches post-swap
	renders = 0
	for i := 0; i < 10; i++ {
		next, _ := m.Update(AnimTickMsg{})
		m = next.(Model)
		_ = m.viewContent() // materialize the frame — renders are lazy now
	}
	if renders != 0 {
		t.Fatalf("an off-screen shimmer turn re-rendered %d times during ticks", renders)
	}
	// Scrolled into view, it animates again (within the 30fps shimmer
	// beat — every second tick — so a few ticks guarantee one).
	m.sc.GotoTop()
	for i := 0; i < 4; i++ {
		next, _ := m.Update(AnimTickMsg{})
		m = next.(Model)
		_ = m.viewContent()
	}
	if renders == 0 {
		t.Fatal("a visible shimmer turn must re-render on ticks")
	}
}

func TestEnterWithPaletteOpenSendsRaw(t *testing.T) {
	sent := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat" {
			var body struct {
				Input string `json:"input"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			sent <- body.Input
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"uuid":"u-1","turn_id":9}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	m, _ := newTestModel(t, nil, WithConversation("u-1"))
	client, err := api.New(srv.URL, filepath.Join(t.TempDir(), "cookies.json"))
	if err != nil {
		t.Fatal(err)
	}
	m.client = client
	m = sized(m)

	m.input.SetValue("ls cha")
	next, _ := m.Update(SuggestionsMsg{Seq: m.suggestSeq, S: &api.Suggestions{
		MenuItems: []api.Suggestion{{Label: "channels", Insert: "channels"}},
	}})
	m = next.(Model)
	if m.suggest == nil {
		t.Fatal("palette should be open")
	}
	// Enter with the palette open: the RAW text goes out, the suggestion
	// is never inserted (web parity, owner 2026-07-09).
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("enter must produce the send command")
	}
	cmd()
	select {
	case got := <-sent:
		if got != "ls cha" {
			t.Fatalf("enter must send the raw input, sent %q", got)
		}
	default:
		t.Fatal("nothing was sent")
	}
	if m.suggest != nil {
		t.Fatal("send must clear the palette")
	}
}

func TestSpaceDismissesPaletteAndTypesThrough(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	m.input.SetValue("ls")
	m.input.CursorEnd()
	next, _ := m.Update(SuggestionsMsg{Seq: m.suggestSeq, S: &api.Suggestions{
		MenuItems: []api.Suggestion{{Label: "channels", Insert: "channels"}},
	}})
	m = next.(Model)
	if m.suggest == nil {
		t.Fatal("palette should be open")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m = next.(Model)
	if m.suggest != nil {
		t.Fatal("space must dismiss the palette")
	}
	if m.input.Value() != "ls " {
		t.Fatalf("the space must still type into the input, got %q", m.input.Value())
	}
}

func TestShiftRPrefillsTheLiveHandle(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)

	// Zero live handles: R types through like any rune.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = next.(Model)
	if m.input.Value() != "R" {
		t.Fatalf("with no handles R must type through, got %q", m.input.Value())
	}
	m.input.Reset()

	// One live handle on the newest turn → instant prefill.
	m.transcript.Append(api.Event{ID: 1, TurnID: 1, Kind: "system",
		Payload: []byte(`{"text":"x","reply_handle":"iota-1111"}`)})
	next, _ = m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = next.(Model)
	if m.input.Value() != "#iota-1111 " {
		t.Fatalf("shift+R must prefill the handle, got %q", m.input.Value())
	}
	m.input.Reset()

	// A newer turn with several live handles → the palette picker opens;
	// tab completes the selected one.
	m.transcript.Append(api.Event{ID: 2, TurnID: 2, Kind: "system",
		Payload: []byte(`{"text":"a","reply_handle":"mu-2222"}`)})
	m.transcript.Append(api.Event{ID: 3, TurnID: 2, Kind: "enhanced",
		Payload: []byte(`{"text":"b","reply_handle":"nu-3333"}`)})
	next, _ = m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = next.(Model)
	if m.suggest == nil || len(m.suggest.MenuItems) != 2 {
		t.Fatalf("several handles must open the picker, got %+v", m.suggest)
	}
	if m.input.Value() != "" {
		t.Fatalf("picker path must not prefill, got %q", m.input.Value())
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(Model)
	if m.input.Value() != "#mu-2222 " {
		t.Fatalf("tab must complete the picked handle, got %q", m.input.Value())
	}

	// Consumed handles never count as live: with turn 2 swept, the scan
	// falls back to turn 1's still-live handle (mirrors the server: the
	// sweep replaces older events with reply_consumed=true, so whatever
	// the payload flags say IS the truth).
	m.input.Reset()
	m.transcript.Replace(api.Event{ID: 2, TurnID: 2, Kind: "system",
		Payload: []byte(`{"text":"a","reply_handle":"mu-2222","reply_consumed":true}`)})
	m.transcript.Replace(api.Event{ID: 3, TurnID: 2, Kind: "enhanced",
		Payload: []byte(`{"text":"b","reply_handle":"nu-3333","reply_consumed":true}`)})
	next, _ = m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = next.(Model)
	if m.input.Value() != "#iota-1111 " {
		t.Fatalf("scan must fall back to the older live handle, got %q", m.input.Value())
	}

	// Everything consumed → R types through.
	m.input.Reset()
	m.transcript.Replace(api.Event{ID: 1, TurnID: 1, Kind: "system",
		Payload: []byte(`{"text":"x","reply_handle":"iota-1111","reply_consumed":true}`)})
	next, _ = m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = next.(Model)
	if m.input.Value() != "R" {
		t.Fatalf("all-consumed must type through, got %q", m.input.Value())
	}
}

// TestShiftUStagesLatestSuggestionOrTypesThrough mirrors
// TestShiftRPrefillsTheLiveHandle's shape: U at an empty prompt is a no-op
// affordance unless the newest ai answer carries a usable suggestion, and
// every no-match/gated case must let the keystroke type through (or type
// normally, for the non-empty-input case) untouched, never silently eat it.
func TestShiftUStagesLatestSuggestionOrTypesThrough(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)

	// Empty transcript: no ai event at all, so U types through.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = next.(Model)
	if m.input.Value() != "U" {
		t.Fatalf("with no suggestions U must type through, got %q", m.input.Value())
	}
	m.input.Reset()

	// The newest ai event carries one suggestion block → staged verbatim.
	m.transcript.Append(api.Event{ID: 1, TurnID: 1, Kind: api.KindAi, Payload: []byte(`{
		"status": "done",
		"blocks": [
			{"type": "text", "text": "Here's a pick."},
			{"type": "suggestion", "command": "show game 12", "note": "top pick"}
		]
	}`)})
	next, _ = m.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = next.(Model)
	if m.input.Value() != "show game 12" {
		t.Fatalf("shift+U must stage the suggestion verbatim, got %q", m.input.Value())
	}
	m.input.Reset()

	// Several suggestions in the same payload → the LAST one wins (old
	// answers predating 3.4.0's one-per-answer cap).
	m.transcript.Replace(api.Event{ID: 1, TurnID: 1, Kind: api.KindAi, Payload: []byte(`{
		"status": "done",
		"blocks": [
			{"type": "suggestion", "command": "show game 12"},
			{"type": "suggestion", "command": "ls games with genre RPG"}
		]
	}`)})
	next, _ = m.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = next.(Model)
	if m.input.Value() != "ls games with genre RPG" {
		t.Fatalf("several suggestions must resolve last-wins, got %q", m.input.Value())
	}
	m.input.Reset()

	// A newer, non-ai event riding after the answer must not hide it —
	// the backward scan skips it, mirroring LiveHandles' own robustness.
	m.transcript.Append(api.Event{ID: 2, TurnID: 2, Kind: "system", Payload: []byte(`{"text":"pong"}`)})
	next, _ = m.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = next.(Model)
	if m.input.Value() != "ls games with genre RPG" {
		t.Fatalf("a newer non-ai event must not hide the suggestion, got %q", m.input.Value())
	}
	m.input.Reset()

	// Non-empty input: U types normally, the staging gate never fires even
	// though a live suggestion exists.
	m.input.SetValue("x")
	m.input.CursorEnd()
	next, _ = m.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = next.(Model)
	if m.input.Value() != "xU" {
		t.Fatalf("with input already present U must type normally, got %q", m.input.Value())
	}
	m.input.Reset()

	// The newest ai event carries no suggestion block at all → types
	// through, same as the empty-transcript case above.
	m.transcript.Replace(api.Event{ID: 1, TurnID: 1, Kind: api.KindAi, Payload: []byte(`{
		"status": "done",
		"blocks": [{"type": "text", "text": "nothing to run"}]
	}`)})
	next, _ = m.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = next.(Model)
	if m.input.Value() != "U" {
		t.Fatalf("with no suggestions U must type through, got %q", m.input.Value())
	}
}

func TestScopeCyclersFollowTheWebRules(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	m.channels = []string{"@all", "@alpha", "@beta"}

	// Hint modes: exact port of the web's #mode().
	for input, want := range map[string]string{
		"":                 "",
		"analyze":          "period",
		"stats vids":       "period",
		"list vids":        "channel",
		"ls games sort id": "channel",
		"list channels":    "",
		"show game #1":     "",
	} {
		if got := hintMode(input); got != want {
			t.Errorf("hintMode(%q) = %q, want %q", input, got, want)
		}
	}

	// Shift+Tab cycles channels only while the channel hint is live.
	m.input.SetValue("list vids")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = next.(Model)
	if m.scopeChannel != "@alpha" { // "@all" is index 0 → next
		t.Fatalf("cycle from @all should land @alpha, got %q", m.scopeChannel)
	}
	if cmd == nil {
		t.Fatal("cycling must fire the persist PATCH")
	}
	// Wraps.
	m.scopeChannel = "@beta"
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = next.(Model)
	if m.scopeChannel != "@all" {
		t.Fatalf("cycle must wrap to @all, got %q", m.scopeChannel)
	}
	// Inert outside the hint.
	m.input.SetValue("show game #1")
	before := m.scopeChannel
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = next.(Model)
	if m.scopeChannel != before {
		t.Fatal("shift+tab must be inert without the channel hint")
	}

	// Ctrl+Space cycles periods during analyze.
	m.input.SetValue("analyze")
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModCtrl})
	m = next.(Model)
	if m.scopePeriod != "28d" {
		t.Fatalf("period cycle from 7d should land 28d, got %q", m.scopePeriod)
	}

	// The unknown-current rule: indexOf misses → treated as 0 → list[1].
	if got := cycleNext([]string{"@all", "@x", "@y"}, "none"); got != "@x" {
		t.Fatalf("unknown current must land list[1], got %q", got)
	}
}

func TestScopeParamsRideOnlyWithLiveHints(t *testing.T) {
	got := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat" && r.Method == http.MethodPost {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			got <- body
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"uuid":"u-1","turn_id":5}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	m, _ := newTestModel(t, nil, WithConversation("u-1"))
	client, err := api.New(srv.URL, filepath.Join(t.TempDir(), "cookies.json"))
	if err != nil {
		t.Fatal(err)
	}
	m.client = client
	m = sized(m)
	m.channels = []string{"@all", "@alpha"}
	m.scopeChannel = "@alpha"
	m.scopePeriod = "28d"

	send := func(text string) map[string]any {
		m.input.SetValue(text)
		next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = next.(Model)
		if cmd == nil {
			t.Fatalf("send %q produced no command", text)
		}
		cmd()
		return <-got
	}

	body := send("list vids")
	if body["channel"] != "@alpha" {
		t.Fatalf("list vids must carry the channel scope, got %v", body["channel"])
	}
	if _, has := body["period"]; has {
		t.Fatal("list vids must not carry a period")
	}

	body = send("analyze")
	if body["period"] != "28d" {
		t.Fatalf("analyze must carry the period scope, got %v", body["period"])
	}
	if _, has := body["channel"]; has {
		t.Fatal("analyze must not carry a channel")
	}

	body = send("show game #1")
	if _, has := body["channel"]; has {
		t.Fatal("unscoped verbs must not carry channel")
	}
	if _, has := body["period"]; has {
		t.Fatal("unscoped verbs must not carry period")
	}
}

func TestAiInputPatternMatchesTheWebRegex(t *testing.T) {
	// The prompt accent gates on the web's own pattern: /^\s*@ai\b/i.
	for _, in := range []string{"@ai hello", "  @AI what now", "@Ai", "@ai\twrapped"} {
		if !aiInputRe.MatchString(in) {
			t.Errorf("should match %q", in)
		}
	}
	for _, in := range []string{"mail@ai.dev", "ai hello", "@aim high", "say @ai later"} {
		if aiInputRe.MatchString(in) {
			t.Errorf("must not match %q", in)
		}
	}
}

// ── AI block/status streaming (event.ai_block / event.ai_status) ─────────

// pendingAiEvent starts a fresh pending "ai" event on the transcript — the
// event.append every ai_block/ai_status test builds on.
func pendingAiEvent(m Model, id, turnID int64, extraFields string) Model {
	payload := `{"status":"pending","blocks":[]` + extraFields + `}`
	return drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventAppend,
		Event: api.Event{ID: id, TurnID: turnID, Kind: api.KindAi, Payload: []byte(payload)},
	}})
}

func aiBlockMsg(eventID int64, index int, text string) CableEventMsg {
	return CableEventMsg{M: cable.StreamMessage{
		Type:    cable.TypeEventAiBlock,
		EventID: eventID,
		Index:   index,
		Block:   json.RawMessage(`{"type":"text","text":"` + text + `"}`),
	}}
}

func aiStatusMsg(eventID int64, text string) CableEventMsg {
	return CableEventMsg{M: cable.StreamMessage{
		Type:    cable.TypeEventAiStatus,
		EventID: eventID,
		Text:    text,
	}}
}

// rawAiPayload peeks at an event's current raw payload bytes without
// mutating it (MutateEventPayload's fn returns ok=false).
func rawAiPayload(t *testing.T, m Model, eventID int64) map[string]any {
	t.Helper()
	var raw json.RawMessage
	m.transcript.MutateEventPayload(eventID, func(p json.RawMessage) (json.RawMessage, bool) {
		raw = append(json.RawMessage(nil), p...)
		return nil, false
	})
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("payload for event %d did not decode: %v (raw: %s)", eventID, err, raw)
	}
	return fields
}

func TestAiBlocksStreamInOrder(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	m = pendingAiEvent(m, 50, 20, "")

	m = drive(m, aiBlockMsg(50, 0, "Block0"))
	m = drive(m, aiBlockMsg(50, 1, "Block1"))

	view := m.viewContent()
	i0, i1 := strings.Index(view, "Block0"), strings.Index(view, "Block1")
	if i0 == -1 || i1 == -1 {
		t.Fatalf("both blocks must render:\n%s", view)
	}
	if i0 >= i1 {
		t.Errorf("blocks must render in index order, got Block0@%d Block1@%d", i0, i1)
	}
}

func TestAiBlocksStreamOutOfOrder(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	m = pendingAiEvent(m, 55, 21, "")

	// Index 1 lands before index 0 — the slot for 0 gets padded, then
	// backfilled once its own message arrives.
	m = drive(m, aiBlockMsg(55, 1, "Block1"))
	m = drive(m, aiBlockMsg(55, 0, "Block0"))

	view := m.viewContent()
	i0, i1 := strings.Index(view, "Block0"), strings.Index(view, "Block1")
	if i0 == -1 || i1 == -1 {
		t.Fatalf("both blocks must render:\n%s", view)
	}
	if i0 >= i1 {
		t.Errorf("out-of-order arrival must still render index order, got Block0@%d Block1@%d", i0, i1)
	}
}

func TestAiStatusLineReplacesEllipsisThenUpdates(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	m = pendingAiEvent(m, 51, 22, "")

	if !strings.Contains(m.viewContent(), "…") {
		t.Fatalf("pending ai turn must show the bare ellipsis before any status:\n%s", m.viewContent())
	}

	m = drive(m, aiStatusMsg(51, "Scouring the internet…"))
	view := m.viewContent()
	if !strings.Contains(view, "Scouring the internet…") {
		t.Fatalf("status line must replace the ellipsis:\n%s", view)
	}

	m = drive(m, aiStatusMsg(51, "Crunching numbers…"))
	view = m.viewContent()
	if strings.Contains(view, "Scouring the internet…") {
		t.Errorf("a stale status line must not survive a newer one:\n%s", view)
	}
	if !strings.Contains(view, "Crunching numbers…") {
		t.Fatalf("second status line must land:\n%s", view)
	}
}

func TestAiReplaceWinsWhollyOverStreamedStateAndStatus(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	m = pendingAiEvent(m, 52, 23, "")
	m = drive(m, aiBlockMsg(52, 0, "Streamed"))
	m = drive(m, aiStatusMsg(52, "Scouring the internet…"))

	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type: cable.TypeEventReplace,
		Event: api.Event{ID: 52, TurnID: 23, Kind: api.KindAi, Payload: []byte(
			`{"status":"done","blocks":[{"type":"text","text":"Final answer"}],"model":"opus"}`)},
	}})

	view := m.viewContent()
	if strings.Contains(view, "Streamed") {
		t.Errorf("streamed blocks must not survive the final replace:\n%s", view)
	}
	if strings.Contains(view, "Scouring the internet") {
		t.Errorf("status line must not survive the final replace:\n%s", view)
	}
	if !strings.Contains(view, "Final answer") {
		t.Fatalf("the replace payload must render:\n%s", view)
	}

	fields := rawAiPayload(t, m, 52)
	if _, has := fields["status_line"]; has {
		t.Error("status_line must not survive the final replace — the server payload never carries it")
	}
}

func TestAiBlockAndStatusForUnknownEventIDAreIgnored(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)

	// No event.append for 999 — these race ahead of it and must be
	// dropped silently (and, above all, not panic).
	m = drive(m, aiBlockMsg(999, 0, "orphan"))
	m = drive(m, aiStatusMsg(999, "orphan status"))

	if m.transcript.Len() != 0 {
		t.Errorf("an unknown event id must not create a transcript entry, got %d events", m.transcript.Len())
	}
	if strings.Contains(m.viewContent(), "orphan") {
		t.Errorf("an orphan block/status must not render:\n%s", m.viewContent())
	}
}

func TestAiBlockAppendPreservesUnknownPayloadFields(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	m = pendingAiEvent(m, 53, 24, `,"sentinel":"keep-me","anchor_event_id":7`)

	m = drive(m, aiBlockMsg(53, 0, "hi"))

	fields := rawAiPayload(t, m, 53)
	if fields["sentinel"] != "keep-me" {
		t.Errorf("unknown top-level field must round-trip untouched, got %v", fields["sentinel"])
	}
	if got, want := fields["anchor_event_id"], float64(7); got != want {
		t.Errorf("anchor_event_id must round-trip, got %v want %v", got, want)
	}
}

// ── containment law (2.0.0) ────────────────────────────────────────────

// TestContainmentCapsWideTerminalsChromeIncluded is the resize
// proof for the owner-locked containment law: EVERYTHING — message
// blocks and the bottom chrome alike — renders LEFT-ANCHORED inside
// min(terminalWidth−2, render.ContentCap) columns as one coherent
// column (owner 2.0.0 smoke: no exemptions; the status bar right-aligns
// WITHIN the column). A long message forces real word-wrap so the
// assertion actually exercises the cap rather than passing on a short
// fixture that would fit either way.
func TestContainmentCapsWideTerminalsChromeIncluded(t *testing.T) {
	if !widthCapEnabled {
		t.Skip("width cap disabled by owner ruling 2026-07-12 — the contract below re-arms with the flag")
	}
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = drive(m, tea.WindowSizeMsg{Width: 220, Height: 24})
	m = drive(m, m.fetchChatCmd("u1", false)())

	if got := m.contentWidth(); got != render.ContentCap {
		t.Fatalf("contentWidth at 220 cols = %d, want the %d-col cap", got, render.ContentCap)
	}

	long := strings.Repeat("wide terminal word ", 20) // ~380 chars — wraps very differently at 100 vs 218
	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type: cable.TypeEventAppend,
		Event: api.Event{
			ID: 50, TurnID: 50, Kind: "system",
			Payload:   []byte(`{"text":"` + long + `"}`),
			CreatedAt: fixedNow,
		},
	}})

	content := m.transcript.View(m.contentWidth())
	for _, line := range strings.Split(content, "\n") {
		if w := lipgloss.Width(line); w > render.ContentCap {
			t.Errorf("message line wider than the containment cap (%d > %d): %q", w, render.ContentCap, line)
		}
	}

	// Owner 2.0.0 smoke (2026-07-12): the status bar follows the SAME
	// content column as everything else — one coherent column on wide
	// terminals, the margin left to the star-field.
	if w := lipgloss.Width(m.statusLine()); w > render.ContentCap {
		t.Errorf("status line width = %d, escaped the content column (cap %d)", w, render.ContentCap)
	}
}

// TestContainmentLeavesNarrowTerminalsExactlyAsToday is the regression
// proof: below the cap's bite point (terminalWidth ≤ ContentCap+2 =
// 102) contentWidth returns the raw terminal width, unmargined — a
// 60-col terminal renders exactly as it did before this law existed,
// and the 80×24 golden frames (also under 102) never move.
func TestContainmentLeavesNarrowTerminalsExactlyAsToday(t *testing.T) {
	// Holds with the cap on OR off: narrow terminals always get raw width.
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = drive(m, tea.WindowSizeMsg{Width: 60, Height: 24})

	if got, want := m.contentWidth(), 60; got != want {
		t.Fatalf("contentWidth at 60 cols = %d, want %d (the cap must not bite below 102)", got, want)
	}
}

// TestViewportWidthPixelsFollowTheCappedColumn proves the wire
// contract: viewport_width POSTed to the server is contentWidth()×8
// pixels — the server sizes its own tables to OUR column, not the raw
// terminal — at both a narrow terminal (cap inert, 60 cols) and a wide
// one (cap biting, 220 cols).
func TestViewportWidthPixelsFollowTheCappedColumn(t *testing.T) {
	for _, width := range []int{60, 220} {
		got := make(chan map[string]any, 1)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/chat" && r.Method == http.MethodPost {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				got <- body
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"uuid":"u-1","turn_id":5}`))
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		client, err := api.New(srv.URL, filepath.Join(t.TempDir(), "cookies.json"))
		if err != nil {
			srv.Close()
			t.Fatal(err)
		}
		m, _ := newTestModel(t, nil, WithConversation("u-1"))
		m.client = client
		m = drive(m, tea.WindowSizeMsg{Width: width, Height: 24})

		wantWidth := m.contentWidth()
		m.input.SetValue("ping")
		next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = next.(Model)
		if cmd == nil {
			t.Fatalf("send at width %d produced no command", width)
		}
		cmd()
		body := <-got
		if want := float64(wantWidth * 8); body["viewport_width"] != want {
			t.Errorf("width %d: viewport_width = %v, want %v (contentWidth()*8 = %d*8)", width, body["viewport_width"], want, wantWidth)
		}
		srv.Close()
	}
}

// TestResumeFetchSendsViewportDerivedLimit pins viewportRows' composition
// at the resume call site (fetchResumeCmd, owner 2026-07-15,
// viewport-driven paging): the outgoing GET /resume.json limit is
// viewportRows(m.height - 4), not a fixed page size — see fetchResumeCmd's
// doc comment.
func TestResumeFetchSendsViewportDerivedLimit(t *testing.T) {
	var gotLimit string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resumeJSON))
	})
	m, _ := newTestModel(t, mux)
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 30})

	m = drive(m, m.fetchResumeCmd()())

	want := itoa(viewportRows(m.height - 4))
	if gotLimit != want {
		t.Errorf("limit = %q, want %q (viewportRows(%d - 4))", gotLimit, want, m.height)
	}
}
