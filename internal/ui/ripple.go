package ui

import (
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// The ambassador wave: four more whisper-intensity effects, live-data
// reactions this time rather than pure ambience — something on the wire
// actually changed. Same house law as ambient.go: each is independently
// kill-switchable, and every one of them (bar effect 3, which needed no
// new code — see its own section below) is additionally gated on
// m.truecolor at its call site, so non-truecolor terminals and every
// golden test (none of which set WithTruecolor) get none of it.
//
//   - statusRippleEnabled: a conversation.update landing on the cable
//     opens a ~600ms window during which the status line's own " · "
//     separators sweep left to right through the brand gradient — see
//     beginRipple, pushed from Model.onCableEvent's TypeConversationUpdate
//     case, and joinWithRippleSeparators, statusLine's own assembly seam.
//   - unreadOdometerEnabled: when notifications.unread actually CHANGES,
//     the ✉ badge rolls to its new value over ~400ms instead of jump-
//     cutting — see beginUnreadRoll and displayUnread, statusLine's other
//     new seam. Off-truecolor terminals get the brief's own "plain
//     instant count swap" instead (beginUnreadRoll's own early return).
//   - (no flag) the ✦ badge pop: ALREADY DONE before this file existed —
//     see the doc comment on its own section below for the verification.
//   - cometTailEnabled: the pending-turn comet spinner's 8-cell sweep
//     gains a 3-cell fading brand-color tail behind its head — see
//     cometFrames, installed once in NewModel when m.truecolor is on
//     (Frames are fixed at spinner construction; there is no per-render
//     seam to hook the way the other three effects have one).
const (
	statusRippleEnabled   = true
	unreadOdometerEnabled = true
	cometTailEnabled      = true
)

// ── Effect 1: status-bar ripple ──────────────────────────────────────────

// rippleDurationTicks/rippleSpanCells translate the brief's "~600ms,
// ~8 cells wide" into shimmerTick (40ms) counts — 15 ticks, an exact
// division, the same 600ms budget revealSpringPhysics settles inside
// (spring.go).
const (
	rippleDurationTicks = int64(600 * time.Millisecond / shimmerTick)
	rippleSpanCells     = 8.0
)

// rippleDimRGB/rippleBrandRGB bound the middot's pulse: statusStyle's own
// resting gray (≈ ColorDim, 256-color 245) climbing to the brand's own
// leading stop (PitoShimmer's "brand pito" — gradient.go) at the window's
// peak. A color swap reads as a brightness pulse without the cost of
// re-styling every segment around it — see joinWithRippleSeparators for
// why the dots are the chosen seam rather than the whole line.
var (
	rippleDimRGB   = render.RGB{R: 0x8a, G: 0x8a, B: 0x8a}
	rippleBrandRGB = render.RGB{R: 0x51, G: 0x70, B: 0xff}
)

// beginRipple starts (or restarts, from a standing start) the status-bar
// ripple's own 600ms window. Called unconditionally from onCableEvent's
// TypeConversationUpdate case — the message's own arrival on the cable IS
// the trigger, regardless of which of its fields (context/notifications)
// actually changed this time.
func (m *Model) beginRipple() {
	if !statusRippleEnabled || !m.truecolor {
		return
	}
	m.rippleTick = 0
	m.rippleAnim = true
}

// rippleActive reports whether the status line should paint through
// ripple.go's seam right now.
func (m Model) rippleActive() bool {
	return statusRippleEnabled && m.truecolor && m.rippleAnim
}

// rippleProgress maps ticks-since-triggered to the pulse window's own
// travel fraction: one linear left-to-right pass, 0 at (or before) the
// start, 1 once the full duration has elapsed — no easing, no bounce,
// matching the brief's "travels the status line once."
func rippleProgress(tick int64) float64 {
	if tick <= 0 {
		return 0
	}
	if tick >= rippleDurationTicks {
		return 1
	}
	return float64(tick) / float64(rippleDurationTicks)
}

// rippleIntensity is how far INSIDE the pulse window column col (0-indexed
// from the status line's own left edge, ignoring the right-align pad) is
// at this tick: 0 outside the window, ramping to 1 at its dead center.
// The window's center sweeps from half a span before column 0 to half a
// span past lineWidth, so a separator sitting right at either edge still
// gets a full, symmetric pass through rather than being clipped by the
// window's own arrival/departure.
func rippleIntensity(col, lineWidth int, tick int64) float64 {
	if lineWidth <= 0 {
		return 0
	}
	half := rippleSpanCells / 2
	center := rippleProgress(tick)*(float64(lineWidth)+rippleSpanCells) - half
	dist := math.Abs(float64(col) - center)
	if dist >= half {
		return 0
	}
	return 1 - dist/half
}

// joinWithRippleSeparators joins pieces with " · " — statusLine's own
// assembly seam for this effect. Outside a ripple this is byte-identical
// to the plain join every prior statusLine build produced (statusStyle's
// own dim " · " between every pair, nothing else touched). Mid-ripple,
// each separator's own middot is recolored by rippleIntensity at its
// actual column, computed in a single forward pass: every piece is
// already fully rendered before it's handed here, so each one's on-screen
// width (and therefore the whole line's total width, and every
// separator's own column) is known up front — no second, measuring-only
// pass required. Re-styling every SEGMENT around the separators (the
// brief's first-choice "re-style only the plain-text gaps") would mean
// re-parsing already-styled ANSI runs to find where one style's run ends
// and the gap begins — fragile, per the brief's own warning — so the
// dots are the seam actually used here: cheap, and still legible as a
// ripple.
func (m Model) joinWithRippleSeparators(pieces []string) string {
	if len(pieces) == 0 {
		return ""
	}
	if !m.rippleActive() {
		var b strings.Builder
		for i, p := range pieces {
			if i > 0 {
				b.WriteString(statusStyle.Render(" · "))
			}
			b.WriteString(p)
		}
		return b.String()
	}
	total := 3 * (len(pieces) - 1) // (N-1) separators, each " · " = 3 cells
	for _, p := range pieces {
		total += lipgloss.Width(p)
	}
	var b strings.Builder
	col := 0
	for i, p := range pieces {
		if i > 0 {
			b.WriteString(m.rippleSeparator(col+1, total)) // the dot sits at offset 1 inside " · "
			col += 3
		}
		b.WriteString(p)
		col += lipgloss.Width(p)
	}
	return b.String()
}

// rippleSeparator renders one " · " with its middot alone recolored — the
// flanking spaces stay statusStyle's dim gray untouched either way, so
// only the glyph itself visibly pulses.
func (m Model) rippleSeparator(dotCol, lineWidth int) string {
	t := rippleIntensity(dotCol, lineWidth, m.rippleTick)
	if t <= 0 {
		return statusStyle.Render(" · ")
	}
	c := lerpRGB(rippleDimRGB, rippleBrandRGB, t)
	return statusStyle.Render(" ") + lipgloss.NewStyle().Foreground(hexColor(c)).Render("·") + statusStyle.Render(" ")
}

// ── Effect 2: unread odometer ────────────────────────────────────────────

// unreadOdoTicks: ~400ms of fast ticks (40ms) — an exact division, the
// brief's own duration for the ✉ badge's roll.
const unreadOdoTicks = int64(400 * time.Millisecond / shimmerTick)

// beginUnreadRoll starts (or redirects, mid-flight) the ✉ badge's roll
// toward newVal. Off-truecolor terminals get the brief's own "plain
// instant count swap": m.unread flips straight to newVal, no animation,
// no tick spent. A roll already in flight redirects FROM its current
// on-screen value (displayUnread), not the stale prior target, so a
// second conversation.update landing mid-roll never visibly jumps.
func (m *Model) beginUnreadRoll(newVal int) {
	if !unreadOdometerEnabled || !m.truecolor {
		m.unread = newVal
		m.unreadOdoAnim = false
		return
	}
	m.unreadFrom = m.displayUnread()
	m.unread = newVal
	m.unreadOdoTick = 0
	m.unreadOdoAnim = true
}

// unreadOdometerValue is the roll's pure interpolation: round(lerp(from,
// to, t)) at t = tick/unreadOdoTicks, clamped to the endpoints at (or
// past) either boundary tick — "keep it dumb and deterministic" (owner
// brief), so a falling count rolls down through the same intermediate
// integers a rising one rolls up through.
func unreadOdometerValue(from, to int, tick int64) int {
	if tick <= 0 {
		return from
	}
	if tick >= unreadOdoTicks {
		return to
	}
	t := float64(tick) / float64(unreadOdoTicks)
	return int(math.Round(float64(from) + (float64(to)-float64(from))*t))
}

// displayUnread is what the status line actually renders: the live
// interpolated value while a roll is in flight, otherwise the settled
// target — the one seam statusLine calls instead of reading m.unread
// directly.
func (m Model) displayUnread() int {
	if !unreadOdometerEnabled || !m.truecolor || !m.unreadOdoAnim {
		return m.unread
	}
	return unreadOdometerValue(m.unreadFrom, m.unread, m.unreadOdoTick)
}

// ── Effect 3: ✦ badge pop — ALREADY DONE, verified rather than re-built ──
//
// The brief's ask: when an ai event's payload transitions to status
// "done" (the final event.replace), that event's reveal spring should
// re-fire so the ✦ badge line (and the rest of the done chrome) pops in
// again rather than snapping onto the screen cold.
//
// model.go's onCableEvent already does exactly this, unconditionally, for
// every event.replace of an "ai"-kind event — see the TypeEventReplace
// case:
//
//	case cable.TypeEventReplace:
//		delete(m.pending, ev.TurnID)
//		m.transcript.Replace(ev)
//		m.markReveal(ev) // a fill landing via replace grows in too
//
// and markReveal's own guard —
//
//	if !m.truecolor || (ev.Kind != api.KindAi && !render.HasShimmer(ev.Payload)) {
//		return
//	}
//
// — passes any KindAi event straight through regardless of its own
// shimmer markup, so the final done-status replace always seeds a fresh
// revealSpring{} (pos=vel=0, turnID: ev.TurnID) in m.revealing: the exact
// "re-trigger the standard reveal spring for that event" the brief calls
// for, badge included, since the whole message re-springs together
// (spring.go's revealSpringPhysics — ~600ms settle, ~4% overshoot). This
// predates the ambassador wave (the reveal-on-replace wiring shipped in
// the glossy pass, well before this file existed), so there's nothing new
// to gate here and deliberately no flag. See
// TestReplaceDoneAiEventRefiresRevealForBadgePop (ripple_test.go) for the
// behavioral proof — it drives exactly this path end to end and asserts
// m.revealing gets that fresh entry.

// ── Effect 4: comet tail ─────────────────────────────────────────────────

// cometCells is the plain comet's own frame count (NewModel's Frames
// literal) — 8 cells, one head position per frame; cometFrames mirrors it
// exactly so the swap changes only what's INSIDE each cell, never the
// spinner's shape or cadence (FPS stays untouched too — see NewModel).
const cometCells = 8

// cometBrandRGB/cometDimRGB/cometTailWeights: the comet's head burns at
// full brand color; the 3 cells immediately behind it fade at the
// brief's own 60/35/15% intensity; everything further back (and every
// cell still ahead, not yet swept) sits at the dim floor — never fully
// dark, the same whisper-intensity law ambient.go's star field and
// breathing dot already follow.
var (
	cometBrandRGB    = render.RGB{R: 0x51, G: 0x70, B: 0xff}
	cometDimRGB      = render.RGB{R: 0x3a, G: 0x3a, B: 0x3a}
	cometTailWeights = [3]float64{0.60, 0.35, 0.15}
)

// cometCellColor is cometFrames' own per-cell color rule, pulled out as a
// pure function so the 60/35/15% fade can be asserted directly rather
// than reverse-parsed out of rendered ANSI: behind == 0 is the head
// (full brand color); 1..3 are the fading tail (lerpRGB toward the dim
// floor by how far behind the head they sit); anything else — further
// back, or still ahead of the head, not yet swept — is the flat dim
// floor.
func cometCellColor(head, cell int) render.RGB {
	switch behind := head - cell; {
	case behind == 0:
		return cometBrandRGB
	case behind >= 1 && behind <= len(cometTailWeights):
		return lerpRGB(cometDimRGB, cometBrandRGB, cometTailWeights[behind-1])
	default:
		return cometDimRGB
	}
}

// cometFrames builds the truecolor comet: the same 8-cell shape/cadence
// as the plain spinner.Spinner NewModel already wires up, just with every
// cell individually pre-styled via cometCellColor — head, fading tail,
// dim rest — so the result is a drop-in replacement for Spinner.Frames.
// Every cell is rendered explicitly, never left as bare unstyled text,
// because the caller (NewModel) sets the spinner's outer Style to the
// zero value once these frames are installed: an outer wrap would
// otherwise re-color the *whole* frame in one flat hue on render, erasing
// the tail's own gradient the instant it crossed an inner reset code.
func cometFrames() []string {
	frames := make([]string, cometCells)
	for head := 0; head < cometCells; head++ {
		var b strings.Builder
		for cell := 0; cell < cometCells; cell++ {
			glyph := "∙"
			if cell == head {
				glyph = "●"
			}
			c := cometCellColor(head, cell)
			b.WriteString(lipgloss.NewStyle().Foreground(hexColor(c)).Render(glyph))
		}
		frames[head] = b.String()
	}
	return frames
}
