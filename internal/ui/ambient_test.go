package ui

import (
	"fmt"
	"hash/fnv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// ── The star machinery's perf rewrites (2026-07-21, the idle-CPU cut) ────

// starHash must produce EXACTLY the sums the retired fmt.Fprintf-into-
// fnv.New32a path produced, for both separators in use — star positions,
// glyphs, tints, sizes, and periods are all derived from these sums, so
// equivalence here IS the proof the sky looks identical.
func TestStarHashMatchesFnv(t *testing.T) {
	rows := []int{0, 1, 2, 3, 17, 44, 3691, 3695, 7382, 2*3691 + 40, -1}
	for _, row := range rows {
		for col := -2; col < 500; col++ {
			for _, sep := range []byte{':', '/'} {
				h := fnv.New32a()
				fmt.Fprintf(h, "%d%c%d", row, sep, col)
				if got, want := starHash(row, col, sep), h.Sum32(); got != want {
					t.Fatalf("starHash(%d, %d, %q) = %d, want %d (fnv)", row, col, sep, got, want)
				}
			}
		}
	}
}

// The incremental base slide must agree with a fresh full sweep at every
// base — including after a backward jump and a resize, which force the
// rebuild path.
func TestSkyRowStarsIncrementalMatchesSweep(t *testing.T) {
	skyRowCache = map[int]*skyRowEntry{}
	t.Cleanup(func() { skyRowCache = map[int]*skyRowEntry{} })
	const saltedRow, width = 7 + 3691, 120
	check := func(base, width int) {
		t.Helper()
		got := skyRowStars(saltedRow, base, width)
		want := sweepRowStars(saltedRow, base, width)
		if len(got) != len(want) {
			t.Fatalf("base %d width %d: %d stars, want %d", base, width, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("base %d width %d star %d: %+v, want %+v", base, width, i, got[i], want[i])
			}
		}
	}
	for base := 0; base < 300; base++ { // slides forward one column at a time
		check(base, width)
	}
	check(1200, width) // far jump: must rebuild, not slide
	check(1150, width) // backward: must rebuild
	check(1150, 90)    // resize: must rebuild
	for base := 1150; base < 1200; base++ {
		check(base, 90)
	}
}

// writeSGRGlyph's whole claim is "byte-identical to the lipgloss style
// it replaced" — pin it, across the component edge cases.
func TestWriteSGRGlyphMatchesLipgloss(t *testing.T) {
	colors := []render.RGB{
		{R: 0, G: 0, B: 0},
		{R: 255, G: 255, B: 255},
		{R: 0x16, G: 0x16, B: 0x1a},
		{R: 0x62, G: 0x62, B: 0x62},
		{R: 9, G: 100, B: 255},
	}
	for _, c := range colors {
		for _, glyph := range []rune{'⠁', '✦', '■'} {
			var b strings.Builder
			writeSGRGlyph(&b, c, glyph)
			want := lipgloss.NewStyle().Foreground(hexColor(c)).Render(string(glyph))
			if b.String() != want {
				t.Errorf("writeSGRGlyph(%+v, %q) = %q, want %q", c, glyph, b.String(), want)
			}
		}
	}
}

// paintStarSky stays a pure function of its inputs (scratch buffers and
// the row cache must never leak state between calls), and content rows
// are never touched.
func TestPaintStarSkyDeterministicAndContentSafe(t *testing.T) {
	skyRowCache = map[int]*skyRowEntry{}
	t.Cleanup(func() { skyRowCache = map[int]*skyRowEntry{} })
	content := "real content line"
	body := strings.Join([]string{content, "", "", "", ""}, "\n")
	a := paintStarSky(body, 5, 120, 3.21)
	b := paintStarSky(body, 5, 120, 3.21)
	if a != b {
		t.Errorf("same frame painted twice must match:\ngot:  %q\nwant: %q", b, a)
	}
	if got := strings.Split(a, "\n")[0]; got != content {
		t.Errorf("content rows must never be touched: %q", got)
	}
	// A different phase must still be deterministic after the cache
	// slid (the incremental path is what real frames hit).
	c1 := paintStarSky(body, 5, 120, 3.30)
	skyRowCache = map[int]*skyRowEntry{}
	c2 := paintStarSky(body, 5, 120, 3.30)
	if c1 != c2 {
		t.Errorf("slid cache and cold cache must paint identically:\ngot:  %q\nwant: %q", c1, c2)
	}
}

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
