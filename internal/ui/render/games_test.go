package render

import (
	"strings"
	"testing"
)

// channel_games renders from the structured rows (tui-needs.md item 5):
// the shared table look, intro kept, cover-grid junk gone.
func TestChannelGamesRendersFromStructuredRows(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "channel_games", 100))
	for _, want := range []string{"#27", "Astro Bot", "1", "Game", "Vids"} {
		if !strings.Contains(out, want) {
			t.Errorf("games table missing %q:\n%s", want, out)
		}
	}
	rules := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Count(line, "─") > 10 {
			rules++
		}
	}
	if rules != 3 {
		t.Errorf("want the standard 3-rule table frame, got %d:\n%s", rules, out)
	}
	// The web cover grid must not leak ("1 vid" floats in it).
	if strings.Contains(out, "1 vid\n") || strings.Contains(out, "vid ") && strings.Contains(out, "cover") {
		t.Errorf("cover grid junk leaked:\n%s", out)
	}
}
