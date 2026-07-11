package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// The glance panel: 2-row braille sparklines lifted verbatim from the
// web's own BrailleAreaChart output, each with its label+scalar legend.
func TestGlancePanelAnatomy(t *testing.T) {
	withTrueColor(t)
	for _, name := range []string{"glance_channel", "glance_vid", "glance_game"} {
		out := stripANSI(renderFixture(t, name, 100))
		// Metric legends with scalar meaning.
		for _, want := range []string{"Views", "Watched hours", "Average view duration", "Subs", "Likes"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: legend missing %q:\n%s", name, want, out)
			}
		}
		// Braille curves present; every sparkline is exactly 2 rows, so
		// braille lines come in consecutive pairs (two-up merges cells
		// side by side but keeps the 2-row rhythm).
		braille := 0
		for _, line := range strings.Split(out, "\n") {
			for _, ru := range line {
				if ru >= 0x2800 && ru <= 0x28FF {
					braille++
					break
				}
			}
		}
		if braille == 0 || braille%2 != 0 {
			t.Errorf("%s: want paired braille rows, got %d braille lines:\n%s", name, braille, out)
		}
		// The dotted-paper grid shows through blank stretches (drawn by
		// the terminal, not lifted from the payload's __bg-row spans):
		// ⠂ dots on the top row, ⣀ baseline on the bottom.
		if !strings.Contains(out, "⠂") || !strings.Contains(out, "⣀") {
			t.Errorf("%s: paper grid missing:\n%s", name, out)
		}
		for _, line := range strings.Split(out, "\n") {
			if w := lipgloss.Width(line); w > 100 {
				t.Errorf("%s: line exceeds width (%d): %q", name, w, line)
			}
		}
	}
}

func TestGlanceTwoUpAndStackedLayouts(t *testing.T) {
	withTrueColor(t)
	wide := stripANSI(renderFixture(t, "glance_channel", 100))
	// Two-up: Views and Watched hours share a line.
	sharedLine := false
	for _, line := range strings.Split(wide, "\n") {
		if strings.Contains(line, "Views") && strings.Contains(line, "Watched hours") {
			sharedLine = true
		}
	}
	if !sharedLine {
		t.Errorf("wide terminal should lay cells two-up:\n%s", wide)
	}
	narrow := stripANSI(renderFixture(t, "glance_channel", 60))
	for _, line := range strings.Split(narrow, "\n") {
		if strings.Contains(line, "Views") && strings.Contains(line, "Watched hours") {
			t.Errorf("narrow terminal must stack cells:\n%s", narrow)
		}
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("narrow line exceeds width (%d): %q", w, line)
		}
	}
}

func TestGlanceValueSpacing(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "glance_channel", 100))
	if strings.Contains(out, "81Likes") || strings.Contains(out, "11Dislikes") {
		t.Errorf("icon labels glued to counts:\n%s", out)
	}
	if !strings.Contains(out, "+11") || !strings.Contains(out, "-28") {
		t.Errorf("subs split value missing:\n%s", out)
	}
}

func TestGlancePendingWaitsQuietly(t *testing.T) {
	payload := `{"body":"<div class=\"x__intro\">All-time report</div><div class=\"pito-analytics-scalars\">⠐⠂ dots</div>","html":true,
		"analytics":{"status":"pending","intro":"All-time report"}}`
	out := stripANSI(plain().Event(event("system", payload)))
	if !strings.Contains(out, "crunching the numbers…") {
		t.Errorf("pending note missing:\n%s", out)
	}
	if strings.Contains(out, "dots") {
		t.Errorf("pending canvas leaked:\n%s", out)
	}
}

// The vid's linked-game card: same zebra kv anatomy as show cards, no
// stats column, full game_detail reply surface.
func TestLinkedGameCardRendersZebraKv(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "linked_game_vid", 100))
	// Unpriced game: em dash, no coin, no star.
	if strings.Contains(out, "●") || strings.Contains(out, "★") {
		t.Errorf("unpriced card must carry no coin glyphs:\n%s", out)
	}
	for _, want := range []string{"Title", "Astro Bot", "ID", "#27", "Genres", "Release", "September 2024"} {
		if !strings.Contains(out, want) {
			t.Errorf("linked-game card missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<img") || strings.Contains(out, "◉") {
		t.Errorf("image residue leaked:\n%s", out)
	}
}

func TestNoDataCellsWearTheServerCopy(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "glance_nodata", 100))
	if !strings.Contains(out, "No data yet.") {
		t.Errorf("no-data cells must carry the server's own copy:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 100 {
			t.Errorf("line exceeds width (%d): %q", w, line)
		}
	}
}
