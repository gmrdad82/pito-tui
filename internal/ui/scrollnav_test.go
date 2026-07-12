package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gmrdad82/pito-tui/internal/api"
)

func TestScrollNavTextInterpolatesLikeTheWeb(t *testing.T) {
	// The web's #format contract: %{count}, %{direction}, then
	// {singular|plural} resolving on count == 1.
	for _, tc := range []struct {
		variant string
		count   int
		dir     string
		want    string
	}{
		{"%{count} %{direction}", 7, "above", "7 above"},
		{"%{count} {message|messages} %{direction}", 1, "below", "1 message below"},
		{"%{count} {message|messages} %{direction}", 3, "above", "3 messages above"},
		{"%{direction}: %{count} {message|messages}", 2, "below", "below: 2 messages"},
	} {
		if got := scrollNavText(tc.variant, tc.count, tc.dir); got != tc.want {
			t.Errorf("scrollNavText(%q, %d, %s) = %q, want %q", tc.variant, tc.count, tc.dir, got, tc.want)
		}
	}
}

// seedTurns appends n one-line system turns straight onto the transcript.
func seedTurns(m *Model, n int) {
	for i := 0; i < n; i++ {
		m.transcript.Append(api.Event{
			ID: int64(1000 + i), TurnID: int64(1000 + i), Kind: "message",
			Payload: []byte(`{"text":"row"}`),
		})
	}
	m.refreshViewport()
}

func TestTurnsOutsideCountsFullyHiddenTurns(t *testing.T) {
	m, _ := newTestModel(t, nil, WithConversation("u-1"))
	m = sized(m)
	seedTurns(&m, 40)

	height := m.sc.VisibleLineCount()
	m.sc.GotoBottom()
	above, below := m.transcript.TurnsOutside(m.contentWidth(), m.sc.YOffset(), height)
	if below != 0 || above == 0 {
		t.Fatalf("at bottom: above=%d below=%d — below must be 0, above > 0", above, below)
	}

	m.sc.GotoTop()
	above, below = m.transcript.TurnsOutside(m.contentWidth(), m.sc.YOffset(), height)
	if above != 0 || below == 0 {
		t.Fatalf("at top: above=%d below=%d — above must be 0, below > 0", above, below)
	}
}

func TestScrollNavPillsRenderAndHide(t *testing.T) {
	m, _ := newTestModel(t, nil, WithConversation("u-1"))
	m = sized(m)
	seedTurns(&m, 40)
	m.sc.GotoTop()
	m.setFollow(false)

	top, bottom := m.scrollNavPills()
	if top != "" {
		t.Fatalf("at the very top nothing is above, got %q", top)
	}
	if bottom == "" {
		t.Fatal("scrolled to top with 40 turns: the bottom pill must show")
	}
	plain := ansi.Strip(bottom)
	if !strings.Contains(plain, "ctrl+end") || !strings.Contains(plain, "▼") {
		t.Fatalf("bottom pill must carry the token and the server's glyph: %q", plain)
	}
	if !strings.Contains(plain, "below") {
		t.Fatalf("bottom pill must interpolate direction below: %q", plain)
	}

	// The pills float over the rendered conversation frame.
	view := ansi.Strip(m.viewContent())
	if !strings.Contains(view, "ctrl+end") {
		t.Fatalf("view must paint the bottom pill:\n%s", view)
	}

	// The suggest palette hides both pills (the web hides while its
	// palette overlays are open).
	m.suggest = &api.Suggestions{MenuItems: []api.Suggestion{{Label: "/help"}}}
	if top, bottom = m.scrollNavPills(); top != "" || bottom != "" {
		t.Fatal("pills must hide while the suggest palette is open")
	}
	m.suggest = nil

	// Mid-scroll shows both, with distinct variants when the pool allows.
	m.sc.SetYOffset(m.sc.TotalLineCount() / 2)
	top, bottom = m.scrollNavPills()
	if top == "" || bottom == "" {
		t.Fatal("mid-scroll must show both pills")
	}
	if !strings.Contains(ansi.Strip(top), "ctrl+home") || !strings.Contains(ansi.Strip(top), "▲") {
		t.Fatalf("top pill anatomy wrong: %q", ansi.Strip(top))
	}
}

func TestCtrlHomeAndCtrlEndJump(t *testing.T) {
	m, _ := newTestModel(t, nil, WithConversation("u-1"))
	m = sized(m)
	seedTurns(&m, 40)
	m.sc.GotoBottom()
	m.setFollow(true)

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl})
	m = next.(Model)
	if m.sc.YOffset() != 0 || m.follow {
		t.Fatalf("ctrl+home must land at the top with follow off, yoff=%d follow=%v", m.sc.YOffset(), m.follow)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModCtrl})
	m = next.(Model)
	if !m.follow || !m.scrollEasing {
		t.Fatal("ctrl+end must re-engage follow and arm the glide")
	}
	// Drive the glide to its landing (more than two screens away lands
	// instantly on the first tick — easeTowardBottom's own rule).
	for i := 0; i < 200 && m.scrollEasing; i++ {
		m.easeTowardBottom()
	}
	if !m.sc.AtBottom() {
		t.Fatalf("the glide must land at the bottom, yoff=%d of %d", m.sc.YOffset(), m.sc.TotalLineCount())
	}
}
