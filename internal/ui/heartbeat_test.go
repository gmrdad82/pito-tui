package ui

// The ambient heartbeat's activity gating (2026-07-21, the idle-CPU
// cut): focus pause/resume, the idle ladder, and the @ai pending→done
// migration off the fast chain. Every test drives a pinned or manually
// advanced clock — no sleeps, no wall-clock assertions (the repo's
// flaky-timing lesson).

import (
	"net/http"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/cable"
)

// clockModel builds a truecolor, sky-on model whose clock the test can
// advance by hand (the harness's fixedNow stays the epoch).
func clockModel(t *testing.T, opts ...Option) (Model, *time.Time) {
	t.Helper()
	now := fixedNow
	opts = append([]Option{
		WithTruecolor(true),
		WithStarSky(true),
		WithNow(func() time.Time { return now }),
	}, opts...)
	m, _ := newTestModel(t, http.NotFoundHandler(), opts...)
	return sized(m), &now
}

// The blur round trip: blur parks the heartbeat with the sky frozen in
// place; focus-in re-arms it and the very next tick resumes the drift
// exactly one step from where it paused — no jump-cut, no wall-clock
// catch-up.
func TestBlurParksHeartbeatAndFocusResumesInPlace(t *testing.T) {
	m, _ := clockModel(t)
	if !m.heartbeatTicking {
		t.Fatal("a sized truecolor model must have the heartbeat running")
	}

	m = drive(m, tea.BlurMsg{})
	if m.focused {
		t.Fatal("BlurMsg must clear focused")
	}
	pausedPhase := m.skyPhase
	m, cmd := driveCmd(m, HeartbeatTickMsg{})
	if cmd != nil || m.heartbeatTicking {
		t.Fatal("the first tick after blur must park the chain, not reschedule")
	}
	if m.skyPhase != pausedPhase {
		t.Fatal("blur must freeze skyPhase, not advance it")
	}

	m, cmd = driveCmd(m, tea.FocusMsg{})
	if !m.focused {
		t.Fatal("FocusMsg must set focused")
	}
	if cmd == nil || !m.heartbeatTicking {
		t.Fatal("focus-in must re-arm the heartbeat immediately")
	}
	m, cmd = driveCmd(m, HeartbeatTickMsg{})
	if cmd == nil {
		t.Fatal("the resumed heartbeat must reschedule")
	}
	if got, want := m.skyPhase, pausedPhase+skyPhaseStep; got != want {
		t.Fatalf("resume must continue from the paused phase: got %v, want %v", got, want)
	}
}

// With pause_on_blur off, blur changes nothing — the knob's contract.
func TestPauseOnBlurKnobOffKeepsTicking(t *testing.T) {
	m, _ := clockModel(t, WithFxTuning(FxTuning{
		PauseOnBlur: false,
		IdleGrace:   30 * time.Second,
		IdleFPS:     8,
		DeepIdle:    5 * time.Minute,
	}))
	m = drive(m, tea.BlurMsg{})
	before := m.skyPhase
	m, cmd := driveCmd(m, HeartbeatTickMsg{})
	if cmd == nil || !m.heartbeatTicking {
		t.Fatal("pause_on_blur=false must keep the heartbeat running while blurred")
	}
	if m.skyPhase == before {
		t.Fatal("the sky must keep drifting while blurred with the knob off")
	}
}

// The idle ladder, state by state, through the pure planner: full rate
// inside the grace window, the idle_fps interval beyond it, parked past
// deep-idle — and deep_idle_minutes=0 disables the park.
func TestHeartbeatPlanIdleLadder(t *testing.T) {
	base := fixedNow
	m := Model{
		focused:       true,
		lastActivity:  base,
		fxPauseOnBlur: true,
		fxIdleGrace:   30 * time.Second,
		fxIdleFPS:     8,
		fxDeepIdle:    5 * time.Minute,
	}
	cases := []struct {
		name     string
		idle     time.Duration
		interval time.Duration
		park     bool
	}{
		{"active", time.Second, heartbeatInterval, false},
		{"last active moment", 29 * time.Second, heartbeatInterval, false},
		{"idle-focused throttle", 31 * time.Second, time.Second / 8, false},
		{"still throttled", 4 * time.Minute, time.Second / 8, false},
		{"deep idle parks", 5 * time.Minute, 0, true},
		{"way past deep idle", time.Hour, 0, true},
	}
	for _, tc := range cases {
		interval, park := m.heartbeatPlan(base.Add(tc.idle))
		if interval != tc.interval || park != tc.park {
			t.Errorf("%s: plan = (%v, %v), want (%v, %v)", tc.name, interval, park, tc.interval, tc.park)
		}
	}

	m.fxDeepIdle = 0 // never park while focused
	if interval, park := m.heartbeatPlan(base.Add(time.Hour)); park || interval != time.Second/8 {
		t.Errorf("deep_idle=0: plan = (%v, %v), want the throttle floor, never parked", interval, park)
	}

	m.fxIdleFPS = 0 // defensive clamp: a broken config still animates
	if interval, park := m.heartbeatPlan(base.Add(time.Minute)); park || interval != time.Second {
		t.Errorf("idle_fps=0 must clamp to 1fps: got (%v, %v)", interval, park)
	}

	m.focused = false // blur wins over everything while the knob is on
	if _, park := m.heartbeatPlan(base.Add(time.Second)); !park {
		t.Error("blurred with pause_on_blur must park regardless of activity")
	}
}

// Under the idle throttle the phase step scales with the interval, so
// the sky's wall-clock drift speed never changes — same motion, fewer
// frames. And any input snaps the very next tick back to the full rate.
func TestIdleThrottleKeepsWallClockSpeedAndInputSnapsBack(t *testing.T) {
	m, now := clockModel(t)
	// Beyond the grace: the next tick reschedules at the throttle…
	*now = fixedNow.Add(time.Minute)
	m, cmd := driveCmd(m, HeartbeatTickMsg{})
	if cmd == nil || !m.heartbeatTicking {
		t.Fatal("idle-focused must throttle, not park")
	}
	if got, want := m.hbInterval, time.Second/8; got != want {
		t.Fatalf("throttled interval = %v, want %v", got, want)
	}
	// …and the tick AFTER that steps the phase by the full elapsed
	// interval — 125ms worth of drift in one step.
	before := m.skyPhase
	m, _ = driveCmd(m, HeartbeatTickMsg{})
	wantStep := skyPhaseStep * float64(time.Second/8) / float64(heartbeatInterval)
	if got := m.skyPhase - before; got < wantStep*0.999 || got > wantStep*1.001 {
		t.Fatalf("throttled phase step = %v, want ≈%v (scaled to the interval)", got, wantStep)
	}
	// A keystroke stamps the activity clock; the next tick re-plans to
	// the full 16ms rate.
	m = drive(m, key("x"))
	m, _ = driveCmd(m, HeartbeatTickMsg{})
	if got := m.hbInterval; got != heartbeatInterval {
		t.Fatalf("input must snap the heartbeat back to the full rate: got %v", got)
	}
}

// Deep idle parks the heartbeat entirely; input wakes it on the very
// next message. This is the no-focus-reporting terminal's whole idle
// path, so it must work without any Focus/Blur traffic.
func TestDeepIdleParksAndInputWakes(t *testing.T) {
	m, now := clockModel(t)
	*now = fixedNow.Add(10 * time.Minute)
	m, cmd := driveCmd(m, HeartbeatTickMsg{})
	if cmd != nil || m.heartbeatTicking {
		t.Fatal("deep idle must park the heartbeat")
	}
	frozen := m.skyPhase
	m, cmd = driveCmd(m, key("x"))
	if cmd == nil || !m.heartbeatTicking {
		t.Fatal("input must wake the parked heartbeat")
	}
	if m.skyPhase != frozen {
		t.Fatal("waking itself must not jump the phase")
	}
}

// The fast gate's @ai contract: pending holds it (same shape as
// unresolved thinking), done does NOT — the done bar is ambient-class.
// This is the exact leak that pinned a 60fps loop open for the process
// lifetime after one finished @ai reply.
func TestAiEventFastGatePendingVsDone(t *testing.T) {
	pending := api.Event{Kind: api.KindAi, Payload: []byte(`{"status":"pending"}`)}
	done := api.Event{Kind: api.KindAi, Payload: []byte(`{"status":"done","blocks":[{"type":"text","md":"hi"}]}`)}
	if !eventNeedsTicks(pending) {
		t.Error("a pending @ai event must hold the fast gate")
	}
	if eventNeedsAmbient(pending) {
		t.Error("a pending @ai event is fast-class, not ambient")
	}
	if eventNeedsTicks(done) {
		t.Error("a done @ai event must NOT hold the fast gate")
	}
	if !eventNeedsAmbient(done) {
		t.Error("a done @ai event's gradient bar must ride the ambient class")
	}
	// No status field at all degrades to pending — mirroring
	// render/ai.go's own tolerant dispatch.
	if !eventNeedsTicks(api.Event{Kind: api.KindAi, Payload: []byte(`{}`)}) {
		t.Error("a status-less @ai payload reads as pending, like the renderer")
	}
}

// The live migration: a pending @ai append holds the fast chain; its
// done replace releases the fast gate and moves the turn to the ambient
// heartbeat — the whole conversation can then idle at zero fast ticks.
func TestDoneAiReplaceMigratesTurnToAmbient(t *testing.T) {
	m, _ := clockModel(t, WithConversation("u-1"))
	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventAppend,
		Event: api.Event{ID: 50, TurnID: 20, Kind: api.KindAi, Payload: []byte(`{"status":"pending"}`)},
	}})
	if !m.shimmer[20] {
		t.Fatal("the pending @ai turn must hold the fast chain")
	}
	if m.ambient[20] {
		t.Fatal("the pending @ai turn must not be ambient yet")
	}
	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type:  cable.TypeEventReplace,
		Event: api.Event{ID: 50, TurnID: 20, Kind: api.KindAi, Payload: []byte(`{"status":"done","blocks":[{"type":"text","md":"answer"}]}`)},
	}})
	if m.shimmer[20] {
		t.Fatal("the done replace must release the fast gate")
	}
	if !m.ambient[20] {
		t.Fatal("the done @ai turn must migrate to the ambient class")
	}
	// Drain the delivery's dot pulse: the fast chain must then CLOSE —
	// before this change a done @ai reply held it open forever.
	m = driveAnim(t, m, 200)
	if m.animating {
		t.Fatal("nothing pending: the fast chain must close")
	}
	if !m.heartbeatTicking {
		t.Fatal("the ambient heartbeat must carry the done bar instead")
	}
}

// While the fast chain is open it owns the 16ms cadence — a heartbeat
// tick landing mid-flight parks instead of stacking a second chain (the
// "at most one 16ms chain" collapse).
func TestHeartbeatDefersToOpenFastChain(t *testing.T) {
	m, _ := clockModel(t)
	m.quitArmed = quitArmTicks // any fast-gate effect
	cmd := m.animate()
	if cmd == nil || !m.animating {
		t.Fatal("the fast chain must arm")
	}
	m, cmd = driveCmd(m, HeartbeatTickMsg{})
	if cmd != nil || m.heartbeatTicking {
		t.Fatal("the heartbeat must defer while the fast chain is open")
	}
	// The fast chain advances the sky itself at the same cadence.
	before := m.skyPhase
	m, _ = driveCmd(m, AnimTickMsg{})
	if got, want := m.skyPhase-before, skyPhaseStep; got != want {
		t.Fatalf("fast-chain sky step = %v, want %v", got, want)
	}
}
