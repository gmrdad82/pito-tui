package ui

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/cable"
)

// Golden frames pin the five main screens. Determinism recipe: Ascii color
// profile (TestMain), plain renderer (no glamour variance), fixed clock,
// fixed 80×24 size, spinner pre-tick frame. Update goldens with
// `go test ./internal/ui/ -run TestGolden -update`.

func goldenFrame(t *testing.T, m Model) {
	t.Helper()
	view := m.View()
	// Frames must fit the terminal: no line may exceed the width the
	// model was sized to.
	for _, line := range strings.Split(view, "\n") {
		if w := len([]rune(line)); w > 80 {
			t.Errorf("line wider than terminal (%d): %q", w, line)
		}
	}
	// Normalize the one nondeterministic bit — the httptest port in the
	// status line — and the padding that varies with it.
	view = testPortRe.ReplaceAllString(view, "127.0.0.1:0")
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	golden.RequireEqual(t, []byte(strings.Join(lines, "\n")))
}

var testPortRe = regexp.MustCompile(`127\.0\.0\.1:\d+`)

func TestGoldenPicker(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t))
	m = sized(m)
	m = drive(m, m.fetchResumeCmd()())
	goldenFrame(t, m)
}

func TestGoldenEmptyConversation(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithNewConversation())
	m = sized(m)
	goldenFrame(t, m)
}

func TestGoldenStreamingTurn(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())
	// A send was acked (turn 9) and its first event arrived; a second send
	// (turn 10) is still pending — spinner line at the tail.
	m = drive(m,
		CableEventMsg{M: cable.StreamMessage{
			Type:  cable.TypeEventAppend,
			Event: api.Event{ID: 5, TurnID: 9, Kind: "echo", Payload: []byte(`{"text":"show game 5"}`), CreatedAt: fixedNow},
		}},
		CableEventMsg{M: cable.StreamMessage{
			Type:  cable.TypeEventAppend,
			Event: api.Event{ID: 6, TurnID: 9, Kind: "thinking", Payload: []byte(`{"resolved":false}`), CreatedAt: fixedNow},
		}},
		SendResultMsg{Res: &api.SendResult{TurnID: 10}},
	)
	goldenFrame(t, m)
}

func TestGoldenReplacedEvent(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())
	m = drive(m,
		CableEventMsg{M: cable.StreamMessage{
			Type:  cable.TypeEventAppend,
			Event: api.Event{ID: 5, TurnID: 9, Kind: "confirmation", Payload: []byte(`{"body":"Unlink Hades II from vid 12?","reply_handle":"@confirm-1"}`), CreatedAt: fixedNow},
		}},
		CableEventMsg{M: cable.StreamMessage{
			Type:  cable.TypeEventReplace,
			Event: api.Event{ID: 5, TurnID: 9, Kind: "confirmation", Payload: []byte(`{"body":"Unlink Hades II from vid 12?","resolved":true,"outcome_text":"Unlinked."}`), CreatedAt: fixedNow},
		}},
	)
	goldenFrame(t, m)
}

func TestGoldenDisconnectedBanner(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())
	m = drive(m, ConnStateMsg{State: cable.StateDisconnected})
	goldenFrame(t, m)
}

// TestFullProgramBoots runs the real Bubble Tea program once through
// teatest: window size in, first frame out, clean quit. The golden frames
// above pin layouts; this pins the program wiring.
func TestFullProgramBoots(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte("pong"))
	}, teatest.WithDuration(5*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
	if _, err := io.ReadAll(tm.FinalOutput(t)); err != nil {
		t.Fatal(err)
	}
}
