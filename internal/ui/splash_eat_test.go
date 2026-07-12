package ui

import (
	"net/http"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSplashEndsAndStopsEatingKeys(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"), WithTruecolor(true), WithSplash(true))
	m = sized(m)
	if !m.splashActive() {
		t.Fatal("setup: splash should be active after first-ready")
	}
	// Drive 3 seconds of 16ms ticks — the splash holds 800ms then rises
	// ~240ms; it must be LONG gone.
	for i := 0; i < 190; i++ {
		next, _ := m.Update(AnimTickMsg{})
		m = next.(Model)
	}
	if m.splashActive() {
		t.Fatalf("splash still active after 3s of ticks: %+v", m.splash)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = next.(Model)
	if got := m.input.Value(); got != "g" {
		t.Fatalf("first key after splash must type, got input %q", got)
	}
}
