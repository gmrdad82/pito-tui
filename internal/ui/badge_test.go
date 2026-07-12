package ui

import (
	"net/http"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// withTrueColor used to scope a test to lipgloss v1's global TrueColor
// profile — v1's Style.Render() downsampled colors per that profile at
// render time. Lip Gloss v2 has no renderer/profile: Style.Render always
// emits full-fidelity ANSI, and the badge's gradient goes through lipgloss
// same as before, so it already emits truecolor unconditionally — nothing
// left to scope.
func withTrueColor(t *testing.T) {
	t.Helper()
}

func TestAiBadgeIsOneVisibleCell(t *testing.T) {
	withTrueColor(t)
	if w := lipgloss.Width(aiBadge(0, true)); w != 1 {
		t.Errorf("truecolor badge width = %d, want 1", w)
	}
	if w := lipgloss.Width(aiBadge(0.3, false)); w != 1 {
		t.Errorf("non-truecolor badge width = %d, want 1", w)
	}
}

func TestAiBadgeTruecolorRidesPhase(t *testing.T) {
	withTrueColor(t)
	if aiBadge(0, true) == aiBadge(0.5, true) {
		t.Error("truecolor badge must vary with phase — it rides the global shimmer sweep")
	}
}

func TestAiBadgeNonTruecolorIsStatic(t *testing.T) {
	withTrueColor(t) // profile shouldn't matter — the non-truecolor path never touches the gradient
	if aiBadge(0, false) != aiBadge(0.5, false) {
		t.Error("non-truecolor badge must stay static across phase")
	}
}

// resumeWithAiJSON mirrors resumeJSON (model_test.go) but flags one row as
// carrying an ai-kind event — the picker's sparkle badge condition.
const resumeWithAiJSON = `{
  "recent": [
    {"uuid": "u1", "title": "flagged", "display_name": "flagged", "last_activity_at": "2026-07-04T11:58:00Z", "ai": true},
    {"uuid": "u2", "title": "plain", "display_name": "plain", "last_activity_at": "2026-07-04T11:57:00Z"}
  ]
}`

func TestPickerBadgeMarksOnlyAiFlaggedRows(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resumeWithAiJSON))
	})

	m, _ := newTestModel(t, mux, WithTruecolor(true))
	m = sized(m)
	m = drive(m, m.fetchResumeCmd()())

	if len(m.rows) != 3 { // new + 2 recent
		t.Fatalf("rows = %d, want 3", len(m.rows))
	}
	if !m.rows[1].ai {
		t.Error("flagged row must decode ai = true")
	}
	if m.rows[2].ai {
		t.Error("unflagged row must decode ai = false")
	}

	view := m.viewContent()
	var flaggedLine, plainLine string
	for _, l := range strings.Split(view, "\n") {
		switch {
		case strings.Contains(l, "flagged"):
			flaggedLine = l
		case strings.Contains(l, "plain"):
			plainLine = l
		}
	}
	if !strings.Contains(flaggedLine, aiSparkle) {
		t.Errorf("flagged row missing badge: %q", flaggedLine)
	}
	if strings.Contains(plainLine, aiSparkle) {
		t.Errorf("unflagged row must not show the badge: %q", plainLine)
	}
}
