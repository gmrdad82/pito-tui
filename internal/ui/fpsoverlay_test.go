package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// ── the ctrl+f9 toggle + its single-writer tick chain ────────────────────

// Turning the chip on must arm a fresh counter and schedule exactly one
// tick — startFPSTick's own guard (fpsoverlay.go) is what keeps a mashed
// ctrl+f9 from ever stacking more than one live chain, but the toggle
// itself must always kick the FIRST tick off.
func TestFPSToggleOnArmsCounterAndSchedulesTick(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithNewConversation())
	m = sized(m)
	if m.fpsOn || m.fps != nil {
		t.Fatalf("chip must start off: fpsOn=%v fps=%v", m.fpsOn, m.fps)
	}

	m, cmd := driveCmd(m, key("ctrl+f9"))
	if !m.fpsOn {
		t.Error("ctrl+f9 must turn the chip on")
	}
	if m.fps == nil {
		t.Fatal("turning on must arm a fresh counter")
	}
	if cmd == nil {
		t.Fatal("turning on must schedule the tick loop")
	}
	if _, ok := cmd().(FPSTickMsg); !ok {
		t.Errorf("scheduled cmd yielded %T, want FPSTickMsg", cmd())
	}
}

// Turning the chip back off drops the counter immediately (fps == nil,
// same as it never having run) and returns no command of its own — but
// the tick chain already in flight from the ON toggle keeps existing
// authority: onFPSTick, not the toggle, is the one place that clears
// fpsTicking. One more harmless tick lands after off, and THAT tick must
// not reschedule (the single-writer contract fpsoverlay.go documents).
func TestFPSToggleOffDropsCounterAndLateTickDoesNotReschedule(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithNewConversation())
	m = sized(m)

	m, cmd := driveCmd(m, key("ctrl+f9")) // on: schedules the first tick
	m, cmd = driveCmd(m, cmd())           // deliver it: still on, reschedules
	if cmd == nil || !m.fpsTicking {
		t.Fatal("the chain must be alive before we exercise the off transition")
	}

	m, cmd = driveCmd(m, key("ctrl+f9")) // off
	if m.fpsOn {
		t.Error("second ctrl+f9 must turn the chip back off")
	}
	if m.fps != nil {
		t.Error("turning off must drop the counter, not just stop counting")
	}
	if cmd != nil {
		t.Error("turning off returns no command of its own")
	}
	if !m.fpsTicking {
		t.Fatal("fpsTicking is cleared by onFPSTick alone, not by the toggle")
	}

	// The tick already scheduled by the ON toggle arrives late, after
	// off. It must die quietly: no reschedule, and fpsTicking finally
	// clears here.
	m, cmd = driveCmd(m, FPSTickMsg{})
	if cmd != nil {
		t.Error("a late tick after off must NOT reschedule the chain")
	}
	if m.fpsTicking {
		t.Error("a late tick after off must be the one to clear fpsTicking")
	}
}

// Plain f9 is deliberately left unbound (the window manager owns it
// globally for dictation and must never see the terminal steal it), so
// it must be a complete no-op against the chip — never armed, never a
// command scheduled. Built directly as tea.KeyPressMsg{Code: tea.KeyF9}
// rather than via the key() helper: key()'s fallback for unrecognized
// names stuffs the whole string into Text too, which makes it look like
// printable input and takes the chat-input insertion path instead of a
// genuine F9 press (a non-printable key, Text always empty).
func TestFPSPlainF9IsANoOp(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithNewConversation())
	m = sized(m)

	m, cmd := driveCmd(m, tea.KeyPressMsg{Code: tea.KeyF9})
	if m.fpsOn || m.fps != nil {
		t.Fatalf("plain f9 must not toggle the chip: fpsOn=%v fps=%v", m.fpsOn, m.fps)
	}
	if cmd != nil {
		t.Error("plain f9 must not schedule any command")
	}
}

// ── the sliding window itself ────────────────────────────────────────────

// fpsCounter.stamp takes `now` as a plain argument, so the fake clock is
// just handing it whatever time.Time the test wants — no model injection
// needed to drive this one directly. Stamps older than the trailing 1s
// must drop on every call, never averaged over a fixed period.
func TestFPSCounterSlidingWindowDropsStampsOlderThanOneSecond(t *testing.T) {
	base := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	c := &fpsCounter{}

	if got := c.stamp(base); got != 1 {
		t.Fatalf("first stamp = %d, want 1", got)
	}
	if got := c.stamp(base.Add(500 * time.Millisecond)); got != 2 {
		t.Fatalf("stamp at +500ms = %d, want 2", got)
	}
	if got := c.stamp(base.Add(999 * time.Millisecond)); got != 3 {
		t.Fatalf("stamp at +999ms (still under the 1s cutoff) = %d, want 3", got)
	}
	// +1001ms: exactly the FIRST stamp (base, now 1001ms old) has aged
	// out; the other two (500ms, 999ms old) are still inside the window.
	if got := c.stamp(base.Add(1001 * time.Millisecond)); got != 3 {
		t.Fatalf("stamp at +1001ms = %d, want 3 (oldest stamp dropped)", got)
	}
	// A jump far beyond 1s must drop every prior stamp, leaving only
	// the new one — a true sliding window, not a running total.
	if got := c.stamp(base.Add(5 * time.Second)); got != 1 {
		t.Fatalf("stamp after a 5s jump = %d, want 1 (window fully cleared)", got)
	}
}

// Same window, exercised through the model's own now-injection idiom
// (WithNow, the fixedNow harness's mechanism — see model_test.go) rather
// than calling fpsCounter directly: each real viewContent() repaint
// stamps at whatever m.now() currently returns, so advancing a mutable
// closure between repaints must show the chip's count rise while stamps
// stay within 1s of each other, then fall once the oldest ones age out.
func TestFPSChipCountThroughRepaintsFollowsSlidingWindow(t *testing.T) {
	now := fixedNow
	m, _ := newTestModel(t, chatServer(t), WithNewConversation(), WithNow(func() time.Time { return now }))
	m = sized(m)
	m.fpsOn = true
	m.fps = &fpsCounter{}

	_ = m.viewContent() // stamp @ +0ms
	now = now.Add(400 * time.Millisecond)
	_ = m.viewContent() // stamp @ +400ms
	now = now.Add(400 * time.Millisecond)
	view := m.viewContent() // stamp @ +800ms — all three still within 1s of +800ms
	if line := strings.SplitN(view, "\n", 2)[0]; !strings.Contains(line, "3 fps") {
		t.Fatalf("first line after 3 repaints inside 1s = %q, want to contain \"3 fps\"", line)
	}

	// Advance to +1500ms: the +0ms and +400ms stamps are now over 1s
	// old and must drop, leaving the +800ms stamp plus this new one.
	now = now.Add(700 * time.Millisecond)
	view = m.viewContent()
	if line := strings.SplitN(view, "\n", 2)[0]; !strings.Contains(line, "2 fps") {
		t.Fatalf("first line once the oldest stamps age out = %q, want to contain \"2 fps\"", line)
	}
}

// ── the chip's own paint guard ────────────────────────────────────────────

// The chip must be entirely absent while off and, once on, must land on
// the viewport's very FIRST line (paintFPSOverlay's flush-left contract)
// — never anywhere else in the frame.
func TestFPSChipPaintsTopLeftOnlyWhileOn(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithNewConversation())
	m = sized(m)

	if view := m.viewContent(); strings.Contains(view, "fps") {
		t.Fatalf("chip must be absent while off:\n%s", view)
	}

	m.fpsOn = true
	m.fps = &fpsCounter{frames: []time.Time{fixedNow}} // one prior stamp
	view := m.viewContent()                            // this repaint adds a second stamp
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[0], "2 fps") {
		t.Errorf("first line = %q, want to contain \"2 fps\"", lines[0])
	}
	for i, line := range lines[1:] {
		if strings.Contains(line, "fps") {
			t.Errorf("line %d also carries the chip, want it flush on line 0 only: %q", i+1, line)
		}
	}
}

// TestGoldenFPSOverlay pins the chip's frame: on, one stamp already
// queued so the repaint this golden captures lands at a deterministic
// count. Regenerate with `go test ./internal/ui/ -run TestGoldenFPSOverlay
// -update` — the house goldenFrame mechanism (golden_test.go).
func TestGoldenFPSOverlay(t *testing.T) {
	m, _ := newTestModel(t, chatServer(t), WithNewConversation())
	m = sized(m)
	m.fpsOn = true
	m.fps = &fpsCounter{}
	goldenFrame(t, m)
}
