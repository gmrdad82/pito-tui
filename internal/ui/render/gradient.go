package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RGB is one gradient stop.
type RGB struct{ R, G, B uint8 }

// Gradient interpolates across any number of stops — the terminal cousin
// of the web's multi-stop CSS gradients (score bar, TTB, context meter,
// shimmer sweeps all share it).
type Gradient struct {
	Stops []RGB
}

// PitoShimmer mirrors the web's subject-shimmer accent ramp.
var PitoShimmer = Gradient{Stops: []RGB{
	{0x87, 0x5f, 0xff}, // pito purple
	{0xff, 0x5f, 0xd7}, // accent pink
	{0x5f, 0xd7, 0xff}, // pito blue
	{0x87, 0x5f, 0xff},
}}

// MeterRamp is the web's green→red 5-stop context/score ramp.
var MeterRamp = Gradient{Stops: []RGB{
	{0x5f, 0xd7, 0x87}, {0xaf, 0xd7, 0x5f}, {0xd7, 0xd7, 0x5f},
	{0xd7, 0x87, 0x5f}, {0xd7, 0x5f, 0x5f},
}}

// At returns the interpolated color at t ∈ [0,1].
func (g Gradient) At(t float64) RGB {
	if len(g.Stops) == 0 {
		return RGB{0xff, 0xff, 0xff}
	}
	if len(g.Stops) == 1 || t <= 0 {
		return g.Stops[0]
	}
	if t >= 1 {
		return g.Stops[len(g.Stops)-1]
	}
	span := t * float64(len(g.Stops)-1)
	i := int(span)
	frac := span - float64(i)
	a, b := g.Stops[i], g.Stops[i+1]
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*frac) }
	return RGB{lerp(a.R, b.R), lerp(a.G, b.G), lerp(a.B, b.B)}
}

// Colorize paints text with the gradient, one rune at a time. phase
// shifts the ramp (the shimmer sweep: advance phase per animation tick);
// phase 0 is the resting state. Colors go through lipgloss — the same
// profile-managed path as every other style — so they survive block
// wrapping and downgrade on lesser terminals exactly like the texts do
// (raw SGR did not: the "white charts" bug).
func (g Gradient) Colorize(text string, phase float64) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return text
	}
	var b strings.Builder
	for i, r := range runes {
		t := float64(i)/float64(max(len(runes)-1, 1)) + phase
		t -= float64(int(t)) // wrap into [0,1)
		b.WriteString(lipgloss.NewStyle().Foreground(hex(g.At(t))).Render(string(r)))
	}
	return b.String()
}

func hex(c RGB) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B))
}

// Bar renders a value ∈ [0,1] as a gradient-filled cell bar — the score
// bar / TTB / context meter primitive. Filled cells sample the ramp left
// to right; empty cells render dim.
func (g Gradient) Bar(value float64, width int) string {
	if width < 1 {
		return ""
	}
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	filled := int(value*float64(width) + 0.5)
	var b strings.Builder
	for i := 0; i < filled; i++ {
		c := g.At(float64(i) / float64(max(width-1, 1)))
		b.WriteString(lipgloss.NewStyle().Foreground(hex(c)).Render("█"))
	}
	if filled < width {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorFaint).Render(strings.Repeat("░", width-filled)))
	}
	return b.String()
}

// Brand paints text in the pito brand gradient (static sweep position) —
// the status bar's glossy signature. Lesser terminals get the primary.
func Brand(text string, truecolor bool) string {
	if truecolor {
		return PitoShimmer.Colorize(text, 0)
	}
	return lipgloss.NewStyle().Foreground(ColorPrimary).Render(text)
}
