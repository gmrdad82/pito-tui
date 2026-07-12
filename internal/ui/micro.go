package ui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// The ambassador wave's own micro-interaction pass: four whisper-intensity
// reactions to things the user just DID or just SAW, layered on top of
// ripple.go's live-data reactions and ambient.go's pure ambience. Same
// house law as both of those files: each is independently kill-switchable
// below, and every one is additionally gated on m.truecolor at its call
// site — non-truecolor terminals, and every golden test (none of which
// set WithTruecolor), get none of this.
//
//   - errorShakeEnabled: a freshly-appended error-kind event jitters
//     horizontally for its first ~200ms (5 ticks of left-pad offset) —
//     see beginErrorShake, triggered from Model.onCableEvent's
//     event.append case, and render.R.SetShake/applyShake, the
//     transcript's own per-event render seam (render.go's Event).
//   - confirmGlintEnabled: a pending confirmation's warn border already
//     pulses (render.go's pulseWarn); this adds a single-cell bright
//     glint sliding top to bottom once every ~4s. Needs no new Model
//     field or gate entry — it rides m.aliveTicks, the same clock
//     ambient.go's shimmer conductor already uses, and a pending
//     confirmation already holds the fast gate open via m.shimmer. See
//     confirmGlintProgress here and render.go's confirmChrome.
//   - ghostTypeEnabled: the palette's ghost completion hint (the
//     footer's Ghost.NextHint — Ghost.CompleteCurrent renders nowhere
//     in paletteView today, so NextHint is the seam that actually
//     exists) types itself in left-to-right over ~150ms (4 ticks)
//     whenever the hint TEXT changes, instead of popping — see
//     beginGhostType/ghostDisplay, Model.paletteView's own seam.
//   - scrollThumbEnabled: while the viewport sits scrolled away from the
//     bottom (m.follow false), a 1-column indicator rides the gutter
//     just right of the content column, fading out over ~800ms once
//     follow flips back to true — see setFollow/paintScrollThumbOverlay,
//     Model.viewContent's post-processing pass (ambient.go's own
//     paintMarginStars, same shape).
const (
	errorShakeEnabled   = true
	confirmGlintEnabled = true
	ghostTypeEnabled    = true
	scrollThumbEnabled  = true
)

// ── Effect 1: error shake ────────────────────────────────────────────────

// errorShake is one error-kind event's in-flight jitter: turnID (for
// Transcript.Touch dirtying, mirroring revealSpring's own field) and tick,
// the index into errorShakeOffsets this event is currently showing.
type errorShake struct {
	turnID int64
	tick   int
}

// errorShakeOffsets is the jitter's own fixed 5-tick sequence (~200ms at
// the house's 40ms tick) — a left-pad cell kicked right, overcorrected
// back, kicked right again, then two resting ticks so the sequence's own
// length is what onAnimTick counts against rather than a separate
// "settled" test on its last value. The "-1" ticks never actually pad
// LEFT of a block's resting column (there's no room to — see
// render.applyShake); they simply read as "back to rest" for one beat
// between the two rightward kicks, which is what reads as a shake rather
// than a single one-shot nudge.
// Stretched for the 60fps loop (each offset holds ~2 frames ≈ the
// original 200ms sequence at 25fps).
var errorShakeOffsets = [10]int{1, 1, -1, -1, 1, 1, 0, 0, 0, 0}

// shakeOffset is errorShakeOffsets' own pure lookup: tick → cell offset,
// 0 for any tick outside the sequence (onAnimTick drops an event's entry
// the moment its tick walks off the end, so this is a defensive floor,
// not a path anything here actually relies on).
func shakeOffset(tick int) int {
	if tick < 0 || tick >= len(errorShakeOffsets) {
		return 0
	}
	return errorShakeOffsets[tick]
}

// beginErrorShake starts eventID's own jitter from tick 0 — called once,
// the moment an error-kind event lands on the transcript (onCableEvent's
// event.append case). Never re-triggered by anything else that touches
// the same event afterward.
func (m *Model) beginErrorShake(ev api.Event) {
	if !errorShakeEnabled || !m.truecolor {
		return
	}
	m.shaking[ev.ID] = errorShake{turnID: ev.TurnID}
}

// pushShakeOffsets recomputes every in-flight shake's CURRENT cell offset
// and hands the whole map to the renderer in one call — used right after
// a fresh beginErrorShake (so the very first paint after an error event
// lands already shows tick 0's offset, not a beat late) and again every
// onAnimTick as ticks advance. An empty map renders identically to a nil
// one (render.R.shakeFor's own "absent ⇒ at rest" rule), so this always
// pushes SOMETHING rather than special-casing the empty case.
func (m *Model) pushShakeOffsets() {
	if m.renderer == nil {
		return
	}
	if len(m.shaking) == 0 {
		m.renderer.SetShake(nil)
		return
	}
	offsets := make(map[int64]int, len(m.shaking))
	for id, sh := range m.shaking {
		offsets[id] = shakeOffset(sh.tick)
	}
	m.renderer.SetShake(offsets)
}

// ── Effect 2: confirm glint ──────────────────────────────────────────────

// glintCycleTicks/glintWindowTicks translate the brief's "once every ~4s,
// sliding over ~500ms" into shimmerTick (40ms) counts — the cycle is an
// exact division (100 ticks); the window rounds to 12 ticks (480ms,
// close enough to the "~500ms" ask that owns the tilde).
const (
	glintCycleTicks  = int64(4 * time.Second / shimmerTick)
	glintWindowTicks = int64(500 * time.Millisecond / shimmerTick)
)

// confirmGlintProgress projects aliveTicks onto the glint's own sweep
// window inside its cycle: -1 outside the window (and ALWAYS -1 when the
// effect is off — the kill switch, same shape as ambient.go's
// conductorWeight returning 0), 0..1 climbing linearly across the window
// otherwise. render.go's confirmChrome reads a non-negative result as
// "this tick's glint sits `progress` of the way down whatever block is
// being painted," resolving that fraction against its OWN row count
// since different confirmations have different heights.
func confirmGlintProgress(aliveTicks int64) float64 {
	if !confirmGlintEnabled {
		return -1
	}
	pos := aliveTicks % glintCycleTicks
	if pos < 0 {
		pos += glintCycleTicks
	}
	if pos >= glintWindowTicks {
		return -1
	}
	if glintWindowTicks <= 1 {
		return 0
	}
	return float64(pos) / float64(glintWindowTicks-1)
}

// ── Effect 3: ghost typing ───────────────────────────────────────────────

// ghostTypeTicks: 4 discrete reveal steps (~150ms at the house's 40ms
// tick) — the brief's own duration for the palette ghost hint's type-in.
const ghostTypeTicks = int64(160 * time.Millisecond / shimmerTick)

// ghostRevealedText slices target rune-by-rune to tick's reveal progress:
// tick <= 0 is empty, tick >= ghostTypeTicks is the full string, every
// tick between adds roughly a quarter of the string's OWN rune count
// (not a fixed character count) so short and long hints both finish on
// the same tick.
func ghostRevealedText(target string, tick int64) string {
	if tick <= 0 || target == "" {
		return ""
	}
	if tick >= ghostTypeTicks {
		return target
	}
	runes := []rune(target)
	n := int(int64(len(runes)) * tick / ghostTypeTicks)
	return string(runes[:n])
}

// beginGhostType starts (or ignores) next's own type-in: a hint IDENTICAL
// to the one already showing (whether mid-reveal or long settled) is a
// no-op — "when the ghost TEXT CHANGES" is the brief's own trigger, not
// every suggestions refresh. An empty next needs no reveal at all; it's
// recorded as the new target (so a later repeat of the SAME empty hint
// stays a no-op too) but never sets ghostTyping.
func (m *Model) beginGhostType(next string) {
	if !ghostTypeEnabled || !m.truecolor || next == m.ghostTarget {
		return
	}
	m.ghostTarget = next
	m.ghostTick = 0
	m.ghostTyping = next != ""
}

// ghostDisplay is what the palette footer actually shows for hint: the
// live left-to-right reveal while it's still m.ghostTarget (the type-in
// this hint started is in flight, or has since settled), or hint
// untouched otherwise — covering both "the effect is off" and the
// defensive case of a render racing its own trigger (shouldn't happen
// given beginGhostType's call site, but showing the settled text beats a
// phantom empty string either way).
func (m Model) ghostDisplay(hint string) string {
	if !ghostTypeEnabled || !m.truecolor || m.ghostTarget != hint {
		return hint
	}
	return ghostRevealedText(hint, m.ghostTick)
}

// overlayBottom paints `over` onto the BOTTOM lines of `base`, each
// overlay line padded with spaces to width so it fully masks whatever
// conversation text sits underneath — the suggestions palette's no-
// layout-shift seam (owner 2026-07-12). base's line count never changes;
// an overlay taller than base just claims every line.
func overlayBottom(base, over string, width int) string {
	baseLines := strings.Split(base, "\n")
	overLines := strings.Split(over, "\n")
	start := len(baseLines) - len(overLines)
	if start < 0 {
		overLines = overLines[-start:]
		start = 0
	}
	for i, line := range overLines {
		if pad := width - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		baseLines[start+i] = line
	}
	return strings.Join(baseLines, "\n")
}

// ── Effect 4: scroll thumb ───────────────────────────────────────────────

// thumbFadeTicks: ~800ms of ticks (40ms) the thumb takes to dim back to
// nothing once follow flips true — an exact division, 20 ticks.
const thumbFadeTicks = int64(800 * time.Millisecond / shimmerTick)

// scrollThumbTrack/scrollThumbBG reuse ambient.go's own ColorFaint
// approximation and near-background floor — the same whisper-intensity
// bounds the star field already established, not a new palette to keep
// in sync.
var (
	scrollThumbTrack = ambientStarFaint
	scrollThumbBG    = ambientStarBG
)

// setFollow is the one seam every follow-state mutation in onChatKey
// routes through, replacing bare `m.follow = ...` assignments. The
// scroll-thumb's fade-out (this effect) starts the instant follow flips
// false→true — whether the user scrolled there or jumped with G — and
// cancels back to the thumb's plain, always-on, unfaded look the instant
// it flips back to false, so a quick up/down/up flick never leaves a
// half-faded ghost thumb on screen.
func (m *Model) setFollow(v bool) {
	if scrollThumbEnabled && m.truecolor && v && !m.follow {
		m.thumbFadeTick = 0
		m.thumbFading = true
	}
	if !v {
		m.thumbFading = false
	}
	m.follow = v
}

// thumbGeometry maps the viewport's own visible/total line counts and
// scroll percent onto the gutter track's row span: thumbRows (how many
// track rows the thumb covers, sized to the visible/total ratio, floored
// at 1 so even a huge scrollback still shows SOME thumb, capped at
// trackHeight) and thumbTop (which row, 0-indexed from the track's own
// top, the thumb starts at — percent 0 pins it to the track's top,
// percent 1 to its bottom, everything between interpolated across
// whatever room the thumb itself doesn't occupy).
func thumbGeometry(trackHeight, visible, total int, percent float64) (thumbTop, thumbRows int) {
	if trackHeight <= 0 || total <= 0 {
		return 0, 0
	}
	thumbRows = trackHeight * visible / total
	if thumbRows < 1 {
		thumbRows = 1
	}
	if thumbRows > trackHeight {
		thumbRows = trackHeight
	}
	span := trackHeight - thumbRows
	thumbTop = int(float64(span)*percent + 0.5)
	if thumbTop < 0 {
		thumbTop = 0
	}
	if thumbTop > span {
		thumbTop = span
	}
	return thumbTop, thumbRows
}

// paintScrollThumbOverlay overlays the scroll-thumb indicator in the
// gutter column immediately right of the content column, across exactly
// the chat viewport's own rows — viewport.Model.View() always pads to
// chatViewportHeight() lines (bubbles' own Height()-styled render), and
// the viewport is unconditionally sections[0] in viewContent, so body's
// first vpHeight lines ARE the viewport, nothing else. A post-processing
// pass over the assembled frame, same shape as ambient.go's
// paintMarginStars: never touches how the viewport rendered its own
// content, and skips a row outright rather than risk stamping over it if
// that row's own content already reaches (or overflows past) the gutter
// column.
func (m Model) paintScrollThumbOverlay(body string) string {
	termWidth := m.width
	// The thumb rides the terminal's LAST column — like every scrollbar
	// (owner: "I love the scrollbar", 2026-07-12) — regardless of the
	// width-cap mode. Rows whose own content reaches the column are
	// skipped below rather than overpainted.
	if termWidth < 60 {
		return body
	}
	contentWidth := termWidth - 1
	vpHeight := m.chatViewportHeight()
	lines := strings.Split(body, "\n")
	if vpHeight > len(lines) {
		vpHeight = len(lines)
	}
	total := m.sc.TotalLineCount()
	if total <= 0 {
		return body
	}
	thumbTop, thumbRows := thumbGeometry(vpHeight, m.sc.VisibleLineCount(), total, m.sc.ScrollPercent())
	weight := 1.0
	if m.thumbFading {
		weight = 1 - float64(m.thumbFadeTick)/float64(thumbFadeTicks)
	}
	for row := 0; row < vpHeight; row++ {
		// The viewport pads every line to ITS OWN full width (bubbles'
		// own Height()/Width()-styled View()), so lipgloss.Width(line)
		// alone can't tell real content from that trailing fill — trim
		// the plain trailing spaces first (Width()'s own fill is always
		// unstyled; see the doc comment above) and measure what's left.
		// A row genuinely too wide even after trimming is skipped
		// outright, untouched, rather than risk stamping over it.
		trimmed := strings.TrimRight(lines[row], " ")
		w := lipgloss.Width(trimmed)
		if w > contentWidth {
			continue
		}
		pad := strings.Repeat(" ", contentWidth-w)
		glyph, c := '⡀', lerpRGB(scrollThumbBG, scrollThumbTrack, weight)
		if row >= thumbTop && row < thumbTop+thumbRows {
			t := 0.0
			if thumbRows > 1 {
				t = float64(row-thumbTop) / float64(thumbRows-1)
			}
			glyph, c = '⣿', lerpRGB(scrollThumbBG, render.PitoShimmer.At(t), weight)
		}
		lines[row] = trimmed + pad + lipgloss.NewStyle().Foreground(hexColor(c)).Render(string(glyph))
	}
	return strings.Join(lines, "\n")
}
