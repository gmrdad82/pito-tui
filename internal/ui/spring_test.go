package ui

import (
	"testing"
)

// TestOverlaySpringSettlesFastWithSlightOvershoot pins the overlay
// profile's own numbers: it must settle well inside the reveal spring's
// 600ms budget (the picker/notifications slide is meant to feel snappier
// than a chart growing in) and stay under the same 8% overshoot bound.
func TestOverlaySpringSettlesFastWithSlightOvershoot(t *testing.T) {
	pos, vel := 0.0, 0.0
	const maxTicks = 40
	settledAt := -1
	peak := 0.0
	for i := 1; i <= maxTicks; i++ {
		pos, vel = overlaySpringPhysics.Update(pos, vel, 1.0)
		if pos > peak {
			peak = pos
		}
		if settledAt == -1 && springSettled(pos, vel, 1.0) {
			settledAt = i
		}
	}
	if settledAt == -1 {
		t.Fatalf("overlay spring did not settle within %d ticks", maxTicks)
	}
	if settledAt*16 > 300 {
		t.Errorf("overlay spring settled at tick %d (%dms) — expected a snappier open than the reveal spring's budget", settledAt, settledAt*16)
	}
	if peak > 1.08 {
		t.Errorf("peak overshoot = %.4f, want <= 1.08", peak)
	}
}

// TestOverlayAnimStepSettlesAndClearsClosing exercises overlayAnim's own
// state machine directly: stepping toward target 1 while active climbs
// to (1,~0); flagging closing flips the target to 0 regardless of
// active, and settling there clears the closing flag (ready for the
// next open from a standing start).
func TestOverlayAnimStepSettlesAndClearsClosing(t *testing.T) {
	var a overlayAnim
	for i := 0; i < 30 && !a.settled(true); i++ {
		a = a.step(true)
	}
	if !a.settled(true) {
		t.Fatalf("overlayAnim did not settle open within 30 ticks: %+v", a)
	}
	if a.pos < 0.99 {
		t.Errorf("settled open pos = %v, want ~1", a.pos)
	}

	// The real caller (stepOverlays) flips Model.mode away the instant
	// closing clears, so active would go false on the very next tick —
	// this loop mirrors that by stopping there too, instead of feeding
	// active=true forever (which would just reopen the spring right back
	// toward 1 once closing clears).
	a.closing = true
	for i := 0; i < 30 && a.closing; i++ {
		a = a.step(true)
	}
	if a.closing {
		t.Errorf("closing flag must clear once the spring settles at 0")
	}
	if a.pos > 0.01 || a.vel > 0.01 {
		t.Errorf("settled closed state = %+v, want pos/vel ~0", a)
	}
}
