package render

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestSimilarStripRendersScoredRows(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "game_similar", 100))
	// One aligned score-bar row per recommended game; ids intact.
	if !strings.Contains(out, "#") || !strings.Contains(out, "[") {
		t.Fatalf("scored rows missing:\n%s", out)
	}
	var barCols []int
	for _, line := range strings.Split(out, "\n") {
		if idx := strings.Index(line, "["); idx >= 0 && strings.Contains(line, "=") {
			barCols = append(barCols, idx)
		}
	}
	if len(barCols) < 2 {
		t.Fatalf("want multiple recommendation rows, got %d:\n%s", len(barCols), out)
	}
	for _, col := range barCols {
		if col != barCols[0] {
			t.Errorf("brackets misaligned across rows: %v", barCols)
		}
	}
	if strings.Contains(out, "<img") || strings.Contains(out, "◉") {
		t.Errorf("image residue leaked:\n%s", out)
	}
}

func TestGameChannelsRendersCoverageAndReco(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "game_channels", 100))
	// Coverage: braille share bars (the web Bar visualizer's ⣿ glyph,
	// dimmed for the empty remainder — never solid blocks) with percents;
	// identity from legend labels.
	for _, want := range []string{"@gmrdad82hard", "100.0%", "⣿"} {
		if !strings.Contains(out, want) {
			t.Errorf("coverage missing %q:\n%s", want, out)
		}
	}
	// Captions ride below their blocks.
	for _, want := range []string{"breakdown by channel", "sorted by fit score"} {
		if !strings.Contains(out, want) {
			t.Errorf("caption missing %q:\n%s", want, out)
		}
	}
	// Recommendations: score-bar rows keyed by the avatar's alt handle.
	if !strings.Contains(out, "[") || !strings.Contains(out, "|") {
		t.Errorf("recommendation score bars missing:\n%s", out)
	}
	// The web's CANVAS must never leak (its blank grid glyph U+2800);
	// the terminal draws its own ⣿ bars, no solid blocks.
	if strings.Contains(out, "⠀") || strings.Contains(out, "█") || strings.Contains(out, "░") {
		t.Errorf("canvas residue or solid blocks:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 100 {
			t.Errorf("line exceeds width (%d > 100): %q", w, line)
		}
	}
}

func TestPendingChannelsWaitsQuietly(t *testing.T) {
	withTrueColor(t)
	payload := `{"body":"<span class=\"pito-game-enhanced-message__intro\">Where this game is concentrated</span><div class=\"pito-metric\">⠀⠀⠀⣿⣿⠀</div>","html":true,
		"channel_distribution":{"status":"pending","intro":"Where this game is concentrated"}}`
	out := stripANSI(plain().Event(event("system", payload)))
	if !strings.Contains(out, strings.Repeat("⠂", 42)) {
		t.Errorf("pending note missing:\n%s", out)
	}
	if strings.Contains(out, "⣿") || strings.Contains(out, "⠀") {
		t.Errorf("pending canvas leaked:\n%s", out)
	}
}

func TestLinkedVideosStillRideTheListViewer(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "game_linked_videos", 110))
	// It's a Video::List payload: table frame + linked columns.
	for _, want := range []string{"Channel", "Duration", "Views", "Likes"} {
		if !strings.Contains(out, want) {
			t.Errorf("linked table missing column %q:\n%s", want, out)
		}
	}
	rules := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Count(line, "─") > 10 {
			rules++
		}
	}
	if rules != 3 {
		t.Errorf("want 3 table rules, got %d:\n%s", rules, out)
	}
}
