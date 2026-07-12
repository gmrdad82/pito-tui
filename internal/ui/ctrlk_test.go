package ui

import (
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The web contract under test: command_palette_controller.js + CommandCatalog.
func TestCtrlKOpensFiltersInsertsWithoutSubmitting(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m = next.(Model)
	if m.mode != modeCommandPalette {
		t.Fatal("ctrl+k must open the command palette")
	}
	m = driveAnim(t, m, 60) // let the open drawer spring settle
	view := ansi.Strip(m.viewContent())
	if !strings.Contains(view, "Commands") || !strings.Contains(view, "/config ai") {
		t.Fatalf("palette must show pito's own title and the catalog:\n%s", view)
	}

	// Fuzzy subsequence filter: "cfgai" matches "AI provider & model"?
	// No — subsequence runs on the LABEL. "aipro" does.
	for _, ru := range "aipro" {
		next, _ = m.Update(tea.KeyPressMsg{Code: ru, Text: string(ru)})
		m = next.(Model)
	}
	items := m.ctrlKVisibleItems()
	if len(items) != 1 || items[0].insert != "/config ai" {
		t.Fatalf("fuzzy filter must isolate the AI provider item, got %#v", items)
	}

	// Enter pre-fills the prompt (placeholders included) and does NOT send.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if got := m.input.Value(); got != "/config ai" {
		t.Fatalf("enter must pre-fill the prompt, got %q", got)
	}
	// The close is spring-deferred (notifications-style): drive ticks
	// until the drawer settles back into chat.
	m = driveAnim(t, m, 60)
	if m.mode != modeChat {
		t.Fatalf("palette must settle back into chat, mode=%v", m.mode)
	}
}

func TestCtrlKAuthGateAndEscape(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	m.needsLogin = true

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m = next.(Model)
	m = driveAnim(t, m, 60) // settle the open spring before closing
	items := m.ctrlKVisibleItems()
	if len(items) != 1 || items[0].insert != "/login <code>" {
		t.Fatalf("unauthenticated palette must offer only /login, got %#v", items)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	m = driveAnim(t, m, 60)
	if m.mode != modeChat {
		t.Fatal("esc must close the palette")
	}
	if m.input.Value() != "" {
		t.Fatal("esc must not touch the prompt")
	}
}

func TestCtrlKFuzzyIsSubsequenceMatch(t *testing.T) {
	if !ctrlKFuzzy("", "anything") {
		t.Error("empty query matches everything")
	}
	if !ctrlKFuzzy("gocred", "Google OAuth credentials") {
		t.Error("in-order subsequence must match")
	}
	if ctrlKFuzzy("credgo", "Google OAuth credentials") {
		t.Error("out-of-order characters must not match")
	}
}
