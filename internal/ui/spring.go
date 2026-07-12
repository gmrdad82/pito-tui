package ui

import (
	"math"

	"github.com/charmbracelet/harmonica"
)

// The house spring physics (harmonica) — ONE consumer remains: the boot
// splash's rise-away (splash.go). The 2.0.0 smoke purged the other two
// motion systems for cause (owner order, 2026-07-12):
//
//   - the DRAWER slide (overlays rising from the bottom edge): its
//     clipping anchored panels to the bottom mid-slide but handed back a
//     top-anchored frame at settle — a visible snap that read as flicker
//     on short bodies (the show-vid picker). Overlays now open settled
//     and close immediately.
//   - the REVEAL grow-in (arriving blocks bouncing to full height):
//     re-revealing replaced blocks yanked a following viewport backward,
//     and on a 120Hz display the bounce read as delay, not craft. New
//     content lands at full height; the follow-glide (model.go's
//     easeTowardBottom) carries the arrival motion instead.
//
// A future motion retry needs CONSISTENT anchoring (slide-down-from-top,
// or keep-padded-at-settle) — see the runbook's smoke log item 45 for
// the full post-mortem.
var overlaySpringPhysics = harmonica.NewSpring(harmonica.FPS(60), 65.0, 0.75)

// springSettled reports whether a spring has effectively arrived at
// target: position within 0.1% and velocity within 0.001/tick — the
// shared "stop animating" threshold.
func springSettled(pos, vel, target float64) bool {
	return math.Abs(pos-target) < 0.001 && math.Abs(vel) < 0.001
}

// overlayAnim survives the drawer purge with exactly ONE consumer: the
// `?` keymap footer's open/close accordion (chrome.go steps it
// directly). The full-screen overlays no longer animate at all.
type overlayAnim struct {
	pos, vel float64
	closing  bool
}

func (a overlayAnim) target(active bool) float64 {
	if active && !a.closing {
		return 1
	}
	return 0
}

func (a overlayAnim) settled(active bool) bool {
	return springSettled(a.pos, a.vel, a.target(active))
}

func (a overlayAnim) step(active bool) overlayAnim {
	t := a.target(active)
	a.pos, a.vel = overlaySpringPhysics.Update(a.pos, a.vel, t)
	if t == 0 && springSettled(a.pos, a.vel, 0) {
		a.pos, a.vel, a.closing = 0, 0, false
	}
	return a
}
