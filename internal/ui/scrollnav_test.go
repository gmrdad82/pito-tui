package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

func TestScrollNavTextInterpolatesCount(t *testing.T) {
	// Owner contract 2026-07-13: one fixed string per side, %{count} the
	// only token — the 50-variant %{direction}/{singular|plural} dance is
	// retired.
	for _, tc := range []struct {
		format string
		count  int
		want   string
	}{
		{"%{count} msgs before", 7, "7 msgs before"},
		{"%{count} msgs after", 1, "1 msgs after"},
		{"%{count} msgs after", 0, "0 msgs after"},
	} {
		if got := scrollNavText(tc.format, tc.count); got != tc.want {
			t.Errorf("scrollNavText(%q, %d) = %q, want %q", tc.format, tc.count, got, tc.want)
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
	if !strings.Contains(plain, "msgs after") {
		t.Fatalf("bottom pill must carry pito's fixed after-copy: %q", plain)
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

	// Mid-scroll shows both.
	m.sc.SetYOffset(m.sc.TotalLineCount() / 2)
	top, bottom = m.scrollNavPills()
	if top == "" || bottom == "" {
		t.Fatal("mid-scroll must show both pills")
	}
	if !strings.Contains(ansi.Strip(top), "ctrl+home") || !strings.Contains(ansi.Strip(top), "▲") {
		t.Fatalf("top pill anatomy wrong: %q", ansi.Strip(top))
	}
	if !strings.Contains(ansi.Strip(top), "msgs before") {
		t.Fatalf("top pill must carry pito's fixed before-copy: %q", ansi.Strip(top))
	}
}

// TestScrollNavPillsMatchContract locks the owner's 2026-07-13 contract:
// copy text, kbd token, glyph, in that order; the copy words render in
// the default (white) foreground — NOT pickerDimStyle — while the kbd
// token keeps render.KbdBare's own styling and the glyph stays dim.
func TestScrollNavPillsMatchContract(t *testing.T) {
	m, _ := newTestModel(t, nil, WithConversation("u-1"))
	m = sized(m)
	seedTurns(&m, 40)
	m.sc.SetYOffset(m.sc.TotalLineCount() / 2)

	above, below := m.transcript.TurnsOutside(m.contentWidth(), m.sc.YOffset(), m.sc.VisibleLineCount())
	if above == 0 || below == 0 {
		t.Fatal("mid-scroll fixture must have turns on both sides")
	}

	top, bottom := m.scrollNavPills()
	nav := render.PitoCopy.ScrollbackNav

	wantTop := scrollNavText(nav.Before, above) + render.KbdBare("ctrl+home", m.truecolor) + pickerDimStyle.Render(nav.JumpToStart)
	if top != wantTop {
		t.Fatalf("top pill = %q, want %q", top, wantTop)
	}
	wantBottom := scrollNavText(nav.After, below) + render.KbdBare("ctrl+end", m.truecolor) + pickerDimStyle.Render(nav.JumpToEnd)
	if bottom != wantBottom {
		t.Fatalf("bottom pill = %q, want %q", bottom, wantBottom)
	}

	// The copy words are unstyled (default/white foreground): they sit
	// verbatim, with no escape sequence, at the head of the pill.
	wantWords := scrollNavText(nav.Before, above)
	if !strings.HasPrefix(top, wantWords) {
		t.Fatalf("copy words must render unstyled at the pill's head: got %q, want prefix %q", top, wantWords)
	}
	if strings.Contains(top, pickerDimStyle.Render(wantWords)) {
		t.Fatalf("copy words must NOT carry the dim style: %q", top)
	}

	// The stripped line reads exactly the owner's contract shape.
	if got, want := ansi.Strip(top), wantWords+" ctrl+home ▲"; got != want {
		t.Fatalf("top pill stripped = %q, want %q", got, want)
	}
	if got, want := ansi.Strip(bottom), scrollNavText(nav.After, below)+" ctrl+end ▼"; got != want {
		t.Fatalf("bottom pill stripped = %q, want %q", got, want)
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
