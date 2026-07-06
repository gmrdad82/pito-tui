package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestShiniesRailsAndBadges(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "shinies_reply_v2", 110))
	// One lane per metric with rail + legend.
	for _, want := range []string{"Subs", "at 2.2K · next: 5K (Ruby)", "●", "◉", "·"} {
		if !strings.Contains(out, want) {
			t.Errorf("shinies lane missing %q:\n%s", want, out)
		}
	}
	// Badges flow under the rail, faces intact (dates are web-only).
	for _, want := range []string{"1 Sub", "2 Subs"} {
		if !strings.Contains(out, want) {
			t.Errorf("badge face missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Jun '26") {
		t.Errorf("badge dates are web-only:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 110 {
			t.Errorf("line exceeds width (%d): %q", w, line)
		}
	}
}

func TestShinyBadgeComponent(t *testing.T) {
	withTrueColor(t)
	r := New(80, WithTruecolor(true))
	badge := stripANSI(r.ShinyBadge("gold", "100K Subs"))
	if !strings.Contains(badge, "100K Subs") {
		t.Errorf("badge face missing: %q", badge)
	}
	long := stripANSI(r.ShinyBadge("jade", "200 Viewsandmore"))
	if !strings.Contains(long, "…") {
		t.Errorf("compact form must trim with ellipsis: %q", long)
	}
	if unknown := stripANSI(r.ShinyBadge("vibranium", "5 Things")); !strings.Contains(unknown, "5 Things") {
		t.Errorf("unknown material must degrade to the neutral pill: %q", unknown)
	}
}

func TestStatsAndShiniesSitInTheirOwnTable(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "show_channel_v2", 100))
	lines := strings.Split(out, "\n")
	statsIdx, handleIdx, blankBetween := -1, -1, false
	for i, line := range lines {
		if strings.Contains(line, "Stats") && statsIdx < 0 {
			statsIdx = i
		}
		if strings.Contains(line, "Handle") && handleIdx < 0 {
			handleIdx = i
		}
	}
	if statsIdx < 0 || handleIdx < 0 || statsIdx > handleIdx {
		t.Fatalf("Stats block must precede the details table (stats=%d handle=%d):\n%s", statsIdx, handleIdx, out)
	}
	for _, line := range lines[statsIdx:handleIdx] {
		if strings.TrimSpace(stripANSI(line)) == "┃" || strings.TrimSpace(stripANSI(line)) == "" {
			blankBetween = true
		}
	}
	if !blankBetween {
		t.Errorf("a blank row must separate the two tables:\n%s", out)
	}
}
