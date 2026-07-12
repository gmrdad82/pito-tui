package ui

import (
	"math"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/cable"
)

// ── Effect 1: status-bar ripple ──────────────────────────────────────────

// The pure sweep: a single linear left-to-right pass, clamped at both
// ends, never regressing mid-flight.
func TestRippleProgressSweepsLeftToRightOnce(t *testing.T) {
	if got := rippleProgress(0); got != 0 {
		t.Errorf("progress at tick 0 = %v, want 0", got)
	}
	if got := rippleProgress(-5); got != 0 {
		t.Errorf("progress before the start must clamp to 0, got %v", got)
	}
	prev := 0.0
	for tick := int64(1); tick <= rippleDurationTicks; tick++ {
		got := rippleProgress(tick)
		if got < prev {
			t.Fatalf("progress must never regress: tick %d = %v after %v", tick, got, prev)
		}
		prev = got
	}
	if got := rippleProgress(rippleDurationTicks); got != 1 {
		t.Errorf("progress at the duration's own last tick = %v, want 1", got)
	}
	if got := rippleProgress(rippleDurationTicks + 100); got != 1 {
		t.Errorf("progress past the duration must clamp to 1, got %v", got)
	}
}

// The window itself: nothing lit before it arrives, full intensity dead
// center, and its peak column must travel strictly rightward as the tick
// advances — the actual "sweeps left to right" proof, independent of any
// Model/statusLine plumbing.
func TestRippleIntensityWindowTravelsAcrossLine(t *testing.T) {
	const lineWidth = 60
	if got := rippleIntensity(lineWidth/2, lineWidth, 0); got != 0 {
		t.Errorf("mid-line intensity at tick 0 = %v, want 0 (window hasn't arrived yet)", got)
	}
	if got := rippleIntensity(-1, lineWidth, rippleDurationTicks/2); got != 0 {
		t.Errorf("out-of-range column must never read positive intensity, got %v", got)
	}

	// peakCol returns the column with the strongest pull at this tick, or
	// -1 if nothing is lit at all (the window hasn't arrived yet, or has
	// already fully swept past — both legitimate at the very ends of the
	// sweep, by the "symmetric pass" design: the window travels from half
	// a span BEFORE column 0 to half a span PAST lineWidth).
	peakCol := func(tick int64) int {
		best, bestT := -1, 0.0
		for col := 0; col <= lineWidth; col++ {
			if in := rippleIntensity(col, lineWidth, tick); in > bestT {
				bestT, best = in, col
			}
		}
		return best
	}
	lastPeak := -1
	for _, tick := range []int64{1, 4, 8, 12, 14} {
		p := peakCol(tick)
		if p == -1 {
			t.Fatalf("tick %d: expected a real peak column inside the sweep, got none", tick)
		}
		if p < lastPeak {
			t.Errorf("tick %d: peak column %d regressed behind the previous peak %d", tick, p, lastPeak)
		}
		lastPeak = p
	}
	if first, last := peakCol(1), peakCol(14); first >= last {
		t.Errorf("peak column must have advanced overall: tick 1 peak=%d, tick 14 peak=%d", first, last)
	}

	// Both ends of the sweep are legitimately dark: half a span before
	// the window has arrived, and half a span after it has fully exited
	// past the right edge.
	if p := peakCol(0); p != -1 {
		t.Errorf("tick 0 (window not yet arrived) must light nothing, got peak col %d", p)
	}
	if p := peakCol(rippleDurationTicks); p != -1 {
		t.Errorf("the final tick (window fully exited) must light nothing, got peak col %d", p)
	}

	// Dead center of the window is exactly 1, by construction (dist=0 ⇒
	// 1-0/half=1). lineWidth=29/tick=9 is chosen so the window's center
	// lands exactly on an integer column at the 60fps tick budget
	// (rippleDurationTicks=37): span=29+8=37, center=9/37×37-4=5 — no
	// rounding slop to account for.
	const preciseLineWidth = 29
	if got := rippleIntensity(5, preciseLineWidth, 9); math.Abs(got-1) > 1e-9 {
		t.Errorf("window center must read exactly 1, got %v", got)
	}
	// A neighbor one cell off-center must read strictly less.
	if got := rippleIntensity(6, preciseLineWidth, 9); got >= 1 {
		t.Errorf("a column off the exact center must read below 1, got %v", got)
	}
}

// Model-level integration: a conversation.update opens the ripple window
// and the animation gate with it; somewhere inside the window the status
// line actually renders differently (the middots really do recolor, not
// just an inert counter ticking); once the window's own budget elapses
// the gate closes and the line matches its pre-ripple self byte for byte.
func TestStatusRippleTravelsThenSettlesBackToPlain(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"), WithTruecolor(true))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())
	// The slimmed status (dot+tag) only grows a separator once the ⚑
	// badge shows — give the ripple a middot to travel through.
	m.unread = 5

	settled := m.statusLine()

	m = drive(m, CableEventMsg{M: cable.StreamMessage{Type: cable.TypeConversationUpdate}})
	if !m.rippleAnim {
		t.Fatal("a conversation.update must open the ripple window, regardless of which fields it carries")
	}
	if !m.animating {
		t.Fatal("the ripple must open the animation gate")
	}

	sawDifference := false
	for i := int64(0); i < rippleDurationTicks; i++ {
		m = drive(m, AnimTickMsg{})
		if m.statusLine() != settled {
			sawDifference = true
		}
	}
	if !sawDifference {
		t.Error("no tick during the ripple window rendered a status line different from the settled one")
	}
	if m.rippleAnim {
		t.Errorf("ripple must have closed within %d ticks (600ms)", rippleDurationTicks)
	}
	if got := m.statusLine(); got != settled {
		t.Errorf("status line after the ripple closes must match the pre-ripple line exactly:\ngot:  %q\nwant: %q", got, settled)
	}
}

// Non-truecolor terminals get none of it: beginRipple is a no-op, so the
// gate never opens on its account.
func TestStatusRippleNeverStartsWithoutTruecolor(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())

	m = drive(m, CableEventMsg{M: cable.StreamMessage{Type: cable.TypeConversationUpdate}})
	if m.rippleAnim {
		t.Error("non-truecolor terminals must never start the status-bar ripple")
	}
}

// ── Effect 2: unread odometer ────────────────────────────────────────────

func TestUnreadOdometerValueMonotonicAndBookended(t *testing.T) {
	// Rising: 3 → 7.
	prev := unreadOdometerValue(3, 7, 0)
	if prev != 3 {
		t.Fatalf("tick 0 must read the FROM value, got %d", prev)
	}
	for tick := int64(1); tick <= unreadOdoTicks; tick++ {
		v := unreadOdometerValue(3, 7, tick)
		if v < prev {
			t.Fatalf("rising roll must never regress: tick %d = %d after %d", tick, v, prev)
		}
		prev = v
	}
	if prev != 7 {
		t.Errorf("settled value must be the TO value, got %d", prev)
	}

	// Falling: 7 → 2, same monotonic contract in the other direction.
	prev = unreadOdometerValue(7, 2, 0)
	for tick := int64(1); tick <= unreadOdoTicks; tick++ {
		v := unreadOdometerValue(7, 2, tick)
		if v > prev {
			t.Fatalf("falling roll must never rise: tick %d = %d after %d", tick, v, prev)
		}
		prev = v
	}
	if prev != 2 {
		t.Errorf("settled value must be the TO value, got %d", prev)
	}

	// Past the budget, pinned at the target regardless of how far tick
	// overshoots.
	if got := unreadOdometerValue(3, 7, unreadOdoTicks+50); got != 7 {
		t.Errorf("overshoot past the budget must clamp to the target, got %d", got)
	}
}

// Model-level: a real notifications count change on the cable rolls the
// displayed badge from old to new over the window, then settles exactly
// on the target and closes its own share of the gate.
func TestUnreadOdometerModelFlowRollsThenSettles(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"), WithTruecolor(true))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())
	m.unread = 3

	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:          cable.TypeConversationUpdate,
		Notifications: &api.NotifCount{Unread: 7},
	}})
	if !m.unreadOdoAnim {
		t.Fatal("an actual unread change must start the roll")
	}
	if m.unread != 7 {
		t.Fatalf("the target updates immediately even mid-roll, got %d", m.unread)
	}
	if m.unreadFrom != 3 {
		t.Fatalf("roll must start from the pre-change value, got %d", m.unreadFrom)
	}

	values := []int{m.displayUnread()}
	for i := int64(0); i < unreadOdoTicks; i++ {
		m = drive(m, AnimTickMsg{})
		values = append(values, m.displayUnread())
	}
	for i := 1; i < len(values); i++ {
		if values[i] < values[i-1] {
			t.Fatalf("displayed value must never regress mid-roll: %v", values)
		}
	}
	if values[0] != 3 {
		t.Errorf("first displayed value must be the pre-change count, got %d", values[0])
	}
	if got := values[len(values)-1]; got != 7 {
		t.Errorf("settled displayed value must be the new count, got %d", got)
	}
	if m.unreadOdoAnim {
		t.Error("the roll must have finished within its own ~400ms budget")
	}

	// An unchanged count arriving afterward must not restart anything.
	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:          cable.TypeConversationUpdate,
		Notifications: &api.NotifCount{Unread: 7},
	}})
	if m.unreadOdoAnim {
		t.Error("a conversation.update with the SAME unread value must not restart the roll")
	}
}

// Off-truecolor terminals get the brief's own plain instant swap — no
// animation, the final value on screen the moment the message lands
// (matches the pre-existing TestContextMeterAndMiniStatusFlow contract).
func TestUnreadOdometerNonTruecolorInstantSwap(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithConversation("u1"))
	m = sized(m)
	m = drive(m, m.fetchChatCmd("u1", false)())
	m.unread = 28

	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:          cable.TypeConversationUpdate,
		Notifications: &api.NotifCount{Unread: 3},
	}})
	if m.unreadOdoAnim {
		t.Error("non-truecolor terminals must never animate the odometer")
	}
	if m.unread != 3 || m.displayUnread() != 3 {
		t.Errorf("non-truecolor must swap instantly: unread=%d display=%d", m.unread, m.displayUnread())
	}
	if !strings.Contains(m.viewContent(), "⚑ 3") {
		t.Errorf("status line must show the final value immediately:\n%s", m.viewContent())
	}
}

// ── Effect 3: ✦ badge pop (verifying the pre-existing wiring) ────────────

// ── Effect 4: comet tail ─────────────────────────────────────────────────

var plainCometFrames = []string{
	"●∙∙∙∙∙∙∙", "∙●∙∙∙∙∙∙", "∙∙●∙∙∙∙∙", "∙∙∙●∙∙∙∙",
	"∙∙∙∙●∙∙∙", "∙∙∙∙∙●∙∙", "∙∙∙∙∙∙●∙", "∙∙∙∙∙∙∙●",
}

// The pure per-cell rule: full brand at the head, a monotonically fading
// tail exactly at the brief's own 60/35/15% behind it, dim everywhere
// else (both further back AND still ahead — unswept).
func TestCometCellColorFadesTailMonotonically(t *testing.T) {
	const head = 5
	if got := cometCellColor(head, head); got != cometBrandRGB {
		t.Errorf("the head cell must be full brand color, got %+v", got)
	}
	prevWeight := 1.0
	for behind := 1; behind <= 3; behind++ {
		c := cometCellColor(head, head-behind)
		want := lerpRGB(cometDimRGB, cometBrandRGB, cometTailWeights[behind-1])
		if c != want {
			t.Errorf("tail cell %d behind the head = %+v, want %+v", behind, c, want)
		}
		// Brightness must strictly fade with distance from the head —
		// check via the weight table directly (cometTailWeights is
		// already documented as descending; this pins that it's actually
		// consumed in order).
		if cometTailWeights[behind-1] >= prevWeight {
			t.Fatalf("cometTailWeights must strictly descend, got %v", cometTailWeights)
		}
		prevWeight = cometTailWeights[behind-1]
	}
	if got := cometCellColor(head, head-4); got != cometDimRGB {
		t.Errorf("4 cells behind the head must be the flat dim floor, got %+v", got)
	}
	if got := cometCellColor(head, head+1); got != cometDimRGB {
		t.Errorf("a cell still ahead of the head (unswept) must be the flat dim floor, got %+v", got)
	}
}

// Truecolor only: a plain model keeps the original, byte-identical comet
// literal (golden safety); a truecolor model gets the recolored 8-cell
// variant with the same visible width per frame.
func TestCometTailOnlyAppliesInTruecolor(t *testing.T) {
	plain, _ := newTestModel(t, chatServer(t))
	for i, want := range plainCometFrames {
		if got := plain.spin.Spinner.Frames[i]; got != want {
			t.Errorf("non-truecolor frame %d = %q, want unchanged %q", i, got, want)
		}
	}

	tc, _ := newTestModel(t, chatServer(t), WithTruecolor(true))
	if len(tc.spin.Spinner.Frames) != cometCells {
		t.Fatalf("truecolor comet must keep the same %d-cell shape, got %d frames", cometCells, len(tc.spin.Spinner.Frames))
	}
	for i, frame := range tc.spin.Spinner.Frames {
		if frame == plainCometFrames[i] {
			t.Errorf("truecolor frame %d must be recolored, got the untouched plain frame", i)
		}
		if w := lipgloss.Width(frame); w != cometCells {
			t.Errorf("frame %d visible width = %d, want %d (color codes must not change cell count)", i, w, cometCells)
		}
	}
}

// ── gate bookkeeping ──────────────────────────────────────────────────────

// Both windowed effects must fold into the shared animation gate on the
// way in, and drop back out once their own budget elapses — the house
// "no springs active ⇒ no ticks" rule (model.go's animGateOpen).
func TestRippleAndOdometerCloseTheGateOnTheirOwnBudgets(t *testing.T) {
	m := Model{truecolor: true, mode: modeChat}
	m.beginRipple()
	m.beginUnreadRoll(5)
	if !m.animGateOpen() {
		t.Fatal("either window in flight must hold the gate open")
	}

	maxTicks := rippleDurationTicks
	if unreadOdoTicks > maxTicks {
		maxTicks = unreadOdoTicks
	}
	for i := int64(0); i < maxTicks; i++ {
		if m.rippleAnim {
			m.rippleTick++
			if m.rippleTick >= rippleDurationTicks {
				m.rippleAnim = false
			}
		}
		if m.unreadOdoAnim {
			m.unreadOdoTick++
			if m.unreadOdoTick >= unreadOdoTicks {
				m.unreadOdoAnim = false
			}
		}
	}
	if m.animGateOpen() {
		t.Errorf("the gate must close once both windows finish: rippleAnim=%v unreadOdoAnim=%v", m.rippleAnim, m.unreadOdoAnim)
	}
}
