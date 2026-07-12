package render

import (
	"math"
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
	r90 := &R{phase: 0.9}
	if p := r90.staggered("Rated "); p < 0 || p >= 1 {
		t.Errorf("staggered phase must wrap into [0,1): %f", p)
	}
	r30 := &R{phase: 0.3}
	if r30.staggered("a") == r30.staggered("b") {
		t.Error("different seeds must shift phase differently")
	}
}

// Shinies and chips scatter on the web's 20 discrete stagger buckets —
// offsets land exactly on the k/20 grid and distinct seeds spread out.
func TestTwentyStepStagger(t *testing.T) {
	seen := map[float64]bool{}
	r0 := &R{}
	for _, seed := range []string{"shiny-1 Sub", "shiny-2 Subs", "shiny-2K Subs", "chip-PS", "chip-Steam", "chip-Xbox"} {
		off := r0.staggered20(seed)
		if math.Abs(off*20-math.Round(off*20)) > 1e-9 {
			t.Errorf("offset %f for %q is off the 20-step grid", off, seed)
		}
		seen[off] = true
	}
	if len(seen) < 4 { // 6 seeds over 20 buckets: a pile-up means broken hashing
		t.Errorf("stagger buckets collapsed: %d distinct of 6", len(seen))
	}
}

// SetConductor(0) must reproduce today's scattered stagger exactly (the
// conductor's resting state is a no-op); SetConductor(1) must collapse
// every distinct seed onto the SAME phase — the synchronized "traveling
// wave" moment the shimmer conductor sweeps in once per ~30s of alive
// time (see internal/ui/ambient.go's conductorWeight).
func TestConductorBlendsStagger(t *testing.T) {
	seeds := []string{"Rated ", "The hour toll ", "@gmrdad82", "shiny-1 Sub", "chip-PS"}

	rest := &R{phase: 0.42}
	base := rest.staggered(seeds[0])
	for _, seed := range seeds[1:] {
		if rest.staggered(seed) == base {
			t.Fatalf("resting conductor (0) must keep seeds scattered: %q and %q collided", seeds[0], seed)
		}
	}

	synced := &R{phase: 0.42, conductor: 1}
	want := synced.staggered(seeds[0])
	for _, seed := range seeds {
		if got := synced.staggered(seed); got != want {
			t.Errorf("conductor=1 must synchronize every seed onto r.phase: %q got %f, want %f", seed, got, want)
		}
		if got := synced.staggered20(seed); got != want {
			t.Errorf("conductor=1 must synchronize staggered20 too: %q got %f, want %f", seed, got, want)
		}
	}

	// A mid-window weight must land strictly between the two extremes for
	// a seed whose natural offset isn't already 0 — proof it's a lerp, not
	// a snap.
	seed := "The hour toll "
	off := phaseOffset(seed)
	if off == 0 {
		t.Fatalf("test seed %q hashed to a zero offset; pick another", seed)
	}
	half := &R{phase: 0.42, conductor: 0.5}
	gotOffset := half.staggered(seed) - half.phase
	if gotOffset < 0 {
		gotOffset += 1
	}
	if math.Abs(gotOffset-off*0.5) > 1e-9 {
		t.Errorf("conductor=0.5 must halve the seed's natural offset: got %f, want %f", gotOffset, off*0.5)
	}
}

// The global 130° sweep: rows of a multi-line surface lean the band —
// the same cell lights at different phases on different rows.
func TestShimmerAngleLeansAcrossRows(t *testing.T) {
	base := RGB{0x80, 0x80, 0x80}
	if rowLean == 0 {
		t.Fatal("130° must produce a nonzero row lean")
	}
	phase := 0.5
	row0 := bandBoostRow(base, 20, 0, 42, phase)
	row1 := bandBoostRow(base, 20, 1, 42, phase)
	if row0 == row1 {
		// The band center differs by rowLean between rows; the same cell
		// must not be identically lit unless both are outside the band.
		row0in := bandBoostRow(base, 21, 0, 42, phase)
		row1in := bandBoostRow(base, 21, 1, 42, phase)
		if row0in == row1in {
			t.Errorf("angle lean has no effect across rows (lean=%f)", rowLean)
		}
	}
}
