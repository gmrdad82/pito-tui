package render

import (
	"hash/fnv"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The terminal bar engine — the 1:1 port of the web's ScoreBarComponent /
// TimeToBeatComponent bar language: a bracketed `=` run painted by a
// positioned-stop gradient, `|` ticks overlaid at percent positions, and
// inline value chips in invert video (the web's fg-block/bg-text chip).
// The score bar is a REUSABLE primitive (owner call): any surface with a
// 0-100 value and a label can render one.

// GradientStop is one positioned color stop (Pos ∈ [0,1]). Unlike
// Gradient's evenly-spaced stops, positions are explicit so hard edges
// (two stops sharing a Pos) and per-payload ramps (the TTB's adaptive
// heat projection) render exactly like their CSS `linear-gradient`.
type GradientStop struct {
	Color RGB
	Pos   float64
}

// StopGradient interpolates piecewise-linearly between positioned stops.
type StopGradient struct {
	Stops []GradientStop
}

// At returns the color at t ∈ [0,1]. On a hard edge (equal positions)
// the later stop wins from that position onward.
func (g StopGradient) At(t float64) RGB {
	if len(g.Stops) == 0 {
		return RGB{0xff, 0xff, 0xff}
	}
	if t <= g.Stops[0].Pos {
		return g.Stops[0].Color
	}
	for i := 1; i < len(g.Stops); i++ {
		a, b := g.Stops[i-1], g.Stops[i]
		if t > b.Pos {
			continue
		}
		span := b.Pos - a.Pos
		if span <= 0 {
			return b.Color
		}
		frac := (t - a.Pos) / span
		lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*frac) }
		return RGB{lerp(a.Color.R, b.Color.R), lerp(a.Color.G, b.Color.G), lerp(a.Color.B, b.Color.B)}
	}
	return g.Stops[len(g.Stops)-1].Color
}

// ShimmerAngleDeg is the GLOBAL sweep angle for every TUI shimmer
// (owner call 2026-07-06: 130°, CSS-style). On a cell grid the angle
// appears as a horizontal lean per row — terminal cells are ~2× taller
// than wide, so the lean is aspect-corrected.
const ShimmerAngleDeg = 130.0

// rowLean is the per-row horizontal band offset that produces the
// global angle across multi-row surfaces.
var rowLean = 2.0 / math.Tan(ShimmerAngleDeg*math.Pi/180)

// glossBoost is the shared surface glint — a GAUSSIAN highlight, the
// blur-soft cousin of the text shimmer's continuous gradient (owner
// call 2026-07-06: no hard-edged bands). There is no cutoff: every cell
// carries some light, falling off smoothly around the traveling peak;
// the row lean keeps the global 130° angle.
func glossBoost(c, tint RGB, i, row, cells int, phase, strength, sigma float64) RGB {
	span := float64(cells) + sigma*4 // run the peak past both edges
	center := phase*span - sigma*2 + float64(row)*rowLean
	d := (float64(i) - center) / sigma
	f := strength * math.Exp(-d*d)
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f) }
	return RGB{lerp(c.R, tint.R), lerp(c.G, tint.G), lerp(c.B, tint.B)}
}

// chartTint is the chart glint: pito-blue lifted toward white so the
// peak reads as light, not as a color stamp.
var chartTint = RGB{0xb7, 0xeb, 0xff}

func bandBoost(c RGB, i, cells int, phase float64) RGB {
	return bandBoostRow(c, i, 0, cells, phase)
}

// bandBoostRow sweeps the chart glint with the row's angle lean.
func bandBoostRow(c RGB, i, row, cells int, phase float64) RGB {
	return glossBoost(c, chartTint, i, row, cells, phase, 0.5, 5.5)
}

// phaseOffset scatters animations by a stable per-element seed so
// neighbors never pulse in sync — the terminal cousin of the web's
// Pito::Shimmer.offset_class stagger buckets.
func phaseOffset(seed string) float64 {
	h := fnv.New32a()
	h.Write([]byte(seed))
	return float64(h.Sum32()%997) / 997
}

// staggered wraps a phase with a seed offset back into [0,1).
func staggered(phase float64, seed string) float64 {
	p := phase + phaseOffset(seed)
	return p - math.Floor(p)
}

// staggered20 quantizes the seed offset onto the web's 20 stagger
// buckets (pito-shimmer-d0…d19) — shinies and platform chips scatter in
// discrete steps, never in sync, never in-between (owner call).
func staggered20(phase float64, seed string) float64 {
	step := math.Floor(phaseOffset(seed)*20) / 20
	p := phase + step
	return p - math.Floor(p)
}

// The shared accent palette for bars — the terminal cousins of the web's
// theme-mixed accents (application.css HEAT_THRESHOLDS + score ramp).
var (
	heatGreen  = RGB{0x5f, 0xd7, 0x87}
	heatLime   = RGB{0xaf, 0xd7, 0x5f}
	heatAmber  = RGB{0xd7, 0xaf, 0x5f}
	heatYellow = RGB{0xd7, 0xd7, 0x5f}
	heatPink   = RGB{0xd7, 0x5f, 0xd7}

	scoreDarkRed   = RGB{0xb2, 0x4c, 0x4c} // near scoreRed — the bar opens IN the ramp, no abrupt head
	scoreRed       = RGB{0xd7, 0x5f, 0x5f}
	scoreRedOrange = RGB{0xd7, 0x73, 0x5f}
	scoreOrange    = RGB{0xd7, 0x87, 0x5f}
)

// ScoreRamp mirrors .pito-score-bar__fill's arc — deep red into green —
// but the left half blends CONTINUOUSLY from a slightly deeper red into
// red (owner call: the web's theme-mixed zones read subtle; saturated
// terminal colors made the old hard edges abrupt). The bar must open in
// the gradient's own starting color, not jump away from it.
var ScoreRamp = StopGradient{Stops: []GradientStop{
	{scoreDarkRed, 0}, {scoreRed, 0.50},
	{scoreRedOrange, 0.55}, {scoreOrange, 0.65},
	{heatYellow, 0.75}, {heatLime, 0.85},
	{heatGreen, 0.90}, {heatGreen, 1},
}}

// BarTick is a `|` overlaid on a bar at Pct (0-100), optionally carrying
// an inline value chip beside it (the score number, the footage hours).
type BarTick struct {
	Pct      float64
	Color    RGB
	Bold     bool
	Chip     string // "" = bare tick
	ChipLeft bool   // chip sits left of the tick (web: score >= 50)
}

// barCells is the [bracket-to-bracket] cell count for a given content
// width and label. Minimum 10 so degenerate widths stay bars.
func barCells(width int, label string) int {
	cells := width - lipgloss.Width(label) - 2
	if cells < 10 {
		cells = 10
	}
	return cells
}

// tickIndex maps a percent position onto a cell index — the terminal
// equivalent of the web's absolute left:% within the track.
func tickIndex(pct float64, cells int) int {
	i := int(pct/100*float64(cells-1) + 0.5)
	if i < 0 {
		i = 0
	}
	if i >= cells {
		i = cells - 1
	}
	return i
}

// barLine composes one bar: label + [ gradient `=` fill + ticks + chips ].
// muted renders the web's no-data state: dim fill, no gradient, no ticks.
func (r *R) barLine(label string, fill StopGradient, ticks []BarTick, width int, muted bool) string {
	cells := barCells(width, label)

	type cell struct {
		ch    string
		tick  *BarTick
		chip  bool
		color RGB
	}
	row := make([]cell, cells)
	for i := range row {
		row[i] = cell{ch: "=", color: fill.At(float64(i) / float64(max(cells-1, 1)))}
	}
	if !muted {
		for t := range ticks {
			tick := &ticks[t]
			i := tickIndex(tick.Pct, cells)
			row[i] = cell{ch: "|", tick: tick}
			if tick.Chip != "" {
				chip := []rune(tick.Chip)
				start := i + 1
				if tick.ChipLeft {
					start = i - len(chip)
				}
				for j, cr := range chip {
					if p := start + j; p >= 0 && p < cells && row[p].tick == nil {
						row[p] = cell{ch: string(cr), chip: true}
					}
				}
			}
		}
	}

	// The grow-in (web pito-bar-reveal): cells beyond the reveal cut
	// render as the muted fill; ticks and chips surface once reached.
	cut := cells
	if r.revealFrac < 1 {
		cut = int(r.revealFrac*float64(cells) + 0.5)
	}
	var b strings.Builder
	b.WriteString(r.dim(label))
	b.WriteString(r.dim("["))
	chipStyle := lipgloss.NewStyle().Reverse(true)
	for i, c := range row {
		switch {
		case i >= cut:
			b.WriteString(lipgloss.NewStyle().Foreground(ColorFaint).Render("="))
		case c.chip:
			b.WriteString(chipStyle.Render(c.ch))
		case c.tick != nil:
			st := lipgloss.NewStyle().Bold(c.tick.Bold)
			if r.truecolor {
				st = st.Foreground(hex(c.tick.Color))
			}
			b.WriteString(st.Render(c.ch))
		case muted || !r.truecolor:
			b.WriteString(lipgloss.NewStyle().Foreground(ColorFaint).Render(c.ch))
		default:
			b.WriteString(lipgloss.NewStyle().Foreground(hex(bandBoost(c.color, i, cells, staggered(r.phase, label)))).Render(c.ch))
		}
	}
	b.WriteString(r.dim("]"))
	return b.String()
}

// ScoreBar is the reusable 0-100 rating bar — label, score ramp, one tick
// at the score with the value chip beside it (left when >= 50, mirroring
// the web's value_side_class). score < 0 renders the muted no-data bar.
func (r *R) ScoreBar(label string, score int, width int) string {
	if score < 0 {
		return r.barLine(label, StopGradient{}, nil, width, true)
	}
	pct := float64(score)
	if pct < 1 {
		pct = 1
	}
	if pct > 99 {
		pct = 99
	}
	tick := BarTick{
		Pct:      pct,
		Color:    ScoreRamp.At(float64(score) / 100),
		Bold:     true,
		Chip:     itoa(score),
		ChipLeft: score >= 50,
	}
	return r.barLine(label, ScoreRamp, []BarTick{tick}, width, false)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

// positionRow lays text fragments onto a blank line at percent positions
// under a bar — the TTB's hour-values row. atEnd anchors the fragment's
// right edge at the position (the web's ttb-label--at-end).
type positionedText struct {
	Text  string
	Pct   float64
	AtEnd bool
}

func (r *R) positionRow(label string, frags []positionedText, width int, style lipgloss.Style) string {
	cells := barCells(width, label)
	row := make([]rune, cells)
	for i := range row {
		row[i] = ' '
	}
	for _, f := range frags {
		text := []rune(f.Text)
		anchor := tickIndex(f.Pct, cells)
		start := anchor - len(text)/2
		if f.AtEnd {
			start = anchor - len(text) + 1
		}
		if start < 0 {
			start = 0
		}
		if start+len(text) > cells {
			start = cells - len(text)
		}
		for j, cr := range text {
			if p := start + j; p >= 0 && p < cells {
				row[p] = cr
			}
		}
	}
	indent := strings.Repeat(" ", lipgloss.Width(label)+1)
	return indent + style.Render(strings.TrimRight(string(row), " "))
}

// ContextMeter draws the web's context meter as a thin braille bar: the
// filled span reveals the green→red MeterRamp left to right (the fill
// is the reveal, like the web's background-clip), the remainder stays
// faint. Server-computed pct — this only draws.
func (r *R) ContextMeter(pct float64, width int) string {
	if width < 1 {
		return ""
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct/100*float64(width) + 0.5)
	var b strings.Builder
	for i := 0; i < width; i++ {
		if i >= filled {
			b.WriteString(lipgloss.NewStyle().Foreground(ColorFaint).Render("⣀"))
			continue
		}
		st := lipgloss.NewStyle()
		if r.truecolor {
			c := MeterRamp.At(float64(i) / float64(max(width-1, 1)))
			st = st.Foreground(hex(bandBoost(c, i, width, staggered(r.phase, "context-meter"))))
		} else {
			st = st.Foreground(ColorOK)
		}
		b.WriteString(st.Render("⣀"))
	}
	return b.String()
}

// phasePulse is a per-seed sine in [-1,1] — the breathing primitive.
func phasePulse(phase float64, seed string) float64 {
	return math.Sin(staggered(phase, seed) * 2 * math.Pi)
}
