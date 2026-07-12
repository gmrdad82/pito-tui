package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// tourServer fakes just enough of the API surface for --tour's script to
// run against: POST /chat creates a conversation the first time (no uuid
// in the body) and acks every later send on it, GET /chat/tour-1.json
// always answers with an empty scrollback (so onChatFetched's fetch/
// connect leg completes cleanly), and GET /notifications.json answers an
// empty page — the /notifications step's own fetch. postCount lets a
// test assert the real send path actually fired.
func tourServer(t *testing.T) (http.Handler, *int32) {
	t.Helper()
	var turn int64
	var posts int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&posts, 1)
		var body struct {
			UUID string `json:"uuid"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		turn++
		w.Header().Set("Content-Type", "application/json")
		if body.UUID == "" {
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"uuid":"tour-1","turn_id":%d}`, turn)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, `{"turn_id":%d}`, turn)
	})
	mux.HandleFunc("GET /chat/tour-1.json", jsonHandler(`{"conversation":{"uuid":"tour-1","title":"tour"},"events":[]}`))
	mux.HandleFunc("GET /notifications.json", jsonHandler(`{"rows":[],"next_cursor":null}`))
	return mux, &posts
}

// driveTourTick advances --tour by exactly one simulated 40ms house tick
// — Model.onAnimTick's own stepTour slice, called DIRECTLY
// rather than through Update(AnimTickMsg{}). Going through the full
// production message would return the SAME recurring animTick() command
// every tick bundled in the batch, and bubbletea's tea.Tick genuinely
// blocks (real time.Sleep) once actually invoked — draining that batch
// honestly, the way a many-hundred-tick test loop needs to, would cost
// real wall-clock seconds for no reason. Calling stepTour
// straight is exactly what onAnimTick does for these two effects, minus
// the phase/shimmer/splash bookkeeping this file's tests don't touch.
func driveTourTick(m Model) Model {
	tourCmd := m.stepTour()
	m = tourDrain(m, tourCmd)
	return m
}

// tourDrain runs cmd for real and feeds whatever message it produces
// back into Update, recursing into the result — deep enough to resolve a
// multi-hop chain like SendResultMsg → fetchChatCmd → ChatFetchedMsg,
// which model_test.go's own runCmd (one level only) doesn't reach. It
// deliberately stops — does not recurse further — at AnimTickMsg or
// spinner.TickMsg: both are the head of an otherwise-unbounded
// self-rescheduling chain (the pending spinner in particular reschedules
// itself for as long as m.pending is non-empty, which in these synthetic
// tests, with no cable ever delivering the turn's events, is forever).
// This file's assertions are all about the TOUR's own state, never about
// the shimmer phase or the pending spinner settling, so dropping both is
// safe.
func tourDrain(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, inner := range msg {
			m = tourDrain(m, inner)
		}
		return m
	case AnimTickMsg, spinner.TickMsg:
		return m
	default:
		next, nextCmd := m.Update(msg)
		return tourDrain(next.(Model), nextCmd)
	}
}

// ── WithTour / TourScript ────────────────────────────────────────────────

func TestWithTourArmsNewConversationAndFirstCaption(t *testing.T) {
	script := []tourStep{{caption: "step one", command: "help", dwell: time.Second}}
	m, _ := newTestModel(t, http.NotFoundHandler(), WithTour(script))
	if m.mode != modeChat || m.conv.UUID != "" {
		t.Fatalf("WithTour must open a blank-uuid chat like WithNewConversation, got mode=%v uuid=%q", m.mode, m.conv.UUID)
	}
	if !m.tourActive() {
		t.Fatal("tour must be active immediately after WithTour")
	}
	if m.tour.caption != "step one" {
		t.Errorf("caption = %q, want the first step's own caption", m.tour.caption)
	}
}

func TestWithTourEmptyScriptIsNoOp(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithTour(nil))
	if m.mode != modePicker {
		t.Fatalf("WithTour(nil) must leave the model exactly as NewModel built it, mode = %v", m.mode)
	}
	if m.tourActive() {
		t.Fatal("an empty script must never report active")
	}
}

func TestTourScriptAIGating(t *testing.T) {
	base := TourScript(false)
	withAI := TourScript(true)
	if len(withAI) != len(base)+1 {
		t.Fatalf("TourScript(true) should carry exactly one extra step, got %d vs %d", len(withAI), len(base))
	}
	for _, step := range base {
		if strings.Contains(step.command, "@ai") {
			t.Errorf("TourScript(false) must never include an @ai step, found %q", step.command)
		}
	}
	found := false
	for _, step := range withAI {
		if strings.Contains(step.command, "@ai") {
			found = true
		}
	}
	if !found {
		t.Error("TourScript(true) must include an @ai step")
	}
	// The notifications step (with its timed esc) and the closing help
	// step must always be last, in that order, regardless of --tour-ai.
	last := base[len(base)-2]
	if !strings.EqualFold(last.command, "/notifications") || last.escAfter <= 0 {
		t.Errorf("second-to-last base step = %+v, want /notifications with escAfter set", last)
	}
	if base[len(base)-1].command != "help" {
		t.Errorf("last base step = %+v, want help", base[len(base)-1])
	}
	lastAI := withAI[len(withAI)-2]
	if !strings.EqualFold(lastAI.command, "/notifications") {
		t.Errorf("/notifications must still be second-to-last with --tour-ai, got %+v", lastAI)
	}
}

// ── Typing ──────────────────────────────────────────────────────────────

func TestTourTypesExactlyTheCommandCharByChar(t *testing.T) {
	srv, posts := tourServer(t)
	script := []tourStep{{caption: "cap", command: "list games", dwell: 50 * time.Millisecond}}
	m, _ := newTestModel(t, srv, WithTour(script))
	m = sized(m)

	// A few ticks in, the input must already hold a proper, still-growing
	// prefix of the command — never anything else, never the whole thing
	// yet (10 chars at ~30ms/char takes more than 3 ticks of 40ms each).
	for i := 0; i < 3; i++ {
		m = driveTourTick(m)
	}
	v := m.input.Value()
	if v == "" || v == "list games" || !strings.HasPrefix("list games", v) {
		t.Fatalf("after 3 ticks, input = %q, want a non-empty, non-final prefix of \"list games\"", v)
	}

	// Enough ticks to finish typing, hold (tourSubmitHold), and submit:
	// the peak value reached (right before onChatKey's enter case resets
	// the input) must be the command byte-for-byte.
	var peak string
	for i := 0; i < 40 && m.input.Value() != ""; i++ {
		if len(m.input.Value()) > len(peak) {
			peak = m.input.Value()
		}
		m = driveTourTick(m)
	}
	if peak != "list games" {
		t.Fatalf("peak typed value = %q, want the exact command %q", peak, "list games")
	}
	if m.input.Value() != "" {
		t.Fatal("submit must reset the input, exactly like a real Enter keypress")
	}
	if atomic.LoadInt32(posts) != 1 {
		t.Fatalf("submit must hit POST /chat exactly once via the real send path, got %d", atomic.LoadInt32(posts))
	}
	if m.conv.UUID != "tour-1" {
		t.Fatalf("the real send path must adopt the server's created uuid, got %q", m.conv.UUID)
	}
}

// ── Submit / dwell / step advance ──────────────────────────────────────

func TestTourAdvancesCaptionsAndEndsWithClosingCard(t *testing.T) {
	srv, _ := tourServer(t)
	script := []tourStep{
		{caption: "first", command: "help", dwell: 40 * time.Millisecond},
		{caption: "second", command: "help", dwell: 40 * time.Millisecond},
	}
	m, _ := newTestModel(t, srv, WithTour(script))
	m = sized(m)

	if m.tour.caption != "first" {
		t.Fatalf("caption = %q, want %q", m.tour.caption, "first")
	}

	// Drive well past both steps' type+dwell budgets and the closing
	// hold — the tour must reach "second" along the way and end inactive.
	seenSecond := false
	for i := 0; i < 500 && m.tourActive(); i++ {
		if m.tour.caption == "second" {
			seenSecond = true
		}
		m = driveTourTick(m)
	}
	if !seenSecond {
		t.Error("tour never advanced to the second step's caption")
	}
	if m.tourActive() {
		t.Fatal("tour should have ended within the tick budget")
	}
	if m.tour.caption != "" {
		t.Errorf("caption must clear once the tour ends, got %q", m.tour.caption)
	}
}

// ── /notifications step: intercept + timed esc ──────────────────────────

func TestTourNotificationsStepOpensThenAutoCloses(t *testing.T) {
	srv, _ := tourServer(t)
	script := []tourStep{
		// escAfter must leave the overlay's ~240ms close spring (see
		// spring.go's overlaySpringPhysics) comfortably inside dwell, or
		// the step would advance to typing "help" while the panel is
		// still on screen (mirroring the production script's own
		// generous 3s-of-6s margin, just compressed).
		{caption: "panel", command: "/notifications", dwell: 600 * time.Millisecond, escAfter: 120 * time.Millisecond},
		{caption: "after", command: "help", dwell: 40 * time.Millisecond},
	}
	m, _ := newTestModel(t, srv, WithTour(script))
	m = sized(m)

	// Type + submit "/notifications" — the client-side intercept
	// (onChatKey) must open the panel, never round-trip to POST /chat.
	opened := false
	for i := 0; i < 120 && !opened; i++ { // 16ms ticks now — same wall-clock budget as the old 40×40ms
		m = driveTourTick(m)
		if m.mode == modeNotifications {
			opened = true
		}
	}
	if !opened {
		t.Fatal("the /notifications step never opened the panel")
	}

	// Enough further ticks for escAfter to fire and the closing spring to
	// settle back into chat, then advance into the next step.
	closedBackToChat := false
	for i := 0; i < 300 && m.tourActive(); i++ {
		m = driveTourTick(m)
		if m.mode == modeChat && m.tour.caption == "after" {
			closedBackToChat = true
			break
		}
	}
	if !closedBackToChat {
		t.Fatal("the /notifications step never auto-closed back into chat and advanced")
	}
}

// ── Esc/ctrl+c abort ─────────────────────────────────────────────────────

func TestTourEscAbortsIntoInteractiveMode(t *testing.T) {
	srv, posts := tourServer(t)
	script := []tourStep{{caption: "cap", command: "list games", dwell: time.Second}}
	m, _ := newTestModel(t, srv, WithTour(script))
	m = sized(m)
	m = driveTourTick(m) // a couple characters typed in

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if cmd != nil {
		t.Error("aborting the tour should not itself produce a command")
	}
	if m.tourActive() {
		t.Fatal("esc must abort the tour")
	}
	if m.mode != modeChat {
		t.Fatalf("aborting must drop into normal chat mode, got %v", m.mode)
	}

	// Further ticks must be complete no-ops for the tour — typing must
	// not resume, and no further sends must fire.
	before := m.input.Value()
	for i := 0; i < 10; i++ {
		m = driveTourTick(m)
	}
	if m.input.Value() != before {
		t.Errorf("input changed after abort: %q -> %q", before, m.input.Value())
	}
	if atomic.LoadInt32(posts) != 0 {
		t.Error("an aborted tour must never have reached the real send path")
	}

	// Now normal interactive typing must work exactly as it always does.
	m.input.SetValue("hello")
	if v := m.input.Value(); v != "hello" {
		t.Errorf("normal typing after abort = %q, want %q", v, "hello")
	}
}

func TestTourCtrlCAbortsRatherThanQuitting(t *testing.T) {
	srv, _ := tourServer(t)
	script := []tourStep{{caption: "cap", command: "help", dwell: time.Second}}
	m, _ := newTestModel(t, srv, WithTour(script))
	m = sized(m)

	// ctrl+c is reported by its own String(); synthesize it the same way
	// the rest of this package's tests do for named combos.
	msg := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	if msg.String() != "ctrl+c" {
		t.Fatalf("test setup: synthesized key stringifies to %q, want ctrl+c", msg.String())
	}
	next, cmd := m.Update(msg)
	m = next.(Model)
	if cmd != nil {
		t.Fatal("ctrl+c during an active tour must abort, not tea.Quit (which would return a non-nil command)")
	}
	if m.tourActive() {
		t.Fatal("ctrl+c must abort the tour")
	}
}

func TestTourAbortDoesNotSwallowRealNotificationsEsc(t *testing.T) {
	// Once the tour is over, esc must go back to meaning exactly what it
	// always did — closing a real, user-opened notifications panel.
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	opened, _ := m.openNotifications()
	m = opened.(Model)
	if m.mode != modeNotifications {
		t.Fatal("test setup: openNotifications must land in modeNotifications")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	// Drawer springs purged 2026-07-12: the close lands immediately.
	if m.mode != modeChat {
		t.Fatal("esc on a real (non-tour) notifications panel must close it")
	}
}

// ── Caption card ──────────────────────────────────────────────────────────

func TestTourCaptionRendersContainedAndCentered(t *testing.T) {
	script := []tourStep{{caption: "a quick tour of pito", command: "help", dwell: time.Second}}
	m, _ := newTestModel(t, http.NotFoundHandler(), WithTour(script), WithTruecolor(true))
	m = drive(m, tea.WindowSizeMsg{Width: 240, Height: 40}) // wide enough to hit the containment cap

	view := m.tourCaptionView()
	if view == "" {
		t.Fatal("caption must render while the tour is active")
	}
	lines := strings.Split(view, "\n")
	if len(lines) != 2 {
		t.Fatalf("caption card must be exactly two lines (text + rule), got %d", len(lines))
	}
	width := m.contentWidth()
	for i, line := range lines {
		if w := lipgloss.Width(line); w > width {
			t.Errorf("caption line %d width %d exceeds contentWidth() %d — must stay contained", i, w, width)
		}
	}
	if !strings.Contains(stripANSIForTest(lines[0]), "a quick tour of pito") {
		t.Errorf("caption text missing from rendered line: %q", lines[0])
	}
}

func TestTourCaptionSurvivesOffTruecolor(t *testing.T) {
	// The mechanics (typing/submit/timing/caption text) must work on a
	// plain terminal — only the gradient paint is truecolor-gated.
	script := []tourStep{{caption: "plain terminal caption", command: "help", dwell: time.Second}}
	m, _ := newTestModel(t, http.NotFoundHandler(), WithTour(script))
	m = sized(m) // WithTruecolor NOT set — plain terminal
	view := m.tourCaptionView()
	if !strings.Contains(view, "plain terminal caption") {
		t.Errorf("caption must still render off-truecolor, got %q", view)
	}
}

func TestTourCaptionEmptyOutsideTour(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"), WithTruecolor(true))
	m = sized(m)
	if got := m.tourCaptionView(); got != "" {
		t.Errorf("caption must be empty with no tour armed, got %q", got)
	}
}

// stripANSIForTest removes SGR escapes for a plain substring check —
// mirrors golden_test.go's own ANSI-stripping approach for this package.
func stripANSIForTest(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEscape = true
		case inEscape && r == 'm':
			inEscape = false
		case !inEscape:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ── Animation gate ──────────────────────────────────────────────────────

func TestTourHoldsTheAnimationGateOpenAlone(t *testing.T) {
	script := []tourStep{{caption: "cap", command: "help", dwell: time.Second}}
	m, _ := newTestModel(t, http.NotFoundHandler(), WithTour(script)) // no truecolor, nothing else animating
	m = sized(m)
	if !m.animGateOpen() {
		t.Fatal("an active tour must hold the animation gate open on its own")
	}
}

// ── Wall-clock accuracy (tourTickDelta) ─────────────────────────────────

func TestTourBanksRealTimePerTickOnSlowTerminals(t *testing.T) {
	// A slow terminal stretches the tick cadence (tea.Tick only schedules
	// the next tick after Update+View complete) — the tour must bank the
	// REAL time between ticks, not a fixed 40ms, or the whole script runs
	// slow by exactly that stretch factor (live-measured under vhs).
	srv, _ := tourServer(t)
	script := []tourStep{{caption: "cap", command: "help", dwell: 2 * time.Second}}
	m, _ := newTestModel(t, srv, WithTour(script))
	m = sized(m)

	// A clock that jumps 200ms per stepTour call — a 5x-stretched cadence.
	clock := fixedNow
	m.now = func() time.Time {
		clock = clock.Add(200 * time.Millisecond)
		return clock
	}

	// Nominal budget: typing 4×30ms + submit hold 250ms + dwell 2s +
	// closing hold 3s ≈ 5.5s. At 200ms banked per tick that is ~28 ticks;
	// with fixed-40ms banking it would take ~135. Assert it finishes well
	// under the fixed-banking count.
	for i := 0; i < 60 && m.tourActive(); i++ {
		m = driveTourTick(m)
	}
	if m.tourActive() {
		t.Fatal("tour must finish on wall-clock time under a slow tick cadence — fixed per-tick banking would still be mid-script here")
	}
}

func TestTourTickDeltaClampsBothWays(t *testing.T) {
	script := []tourStep{{caption: "cap", command: "help", dwell: time.Second}}
	m, _ := newTestModel(t, http.NotFoundHandler(), WithTour(script))
	m = sized(m)

	// First call has no anchor — exactly one nominal tick.
	if got := m.tourTickDelta(); got != shimmerTick {
		t.Errorf("first delta = %v, want shimmerTick %v", got, shimmerTick)
	}
	// The harness's pinned clock never advances: min-clamp keeps every
	// later delta at the nominal tick too — the property that keeps all
	// the other tour tests tick-deterministic.
	if got := m.tourTickDelta(); got != shimmerTick {
		t.Errorf("pinned-clock delta = %v, want shimmerTick %v", got, shimmerTick)
	}
	// A pathological stall (system sleep, minutes-long pause) banks at
	// most tourMaxTickDelta — one gap must never swallow a whole dwell.
	clock := fixedNow
	m.now = func() time.Time { return clock }
	m.tour.lastTick = fixedNow
	clock = fixedNow.Add(time.Hour)
	if got := m.tourTickDelta(); got != tourMaxTickDelta {
		t.Errorf("stalled delta = %v, want the tourMaxTickDelta cap %v", got, tourMaxTickDelta)
	}
}
