package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// withTrueColor scopes a test to the truecolor profile — gradients now
// render through lipgloss (the white-charts fix), so Ascii-profile tests
// see them stripped like every other style.
func withTrueColor(t *testing.T) {
	t.Helper()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
}

func TestGradientAtStops(t *testing.T) {
	g := Gradient{Stops: []RGB{{0, 0, 0}, {100, 200, 50}}}
	if c := g.At(0); c != (RGB{0, 0, 0}) {
		t.Errorf("At(0) = %+v", c)
	}
	if c := g.At(1); c != (RGB{100, 200, 50}) {
		t.Errorf("At(1) = %+v", c)
	}
	if c := g.At(0.5); c != (RGB{50, 100, 25}) {
		t.Errorf("At(0.5) = %+v", c)
	}
	// Multi-stop: the midpoint of a 3-stop ramp is the middle stop.
	m := Gradient{Stops: []RGB{{0, 0, 0}, {10, 10, 10}, {20, 20, 20}}}
	if c := m.At(0.5); c != (RGB{10, 10, 10}) {
		t.Errorf("3-stop At(0.5) = %+v", c)
	}
}

func TestColorizeEmitsTruecolorPerRune(t *testing.T) {
	withTrueColor(t)
	out := PitoShimmer.Colorize("23", 0)
	if strings.Count(out, "\x1b[38;2;") != 2 {
		t.Errorf("want one SGR per rune: %q", out)
	}
	if !strings.Contains(out, "\x1b[0m") {
		t.Errorf("styles must reset: %q", out)
	}
	// Phase shifts the ramp: different phase, different bytes.
	if PitoShimmer.Colorize("shimmer", 0) == PitoShimmer.Colorize("shimmer", 0.5) {
		t.Error("phase must move the sweep")
	}
}

func TestBarFill(t *testing.T) {
	withTrueColor(t)
	out := MeterRamp.Bar(0.5, 10)
	if strings.Count(out, "█") != 5 || strings.Count(out, "░") != 5 {
		t.Errorf("half bar wrong: %q", out)
	}
	if full := MeterRamp.Bar(1, 4); strings.Count(full, "█") != 4 || strings.Contains(full, "░") {
		t.Errorf("full bar wrong: %q", full)
	}
	if empty := MeterRamp.Bar(0, 4); strings.Count(empty, "░") != 4 {
		t.Errorf("empty bar wrong: %q", empty)
	}
}

func TestShimmerMarkersExtractAndPaint(t *testing.T) {
	body := `{"body":"The catalogue holds <span class=\"pito-subject-shimmer pito-shimmer-d16\">23</span> vids.","html":true}`

	// Plain/256-color renderers: static accent, markers never leak.
	out := plain().Event(event("system", body))
	if strings.ContainsRune(out, ShimmerStart) || strings.ContainsRune(out, ShimmerEnd) {
		t.Errorf("markers leaked: %q", out)
	}
	if !strings.Contains(out, "23") {
		t.Errorf("shimmer word lost: %q", out)
	}

	// Truecolor renderer + profile: the word is gradient-painted.
	withTrueColor(t)
	tc := New(60, WithStyle("dark"), WithTruecolor(true))
	out = tc.Event(event("system", body))
	if !strings.Contains(out, "\x1b[38;2;") {
		t.Errorf("truecolor shimmer missing: %q", out)
	}
}
