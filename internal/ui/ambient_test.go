package ui

import (
	"strings"
	"testing"
)

// ── Effect 1: margin star-field ──────────────────────────────────────────

// Same frame, same phase, twice in a row must produce byte-identical
// output — paintMarginStars is a pure function of (body, contentWidth,
// termWidth, phase); nothing about the star field may depend on
// anything else (wall clock, map iteration order, …).
func TestMarginStarfieldDeterministic(t *testing.T) {
	body := strings.Join([]string{
		strings.Repeat("x", 40),
		strings.Repeat("y", 30),
		"> typing…",
		"status line",
	}, "\n")
	a := paintMarginStars(body, 40, 100, 0.37)
	b := paintMarginStars(body, 40, 100, 0.37)
	if a != b {
		t.Errorf("same frame rendered twice must match:\ngot:  %q\nwant: %q", b, a)
	}
}

// A star's column always starts at max(contentWidth, the row's own real
// width) — so the original content bytes of every content row must
// survive as an exact PREFIX of the painted row, and the input/status
// lines (always the final two, per viewContent) must be untouched.
func TestMarginStarfieldNeverInsideContentColumn(t *testing.T) {
	lines := []string{
		strings.Repeat("a", 40), // exactly fills contentWidth
		strings.Repeat("b", 10), // well short of contentWidth
		"> typing…",
		"status line",
	}
	body := strings.Join(lines, "\n")
	painted := paintMarginStars(body, 40, 100, 0.5)
	out := strings.Split(painted, "\n")
	if len(out) != len(lines) {
		t.Fatalf("line count changed: got %d, want %d", len(out), len(lines))
	}
	for i, want := range lines[:2] {
		if !strings.HasPrefix(out[i], want) {
			t.Errorf("row %d: content column was altered — got %q, want prefix %q", i, out[i], want)
		}
	}
	if out[2] != lines[2] || out[3] != lines[3] {
		t.Errorf("input/status lines must never carry stars: got %q / %q", out[2], out[3])
	}
}

// Below the brief's own 12-column threshold, nothing gets painted at all.
func TestMarginStarfieldRequiresWideMargin(t *testing.T) {
	body := strings.Repeat("a", 40) + "\n> typing…\nstatus line"
	if got := paintMarginStars(body, 40, 45, 0.5); got != body {
		t.Errorf("a margin under %d cols must paint nothing:\ngot:  %q\nwant: %q", starMinMargin, got, body)
	}
}

// No star's brightness may ever exceed the ColorFaint-weight cap,
// regardless of the pulse value fed in (including out-of-range ones —
// lerpRGB clamps).
func TestStarBrightnessNeverExceedsFaintCap(t *testing.T) {
	for _, pulse := range []float64{-1, 0, 0.25, 0.5, 0.75, 1, 2} {
		c := lerpRGB(ambientStarBG, ambientStarFaint, pulse)
		if c.R > ambientStarFaint.R || c.G > ambientStarFaint.G || c.B > ambientStarFaint.B {
			t.Errorf("lerpRGB(pulse=%v) exceeded the faint cap: %+v > %+v", pulse, c, ambientStarFaint)
		}
	}
}

// ── Effect 2: shimmer conductor ──────────────────────────────────────────

// The conductor's weight must ramp 0→1→0 across its window and sit at 0
// everywhere else in the ~30s cycle.
func TestConductorWeightRampsOverWindow(t *testing.T) {
	windowStart := conductorCycleTicks - conductorWindowTicks

	if w := conductorWeight(0); w != 0 {
		t.Errorf("weight must be 0 well before the window: got %f", w)
	}
	if w := conductorWeight(windowStart - 1); w != 0 {
		t.Errorf("weight must still be 0 one tick before the window opens: got %f", w)
	}
	if w := conductorWeight(windowStart); w != 0 {
		t.Errorf("weight must open the window at 0 (ramp start): got %f", w)
	}
	mid := windowStart + conductorWindowTicks/2
	if w := conductorWeight(mid); w < 0.9 {
		t.Errorf("weight must peak near 1 at the window's midpoint: got %f", w)
	}
	last := windowStart + conductorWindowTicks - 1
	if w := conductorWeight(last); w <= 0 || w >= 0.3 {
		t.Errorf("weight must have descended back near 0 by the window's last tick: got %f", w)
	}
	// The cycle repeats identically one full period later.
	if w := conductorWeight(conductorCycleTicks + mid); w < 0.9 {
		t.Errorf("weight must ramp again on the next cycle: got %f", w)
	}
}

// ── Effect 3: cable-activity pulse ───────────────────────────────────────

// The dot law (owner 2026-07-12): a delivered cable message arms one
// green breath; the window counts down on the fast loop and holds the
// gate open until it's done; no idle machinery remains.
func TestCableActivityPulseArmsAndDecays(t *testing.T) {
	m := Model{truecolor: true, mode: modeChat}
	m.beginDotPulse()
	if m.dotPulseTicks != dotPulseTicksTotal {
		t.Fatalf("beginDotPulse must arm the window, got %d", m.dotPulseTicks)
	}
	if !m.animGateOpen() {
		t.Fatal("a live pulse must hold the animation gate open")
	}
	// Mid-window the dot renders greener than its resting floor and the
	// pulse dot differs from the static OK dot (it is mid-breath).
	m.dotPulseTicks = dotPulseTicksTotal / 2
	if m.activityPulseDot() == dotOK {
		t.Error("mid-pulse dot must differ from the static OK dot")
	}
	// Non-truecolor never arms (static dots stay legible).
	plain := Model{mode: modeChat}
	plain.beginDotPulse()
	if plain.dotPulseTicks != 0 {
		t.Error("non-truecolor must not pulse")
	}
}
