package ui

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// seedHitPickerScenario builds a transcript with:
//   - an early "anchor" turn (event id 501) a hit can resolve to;
//   - 40 filler turns pushing it well off-screen, mirroring seedTurns'
//     own n=40 in scrollnav_test.go — enough that a jump to the anchor is
//     a real scroll (not a no-op on a transcript that already fits the
//     viewport), and that the anchor is provably not "at the bottom";
//   - the NEWEST turn: a "system" event carrying a REAL hits card (the
//     table_heading/table_rows shape
//     Pito::MessageBuilder::Conversation::Hits builds — pito
//     lib/pito/message_builder/conversation/hits.rb — like mode, two
//     score-cell rows) — row 0 resolves to the anchor above (jumpable),
//     row 1's anchor (event id 999) is never appended, simulating a
//     cross-conversation hit (hitpicker.go's "(elsewhere)" case).
func seedHitPickerScenario(m *Model) {
	m.transcript.Append(api.Event{
		ID: 501, TurnID: 501, Kind: "system",
		Payload: []byte(`{"text":"Hades II hit a new peak CCU this week."}`),
	})
	for i := 0; i < 40; i++ {
		m.transcript.Append(api.Event{
			ID: int64(600 + i), TurnID: int64(600 + i), Kind: "system",
			Payload: []byte(`{"text":"filler"}`),
		})
	}
	m.transcript.Append(api.Event{
		ID: 900, TurnID: 900, Kind: "system",
		Payload: []byte(`{
			"body": "2 conversations found.",
			"html": true,
			"table_heading": ["Conversation", "Score"],
			"table_rows": [
				{
					"cells": [
						{
							"text": "Hades II thoughts",
							"class": "pito-action-shimmer pito-shimmer-d5 pito-cell-title",
							"data": {
								"controller": "pito--chat-prefill",
								"action": "click->pito--chat-prefill#fill",
								"pito--chat-prefill-text-value": "/resume conv-uuid-1",
								"pito--chat-prefill-submit-value": "true"
							}
						},
						{"score": 87}
					],
					"data": {"anchor_event_id": 501, "conversation_uuid": "conv-uuid-1"}
				},
				{
					"cells": [
						{
							"text": "Hades II vs Hades I",
							"class": "pito-action-shimmer pito-shimmer-d2 pito-cell-title",
							"data": {
								"controller": "pito--chat-prefill",
								"action": "click->pito--chat-prefill#fill",
								"pito--chat-prefill-text-value": "/resume conv-uuid-2",
								"pito--chat-prefill-submit-value": "true"
							}
						},
						{"score": 42}
					],
					"data": {"anchor_event_id": 999, "conversation_uuid": "conv-uuid-2"}
				}
			],
			"list_footer": "Reply with a hit's row number."
		}`),
	})
	m.refreshViewport()
}

// chatSinkServer serves POST /chat, forwarding each request's raw `input`
// onto sent (buffered — the caller reads it back right after driving the
// returned send command, entitypicker_test.go's own pattern) and
// acknowledging with a bare turn ack, same minimal shape entityServer
// uses.
func chatSinkServer(t *testing.T, sent chan string) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		sent <- body.Input
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"turn_id":7}`))
	})
	return mux
}

// TestShiftJOpensHitPickerOrTypesThrough mirrors
// TestShiftRPrefillsTheLiveHandle's shape (model_test.go): J at an empty
// prompt is a no-op affordance unless its gate is live, and every no-match
// case must let the keystroke type through untouched, never silently eat
// it.
func TestShiftJOpensHitPickerOrTypesThrough(t *testing.T) {
	m, _ := newTestModel(t, nil, WithConversation("u-1"))
	m = sized(m)

	// Empty transcript: no latest event at all, so J types through.
	next, _ := m.Update(key("J"))
	m = next.(Model)
	if m.input.Value() != "J" || m.mode != modeChat {
		t.Fatalf("with an empty transcript J must type through, got value=%q mode=%v", m.input.Value(), m.mode)
	}
	m.input.Reset()

	// The newest event is kind "system" but carries no conversation_hits:
	// still falls through.
	m.transcript.Append(api.Event{ID: 1, TurnID: 1, Kind: "system", Payload: []byte(`{"text":"pong"}`)})
	m.refreshViewport()
	next, _ = m.Update(key("J"))
	m = next.(Model)
	if m.input.Value() != "J" || m.mode != modeChat {
		t.Fatalf("with no hits on the latest event J must type through, got value=%q mode=%v", m.input.Value(), m.mode)
	}
	m.input.Reset()

	// The newest event is a non-"system" kind (echo): also falls through,
	// even though an earlier turn in the same transcript carries hits.
	m.transcript.Append(api.Event{ID: 2, TurnID: 2, Kind: "echo", Payload: []byte(`{"text":"ls games"}`)})
	m.refreshViewport()
	next, _ = m.Update(key("J"))
	m = next.(Model)
	if m.input.Value() != "J" || m.mode != modeChat {
		t.Fatalf("with a non-system latest event J must type through, got value=%q mode=%v", m.input.Value(), m.mode)
	}
	m.input.Reset()

	// The newest event carries hits: J opens the picker instead of typing,
	// with rows precomputed against the loaded transcript.
	seedHitPickerScenario(&m)
	next, _ = m.Update(key("J"))
	m = next.(Model)
	if m.mode != modeHitPicker {
		t.Fatalf("mode = %v, want modeHitPicker", m.mode)
	}
	if m.input.Value() != "" {
		t.Fatalf("the picker path must not type into the input, got %q", m.input.Value())
	}
	if len(m.hitPick.rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(m.hitPick.rows))
	}
	if !m.hitPick.rows[0].jumpable {
		t.Error("row 0's anchor (event 501) is loaded — must be jumpable")
	}
	if m.hitPick.rows[1].jumpable {
		t.Error("row 1's anchor (event 999) was never appended — must NOT be jumpable")
	}
}

// TestHitPickerDigitJumpsToLoadedAnchor covers the happy path: a digit on
// a row whose anchor IS in the loaded transcript closes the overlay,
// scrolls straight to the anchor turn's start (Transcript.EventLineRange),
// and releases follow — mirroring ctrl+home's own
// setFollow(m.sc.AtBottom()) rule (scrollnav_test.go's
// TestCtrlHomeAndCtrlEndJump asserts the same shape: offset landed, follow
// off).
func TestHitPickerDigitJumpsToLoadedAnchor(t *testing.T) {
	m, _ := newTestModel(t, nil, WithConversation("u-1"))
	m = sized(m)
	seedHitPickerScenario(&m)

	next, _ := m.Update(key("J"))
	m = next.(Model)
	if m.mode != modeHitPicker {
		t.Fatalf("setup: picker did not open, mode = %v", m.mode)
	}

	wantStart, _, ok := m.transcript.EventLineRange(501)
	if !ok {
		t.Fatal("setup: anchor event 501 must resolve in the loaded transcript")
	}

	next, _ = m.Update(key("1"))
	m = next.(Model)

	if m.mode != modeChat {
		t.Fatalf("digit jump must close the picker, mode = %v", m.mode)
	}
	if len(m.hitPick.rows) != 0 {
		t.Fatalf("digit jump must reset the picker state, rows = %+v", m.hitPick.rows)
	}
	if m.sc.YOffset() != wantStart {
		t.Fatalf("yoffset = %d, want %d (EventLineRange(501)'s start)", m.sc.YOffset(), wantStart)
	}
	if m.follow {
		t.Error("a deliberate jump must release follow mode")
	}
	if len(m.notices) != 0 {
		t.Fatalf("a successful jump must not push a notice, got %v", m.notices)
	}
}

// TestHitPickerUnloadedAnchorRendersElsewhereAndSubmitsResume covers the
// cross-conversation row: it renders the dimmed "(elsewhere)" marker up
// front, and picking it closes the picker AND submits `/resume
// <conversation_uuid>` through the real send path — the exact command a
// click on the web's conversation-name cell types+submits (hits.rb's
// name_cell doc comment) — rather than the dead "jump isn't wired for it
// yet" notice this affordance used to leave behind before the picker had
// a uuid to act on (see jumpToHit/resumeHit, hitpicker.go). Row 1's hit
// (seedHitPickerScenario) carries a non-nil Score (like mode), so the
// submitted command carries no trailing anchor id.
func TestHitPickerUnloadedAnchorRendersElsewhereAndSubmitsResume(t *testing.T) {
	sent := make(chan string, 1)
	m, _ := newTestModel(t, chatSinkServer(t, sent), WithConversation("u-1"))
	m = sized(m)
	seedHitPickerScenario(&m)

	next, _ := m.Update(key("J"))
	m = next.(Model)
	if m.mode != modeHitPicker {
		t.Fatalf("setup: picker did not open, mode = %v", m.mode)
	}

	view := ansi.Strip(m.hitPickerView())
	if !strings.Contains(view, "(elsewhere)") {
		t.Errorf("the unresolvable row must render the elsewhere marker:\n%s", view)
	}

	next, cmd := m.Update(key("2"))
	m = next.(Model)

	if m.mode != modeChat {
		t.Fatalf("picking an unloaded row must still close the picker, mode = %v", m.mode)
	}
	if len(m.hitPick.rows) != 0 {
		t.Fatalf("picking a row must reset the picker state, rows = %+v", m.hitPick.rows)
	}
	if cmd == nil {
		t.Fatal("picking an unloaded row must produce the /resume send command")
	}
	m = drive(m, cmd())

	select {
	case got := <-sent:
		want := "/resume conv-uuid-2"
		if got != want {
			t.Fatalf("submitted input = %q, want %q", got, want)
		}
	default:
		t.Fatal("picking an unloaded row must SEND /resume (web conversation-name click submits too)")
	}
	if len(m.histEntries) == 0 || m.histEntries[0] != "/resume conv-uuid-2" {
		t.Fatalf("the submitted /resume must enter input history: %#v", m.histEntries)
	}
}

// TestGoldenHitPicker pins the overlay's own view — the picker replaces
// the whole frame while open (model.go's View), so this reuses
// goldenFrame (golden_test.go) exactly like the five main-screen goldens,
// just gated on modeHitPicker instead of a fetch. Regenerate with
// `go test ./internal/ui/ -run TestGoldenHitPicker -update`.
func TestGoldenHitPicker(t *testing.T) {
	m, _ := newTestModel(t, nil, WithConversation("u1"))
	m = sized(m)
	seedHitPickerScenario(&m)

	next, _ := m.Update(key("J"))
	m = next.(Model)
	if m.mode != modeHitPicker {
		t.Fatalf("setup: picker did not open, mode = %v", m.mode)
	}

	goldenFrame(t, m)
}
