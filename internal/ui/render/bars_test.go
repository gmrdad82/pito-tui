package render

import (
	"strings"
	"testing"
)

// The score ramp must OPEN in its own starting color — no abrupt head
// zone (owner call): adjacent samples across the left half stay close.
func TestScoreRampLeftHalfIsContinuous(t *testing.T) {
	prev := ScoreRamp.At(0)
	for i := 1; i <= 50; i++ {
		c := ScoreRamp.At(float64(i) / 100)
		if delta(prev.R, c.R)+delta(prev.G, c.G)+delta(prev.B, c.B) > 12 {
			t.Errorf("abrupt ramp step at %d%%: %+v → %+v", i, prev, c)
		}
		prev = c
	}
	// And the far left is red family, not a detached dark zone.
	head := ScoreRamp.At(0)
	mid := ScoreRamp.At(0.4)
	if delta(head.R, mid.R) > 0x40 {
		t.Errorf("bar head too far from the ramp's red body: %+v vs %+v", head, mid)
	}
}

func delta(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// The TTB gradient is DYNAMIC, not linear: stop positions are the web's
// own projection of fixed hour thresholds (0/10/40/100h) onto each
// game's completionist axis. A short game clamps the hot thresholds to
// the right edge (greenish bar); a marathon compresses them to the left
// (reddish bar). parseHeatStops must preserve exactly that.
func TestTTBGradientProjectionIsDynamic(t *testing.T) {
	mix := func(expr, pct string) string { return expr + " " + pct + "%" }
	green := "color-mix(in oklch, var(--accent-green) 70%, var(--fg-default))"
	lime := "color-mix(in oklch, color-mix(in oklch, var(--accent-green), var(--accent-yellow)) 58%, var(--fg-default))"
	amber := "color-mix(in oklch, color-mix(in oklch, var(--accent-orange) 60%, var(--accent-yellow)) 58%, var(--fg-default))"
	pink := "color-mix(in oklch, var(--accent-red), var(--accent-purple))"
	style := func(stops ...string) string {
		return "background-image: var(--pito-bar-band, linear-gradient(transparent, transparent)), linear-gradient(to right in oklch, " + strings.Join(stops, ", ") + ");"
	}

	// 23h completionist: 10h→43.48%, 40h/100h clamp to 100%.
	short := StopGradient{Stops: parseHeatStops(style(
		mix(green, "0.0"), mix(lime, "43.48"), mix(amber, "100"), mix(pink, "100"),
	))}
	if c := short.At(0.7); !(c.G > c.R) {
		t.Errorf("short game must stay greenish at 70%%: %+v", c)
	}

	// 300h completionist: 10h→3.33%, 40h→13.33%, 100h→33.33%, pink to the edge.
	marathon := StopGradient{Stops: parseHeatStops(style(
		mix(green, "0.0"), mix(lime, "3.33"), mix(amber, "13.33"), mix(pink, "33.33"), mix(pink, "100"),
	))}
	if c := marathon.At(0.5); !(c.R > c.G) {
		t.Errorf("marathon must run reddish by 50%%: %+v", c)
	}
	if c := marathon.At(0.9); !(c.R > c.G) {
		t.Errorf("marathon must stay reddish at 90%%: %+v", c)
	}
	// And the same positions verify the real fixture (124h: pink from 80.65%).
	if c := short.At(0.2); !(c.G > c.R) {
		t.Errorf("short game opens green: %+v", c)
	}
}

// Neighboring elements must never pulse in sync: distinct seeds land
// distinct phase offsets, and the offset wraps cleanly.
func TestShimmerStagger(t *testing.T) {
	seeds := []string{"Rated ", "The hour toll ", "@gmrdad82", "@gmrdad82hard", "Views"}
	seen := map[int]bool{}
	for _, seed := range seeds {
		bucket := int(phaseOffset(seed) * 100)
		seen[bucket] = true
	}
	if len(seen) < len(seeds)-1 { // allow one collision, not a pile-up
		t.Errorf("stagger buckets collapsed: %d distinct of %d", len(seen), len(seeds))
	}
	if p := staggered(0.9, "Rated "); p < 0 || p >= 1 {
		t.Errorf("staggered phase must wrap into [0,1): %f", p)
	}
	if staggered(0.3, "a") == staggered(0.3, "b") {
		t.Error("different seeds must shift phase differently")
	}
}
