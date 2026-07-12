package render

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// RGB is one gradient stop.
type RGB struct{ R, G, B uint8 }

// Gradient interpolates across any number of stops — the terminal cousin
// of the web's multi-stop CSS gradients (score bar, TTB, context meter,
// shimmer sweeps all share it).
type Gradient struct {
	Stops []RGB
}

// PitoShimmer is the BRAND ramp — pito-blue into purple and back. Since
// the 2026-07-12 color spec v2 it paints only brand surfaces (the
// status identity, Kbd chips, aiPending's gradient kin); subjects and
// references ride their own derived ramps below.
var PitoShimmer = Gradient{Stops: []RGB{
	{0x51, 0x70, 0xff}, // brand pito
	{0xbb, 0x9a, 0xf7}, // accent purple
	{0x51, 0x70, 0xff}, // back to brand
}}

// ── color spec v2 (owner 2026-07-12) ────────────────────────────────────
// "Use Cyan for reference with some good pair shimmering color
// (mathematically derived from the Cyan)… Same for the Subject: base
// Pink and its shimmering pair derived from it. Your colors, same
// principle, ignoring themes." The pairs are DERIVED, not picked: the
// traveling band is the base rotated +24° around the hue wheel and
// lifted +14% lightness — near-analogous, so the sweep reads as the
// same color catching light, on any background.

// deriveBandPair computes a base color's shimmer band.
func deriveBandPair(base RGB) RGB {
	h, sat, l := rgbToHSL(base)
	return hslToRGB(h+24.0/360.0, sat, min(l+0.14, 0.92))
}

// shimmerFor builds the base→band→base ramp for a base color.
func shimmerFor(base RGB) Gradient {
	band := deriveBandPair(base)
	return Gradient{Stops: []RGB{base, band, base}}
}

var (
	// subjectBase: a clean pink, chosen for legibility on dark ground.
	subjectBase = RGB{0xe8, 0x7d, 0xba}
	// referenceBase: the palette cyan the owner named.
	referenceBase = RGB{0x7d, 0xcf, 0xff}

	// SubjectShimmer / ReferenceShimmer: base + derived band (above).
	SubjectShimmer   = shimmerFor(subjectBase)
	ReferenceShimmer = shimmerFor(referenceBase)
)

// rgbToHSL / hslToRGB: the standard conversions, enough precision for
// deriving band pairs at init (not a hot path).
func rgbToHSL(c RGB) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	maxV := math.Max(r, math.Max(g, b))
	minV := math.Min(r, math.Min(g, b))
	l = (maxV + minV) / 2
	if maxV == minV {
		return 0, 0, l
	}
	d := maxV - minV
	if l > 0.5 {
		s = d / (2 - maxV - minV)
	} else {
		s = d / (maxV + minV)
	}
	switch maxV {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

func hslToRGB(h, s, l float64) RGB {
	h = math.Mod(h, 1)
	if h < 0 {
		h += 1
	}
	if s == 0 {
		v := uint8(l * 255)
		return RGB{v, v, v}
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	hue := func(t float64) float64 {
		if t < 0 {
			t++
		}
		if t > 1 {
			t--
		}
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 1.0/2:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	return RGB{
		uint8(hue(h+1.0/3)*255 + 0.5),
		uint8(hue(h)*255 + 0.5),
		uint8(hue(h-1.0/3)*255 + 0.5),
	}
}

// AIAccent is the web's data-accent="ai" pair — accent purple into
// brand pito-blue. Vertical on segment bars, static on the prompt.
var AIAccent = Gradient{Stops: []RGB{
	{0xbb, 0x9a, 0xf7}, // accent purple
	{0x51, 0x70, 0xff}, // brand pito
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

func hex(c RGB) color.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B))
}

// scaleRGB scales c's brightness by factor (1 = unchanged), clamping each
// channel back into [0,255] — the shiny badge halo's breathing edge color
// rides this (factor ∈ [0.75, 1.25], ±25% around the material's edge).
func scaleRGB(c RGB, factor float64) RGB {
	clamp := func(v float64) uint8 {
		switch {
		case v <= 0:
			return 0
		case v >= 255:
			return 255
		default:
			return uint8(v + 0.5)
		}
	}
	return RGB{clamp(float64(c.R) * factor), clamp(float64(c.G) * factor), clamp(float64(c.B) * factor)}
}

// lerpRGB blends a toward b by f ∈ [0,1] (0 = a, 1 = b) — the shared
// per-channel interpolation every gradient/gloss helper in this package
// already inlines its own copy of; shiny badges use this one directly
// for the ink glint and the dim date suffix.
func lerpRGB(a, b RGB, f float64) RGB {
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f) }
	return RGB{lerp(a.R, b.R), lerp(a.G, b.G), lerp(a.B, b.B)}
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
