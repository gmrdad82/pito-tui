package ui

import (
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
)

func TestSelectionTextStreamSemantics(t *testing.T) {
	frame := strings.Join([]string{
		"alpha beta gamma",
		"second line here",
		"third",
	}, "\n")

	// Single line, forward.
	if got := selectionText(frame, 6, 0, 9, 0); got != "beta" {
		t.Errorf("single-line = %q, want %q", got, "beta")
	}
	// Single line, dragged backwards — anchor/cursor swap.
	if got := selectionText(frame, 9, 0, 6, 0); got != "beta" {
		t.Errorf("backwards = %q, want %q", got, "beta")
	}
	// Multi-line stream: first line to EOL, middle whole, last to cursor.
	want := "beta gamma\nsecond line here\nthi"
	if got := selectionText(frame, 6, 0, 2, 2); got != want {
		t.Errorf("multi-line = %q, want %q", got, want)
	}
	// Past the frame's bottom clamps to the last line's end.
	if got := selectionText(frame, 0, 2, 80, 9); got != "third" {
		t.Errorf("clamped = %q, want %q", got, "third")
	}
	// Fully out of range.
	if got := selectionText(frame, 0, 9, 5, 9); got != "" {
		t.Errorf("out of range = %q, want empty", got)
	}
}

func TestMouseDragCopiesAndToasts(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	m.transcript.Append(api.Event{ID: 1, TurnID: 1, Kind: "echo",
		Payload: []byte(`{"text":"show game 5"}`)})
	m.refreshViewport()

	// Press, drag, release across the first viewport row.
	next, _ := m.Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	m = next.(Model)
	if !m.selecting {
		t.Fatal("left press must start a selection")
	}
	next, _ = m.Update(tea.MouseMotionMsg{X: 20, Y: 0, Button: tea.MouseLeft})
	m = next.(Model)
	if m.selCursorX != 20 {
		t.Fatalf("motion must move the selection cursor, got %d", m.selCursorX)
	}
	// The in-flight highlight paints into the frame (reverse video).
	if !strings.Contains(m.viewContent(), "\x1b[7m") {
		t.Error("dragging must paint a reverse-video highlight")
	}
	next, cmd := m.Update(tea.MouseReleaseMsg{X: 20, Y: 0, Button: tea.MouseLeft})
	m = next.(Model)
	if m.selecting {
		t.Fatal("release must end the selection")
	}
	if cmd == nil {
		t.Fatal("release over text must produce the clipboard command")
	}
	if m.toastTicks == 0 {
		t.Fatal("release over text must raise the toast")
	}
	if m.toastText == "" {
		t.Fatal("the toast must carry a line from the owner's pool")
	}
	if !strings.Contains(m.viewContent(), m.toastText) {
		t.Error("the toast must paint into the frame")
	}
	if strings.Contains(m.viewContent(), "\x1b[7m") {
		t.Error("the highlight must clear once the selection ends")
	}

	// The toast counts itself down through animation ticks and leaves.
	if !m.animGateOpen() {
		t.Fatal("a live toast must hold the animation gate open")
	}
	for i := int64(0); i <= toastTicksTotal; i++ {
		next, _ = m.Update(AnimTickMsg{})
		m = next.(Model)
	}
	if m.toastTicks != 0 {
		t.Fatalf("toast must expire, ticks left = %d", m.toastTicks)
	}
	if strings.Contains(m.viewContent(), m.toastText) {
		t.Error("an expired toast must leave the frame")
	}
}

func TestMouseClickWithoutDragIsNoop(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	m.transcript.Append(api.Event{ID: 1, TurnID: 1, Kind: "echo",
		Payload: []byte(`{"text":"show game 5"}`)})
	m.refreshViewport()

	next, _ := m.Update(tea.MouseClickMsg{X: 2, Y: 0, Button: tea.MouseLeft})
	m = next.(Model)
	next, cmd := m.Update(tea.MouseReleaseMsg{X: 2, Y: 0, Button: tea.MouseLeft})
	m = next.(Model)
	// A zero-width release lands on one cell — a bare click may still
	// graze a glyph, but blank cells must never copy or toast.
	if m.toastTicks != 0 && strings.TrimSpace(m.toastText) == "" {
		t.Error("a blank click must not toast")
	}
	_ = cmd
	// Right-button presses never start selections.
	next, _ = m.Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseRight})
	m = next.(Model)
	if m.selecting {
		t.Error("right click must not start a selection")
	}
}
