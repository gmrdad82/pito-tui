package ui

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// The ambient layer: three whisper-intensity effects that make the
// screen feel alive without ever reading as noise (owner brief,
// 2026-07-12). Each is independently kill-switchable below — flip one
// to false and that effect vanishes; nothing else needs to change.
//
//   - marginStarfieldEnabled: sparse braille dots twinkling in the
//     terminal's empty right margin, painted as a post-processing pass
//     over the assembled frame — see paintMarginStars, called from
//     Model.viewContent.
//   - shimmerConductorEnabled: every ~30s of ALIVE time (animation
//     ticks, never wall-clock idle), a ~2s window where every shimmer
//     painter's per-element stagger collapses onto one shared traveling
//     phase — see conductorWeight, pushed from Model.onAnimTick into
//     render.R via SetConductor.
//   - cableActivityPulseEnabled: the status bar's presence dot takes
//     one green breath whenever the cable delivers a message (owner
//     2026-07-12: no idle breathing; activity only) — see beginDotPulse
//     and Model.activityPulseDot.
//
// All three are additionally gated on m.truecolor at their call sites —
// non-truecolor terminals, and every golden test (none of which set
// WithTruecolor), get none of this.
const (
	marginStarfieldEnabled    = true
	starSkyEnabled            = true // the moving sky on blank rows (owner eye-candy mandate)
	shimmerConductorEnabled   = true
	cableActivityPulseEnabled = true
)

// skyTickInterval: the sky's own heartbeat — 16ms ≈ 60fps (owner
// 2026-07-12: the first cut's 120ms/8fps read as lag on a 280Hz OLED;
// "can we use 60fps?" — yes). Bubble Tea v2's cell-diff renderer only
// writes the star cells that changed, so the full-rate loop stays
// cheap; the fast 33ms gate is untouched (transcript repaints are the
// expensive path, the sky is not).
const skyTickInterval = 16 * time.Millisecond

func skyTick() tea.Cmd {
	return tea.Tick(skyTickInterval, func(time.Time) tea.Msg { return SkyTickMsg{} })
}

// startSkyIfIdle keeps the sky's loop alive whenever the app is on the
// chat surface with truecolor — a self-rescheduling slow tick, cheap by
// construction (blank rows only; the dirty-gated fast path never runs).
func (m *Model) startSky() tea.Cmd {
	if !starSkyEnabled || !m.skyOn || !m.truecolor || m.skyTicking {
		return nil
	}
	m.skyTicking = true
	return skyTick()
}

func (m Model) onSkyTick() (tea.Model, tea.Cmd) {
	if !starSkyEnabled || !m.skyOn || !m.truecolor {
		m.skyTicking = false
		return m, nil
	}
	// Same drift speed the 120ms loop had (0.35/tick), spread across
	// 7.5× the frames — smoothness, not acceleration.
	m.skyPhase += 0.047
	return m, skyTick()
}

// ── Effect 1: margin star-field ──────────────────────────────────────────

// starMinMargin is the brief's own threshold: the terminal must be at
// least this many columns wider than the content column before any
// star gets painted — narrower than that and "the margin" doesn't read
// as a margin at all, just a ragged edge.
const starMinMargin = 12

// starDensity: roughly one star candidate per this many margin cells —
// sparse dust, not a wall.
const starDensity = 80

// starGlyphs are the eight single-dot braille cells (one dot lit, seven
// blank). Which glyph a given star draws is itself hashed from its
// position, so neighbors don't all draw the identical dot.
var starGlyphs = [8]rune{'⠁', '⠂', '⠄', '⡀', '⢀', '⠈', '⠐', '⠠'}

// ambientStarBG/ambientStarFaint bound every star's brightness: the
// twinkle's low point (near-background, almost invisible) and its high
// point — capped at roughly ColorFaint's own weight (owner rule: no
// star ever brighter than ColorFaint), never anything louder.
var (
	ambientStarBG    = render.RGB{R: 0x16, G: 0x16, B: 0x1a}
	ambientStarFaint = render.RGB{R: 0x62, G: 0x62, B: 0x62} // ≈ ColorFaint (256-color 241)
)

// skyStar is one cached star of a row at a given drift base.
type skyStar struct {
	col    int
	glyph  rune
	offset float64
}

// skyRowStarCache memoizes star positions per (salted row, drift base):
// at 60fps the drift base only shifts every ~7 frames, so the fnv sweep
// across every column (the sky's only real cost) runs on base changes
// instead of every frame. The UI is single-goroutine — no lock. Bounded:
// wholesale reset past 8k entries (a few minutes of drift).
var skyRowStarCache = map[[3]int][]skyStar{}

func skyRowStars(saltedRow, base, termWidth int) []skyStar {
	key := [3]int{saltedRow, base, termWidth}
	if stars, ok := skyRowStarCache[key]; ok {
		return stars
	}
	if len(skyRowStarCache) > 8192 {
		skyRowStarCache = map[[3]int][]skyStar{}
	}
	stars := []skyStar{}
	for col := 2; col < termWidth-1; col++ {
		if present, glyph, offset := starAt(saltedRow, col+base); present {
			stars = append(stars, skyStar{col: col, glyph: glyph, offset: offset})
		}
	}
	skyRowStarCache[key] = stars
	return stars
}

// starAt deterministically decides whether (row, col) carries a star:
// same coordinates always produce the same answer, and if present, the
// same glyph and the same phase offset — the star field's positions
// never reshuffle between frames. Only a present star's brightness ever
// moves, and only when phase itself moves.
func starAt(row, col int) (present bool, glyph rune, offset float64) {
	h := fnv.New32a()
	fmt.Fprintf(h, "%d:%d", row, col)
	sum := h.Sum32()
	if sum%starDensity != 0 {
		return false, 0, 0
	}
	glyph = starGlyphs[(sum>>8)%8]
	offset = float64((sum>>16)%997) / 997
	return true, glyph, offset
}

// paintStarSky is the star field's 2.0.0 upgrade (owner eye-candy
// mandate, 2026-07-12: "on spare space use some starfield / star sky
// stars moving"): the spare space is no longer a capped margin — it is
// every EMPTY row of the frame's content region. Two parallax layers of
// the same deterministic stars drift horizontally at different speeds
// (the whole sampling grid slides with skyPhase, so positions stay
// stable relative to the field while the field itself glides), each
// star still twinkling on its own offset. Blank rows only — a row with
// ANY real content is never touched, so the sky lives strictly in the
// void below short conversations and behind the splash.
func paintStarSky(body string, contentRows, termWidth int, skyPhase float64) string {
	if termWidth < 20 {
		return body
	}
	lines := strings.Split(body, "\n")
	if contentRows > len(lines) {
		contentRows = len(lines)
	}
	// Layer drifts in CONTINUOUS cells: slow background, faster
	// foreground. The fractional part drives sub-cell motion blending —
	// a star fades out of its cell and into the next as it glides, so
	// 60fps reads as sliding, not stepping (the first cut's whole-cell
	// jumps were half the perceived lag).
	type layerDrift struct {
		base int
		frac float64
	}
	var layers [2]layerDrift
	for i, speed := range [2]float64{3, 8} {
		d := skyPhase * speed
		base := math.Floor(d)
		layers[i] = layerDrift{base: int(base), frac: d - base}
	}
	for row := 0; row < contentRows; row++ {
		if strings.TrimSpace(lines[row]) != "" {
			continue
		}
		// intensity per column: blended contributions land here first,
		// then paint in one left-to-right pass.
		cells := map[int]float64{}
		glyphs := map[int]rune{}
		for layer, ld := range layers {
			for _, st := range skyRowStars(row+layer*3691, ld.base, termWidth) {
				col, glyph, offset := st.col, st.glyph, st.offset
				{
					pulse := (math.Sin((skyPhase*0.13+offset)*2*math.Pi) + 1) / 2
					// The star sits between col and col-1 by frac: split its
					// light across both cells (linear crossfade).
					if cells[col] < pulse*(1-ld.frac) {
						cells[col] = pulse * (1 - ld.frac)
						glyphs[col] = glyph
					}
					if col-1 >= 2 && cells[col-1] < pulse*ld.frac {
						cells[col-1] = pulse * ld.frac
						glyphs[col-1] = glyph
					}
				}
			}
		}
		if len(cells) == 0 {
			continue
		}
		cols := make([]int, 0, len(cells))
		for col := range cells {
			cols = append(cols, col)
		}
		sort.Ints(cols)
		var b strings.Builder
		cursor := 0
		for _, col := range cols {
			for ; cursor < col; cursor++ {
				b.WriteByte(' ')
			}
			c := lerpRGB(ambientStarBG, ambientStarFaint, cells[col])
			b.WriteString(lipgloss.NewStyle().Foreground(hexColor(c)).Render(string(glyphs[col])))
			cursor++
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}

// paintMarginStars overlays the twinkling star field on an assembled
// frame's empty right margin — a post-processing pass (pad-to-margin +
// overlay), never a change to how any section rendered its own content.
// Two hard boundaries, both enforced by construction rather than
// trusted from the caller:
//
//   - never inside the content column: a row's star columns always
//     start at max(contentWidth, that row's own real rendered width),
//     so even a line that overflows contentWidth can't get a star
//     stamped over real content.
//   - never on the input/status lines: viewContent always appends
//     exactly those two as the frame's FINAL two entries, so the last
//     two lines of the split are unconditionally skipped.
//
// phase rides whatever the model's existing shimmer phase is — stars
// have no tick of their own (the fast 40ms gate must still close when
// only ambience remains); they simply draw with whatever phase was
// last pushed by a real animation tick.
func paintMarginStars(body string, contentWidth, termWidth int, phase float64) string {
	if termWidth-contentWidth < starMinMargin {
		return body
	}
	lines := strings.Split(body, "\n")
	contentRows := len(lines) - 2 // input + status, always last (see doc comment above)
	if contentRows < 1 {
		return body
	}
	type star struct {
		col    int
		glyph  rune
		offset float64
	}
	for row := 0; row < contentRows; row++ {
		line := lines[row]
		w := lipgloss.Width(line)
		start := contentWidth
		if w > start {
			start = w
		}
		if start >= termWidth {
			continue
		}
		var stars []star
		for col := start; col < termWidth; col++ {
			if present, glyph, offset := starAt(row, col); present {
				stars = append(stars, star{col, glyph, offset})
			}
		}
		if len(stars) == 0 {
			continue
		}
		var b strings.Builder
		b.WriteString(line)
		cursor := w
		for _, s := range stars {
			for ; cursor < s.col; cursor++ {
				b.WriteByte(' ')
			}
			pulse := (math.Sin((phase+s.offset)*2*math.Pi) + 1) / 2 // 0..1..0
			c := lerpRGB(ambientStarBG, ambientStarFaint, pulse)
			b.WriteString(lipgloss.NewStyle().Foreground(hexColor(c)).Render(string(s.glyph)))
			cursor++
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}

// ── Effect 2: shimmer conductor ──────────────────────────────────────────

// conductorCycleTicks/conductorWindowTicks translate the brief's "every
// ~30s of ALIVE time, a 2s sweep window" into shimmerTick (40ms) tick
// counts — both exact divisions (750 and 50 ticks), no rounding.
const (
	conductorCycleTicks  = int64(30 * time.Second / shimmerTick)
	conductorWindowTicks = int64(2 * time.Second / shimmerTick)
)

// conductorWeight is aliveTicks' pure projection onto the shimmer
// conductor's blend weight: 0 for the first ~28s of every ~30s cycle,
// then a half-sine ramp 0→1→0 across the final ~2s window — the moment
// every shimmer painter's stagger (render.R's staggered/staggered20)
// collapses onto the SAME r.phase and the screen reads as one
// traveling wave, before scattering back to normal. Always 0 when
// shimmerConductorEnabled is false — the kill switch.
func conductorWeight(aliveTicks int64) float64 {
	if !shimmerConductorEnabled {
		return 0
	}
	pos := aliveTicks % conductorCycleTicks
	windowStart := conductorCycleTicks - conductorWindowTicks
	if pos < windowStart {
		return 0
	}
	progress := float64(pos-windowStart) / float64(conductorWindowTicks)
	return math.Sin(progress * math.Pi)
}

// ── Effect 3: cable-activity pulse ───────────────────────────────────────
// Owner re-spec 2026-07-12: "have the idle have no breathing. breathing
// should be green only when activity appears on cable." The old idle
// breathing (its own 500ms loop, gradient hues) is GONE; in its place
// the healthy dot takes one green breath every time the cable actually
// delivers something — a heartbeat on traffic, silence at rest.

// dotPulseTicksTotal: one inhale-exhale, ~1.2s of fast-loop ticks.
const dotPulseTicksTotal = int64(1200 * time.Millisecond / shimmerTick)

// beginDotPulse arms the pulse window (called from onCableEvent for
// every delivered message). The caller keeps ticks flowing via
// animate(), same as the ripple.
func (m *Model) beginDotPulse() {
	if !cableActivityPulseEnabled || !m.truecolor {
		return
	}
	m.dotPulseTicks = dotPulseTicksTotal
}

// activityPulseDot renders the presence dot mid-pulse: a sine breath
// between dim green and the full OK green — green only (owner ruling),
// never the gradient hues the retired idle breath used.
func (m Model) activityPulseDot() string {
	progress := 1 - float64(m.dotPulseTicks)/float64(dotPulseTicksTotal)
	pulse := math.Sin(progress * math.Pi) // 0→1→0 over the window
	const floor = 0.45
	t := floor + pulse*(1-floor)
	c := render.RGB{
		R: uint8(0x5f * t / 1),
		G: uint8(float64(0xd7) * t),
		B: uint8(float64(0x87) * t),
	}
	return lipgloss.NewStyle().Foreground(hexColor(c)).Render("■")
}

// ── shared helpers ────────────────────────────────────────────────────────

// lerpRGB blends a→b at t (clamped to [0,1] — callers may hand in a
// sine that overshoots at the float edges).
func lerpRGB(a, b render.RGB, t float64) render.RGB {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return render.RGB{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B)}
}

func hexColor(c render.RGB) color.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B))
}
