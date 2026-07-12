package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// The ambassador wave's closing artifact: `pito-tui --tour`, a fully
// scripted, zero-interaction walkthrough — the thing shown to outsiders
// (including Charm). It rides the SAME tick/msg loop every other effect
// in this package already drives (Model.onAnimTick, the shared 40ms
// house cadence), never a parallel timer or a parallel send path:
//
//   - typing types the step's command into the REAL input, tick by tick
//     (stepTourTyping) — the ghost-hint type-in (micro.go effect 3) run
//     in reverse, into the input itself rather than a suggestion preview,
//     so the audience can read the command as it's typed.
//   - submitting fires a synthetic Enter KeyPressMsg through the model's
//     own Update — the exact path a real keystroke takes (onChatKey's
//     "enter" case), so the send/notifications-intercept/palette-dismiss
//     logic never needs a second implementation.
//   - the caption card (tourCaptionView) sits directly above the input,
//     brand-gradient text on a dim rule — the keymap footer's own quiet
//     chrome language (chrome.go), applied to a single centered banner.
//
// Esc/ctrl+c abort the script into normal interactive use at any point
// (Model.onKey's own tour-abort branch) — a live demo must never be one
// slipped keystroke from vanishing.

// tourCharInterval is the average time between typed characters — the
// brief's own "~30ms/char". Tracked as a time.Duration accumulator
// against tourTickDelta's measured per-tick time rather than a dedicated
// ticker: on a healthy 40ms cadence most ticks emit one rune and every
// third or so emits two, averaging out to exactly 30ms/char — see
// stepTourTyping.
const tourCharInterval = 30 * time.Millisecond

// tourMaxTickDelta caps how much real time a single tick may bank
// (tourTickDelta). Ticks CAN legitimately arrive far apart — a slow
// terminal back-pressuring the render loop (live-measured: vhs's
// ttyd/Chrome capture stretches the 40ms cadence past 200ms once several
// shimmer-marked turns are all repainting every tick), or the process
// getting stopped and resumed — and without a cap one pathological gap
// would swallow an entire step's dwell in a single tick, skipping right
// past whatever the audience was supposed to be watching.
const tourMaxTickDelta = 500 * time.Millisecond

// tourClosingHold is how long the closing caption stays up once the
// script is exhausted, before the tour hands control back — the app
// underneath is already fully interactive the entire time.
const tourClosingHold = 3 * time.Second

// tourClosingCaption is the sign-off line shown for tourClosingHold.
const tourClosingCaption = "pito — the terminal companion"

// tourStep is one entry of --tour's compiled-in script: a caption shown
// on the caption card while it plays, the command typed into the real
// input and submitted through the ordinary Enter path, and how long the
// step holds once submitted — long enough for the reply to arrive,
// render, and settle before the next command starts typing over it.
type tourStep struct {
	caption string
	command string
	dwell   time.Duration
	// escAfter, when nonzero, fires a synthetic Esc partway through the
	// dwell — the /notifications step's "then esc": long enough for the
	// panel's own open spring to finish traveling on camera before it
	// closes again. Zero means no auto-esc; the step just dwells and
	// advances, like every step but /notifications.
	escAfter time.Duration
}

// TourScript builds --tour's walkthrough: four steps against a
// brand-new conversation, an optional @ai step gated behind aiEnabled
// (run.go's --tour-ai — it spends the owner's own AI provider budget,
// so it never rides the default script), then /notifications (with its
// own timed esc) and help.
func TourScript(aiEnabled bool) []tourStep {
	script := []tourStep{
		{caption: "a quick tour of pito", command: "list games", dwell: 6 * time.Second},
		{caption: "drill into one game", command: "show game 12", dwell: 8 * time.Second},
		{caption: "a channel, at a glance", command: "at-a-glance channel @gmrdad82hard", dwell: 8 * time.Second},
		{caption: "the deeper breakdowns", command: "breakdowns channel @gmrdad82", dwell: 12 * time.Second},
	}
	if aiEnabled {
		script = append(script, tourStep{
			caption: "and the ai, when you want it",
			command: "@ai what should I play next?",
			dwell:   15 * time.Second,
		})
	}
	return append(script,
		tourStep{caption: "notifications, live", command: "/notifications", dwell: 6 * time.Second, escAfter: 3 * time.Second},
		tourStep{caption: "everything the grammar knows", command: "help", dwell: 6 * time.Second},
	)
}

// tourPhase is the tour's own tiny state machine, per step.
type tourPhase int

const (
	tourTyping     tourPhase = iota // appending the command onto m.input
	tourSubmitting                  // fully typed; holding on screen before Enter fires
	tourDwelling                    // submitted; waiting out the step's dwell (and its optional esc)
	tourClosing                     // last step done; holding the sign-off caption
)

// tourSubmitHold is the beat the fully-typed command holds on screen
// before Enter fires — without it, the last keystroke and the submit
// would land in the SAME tick, and the audience (this being a live demo)
// would never actually see the finished command before it vanished into
// the transcript.
const tourSubmitHold = 250 * time.Millisecond

// tourState is --tour's entire state (Model.tour): which step, which
// phase within it, and enough per-phase counters to know when to move
// on. A zero value (empty script) means "no tour" everywhere this is
// read — see tourActive.
type tourState struct {
	script []tourStep
	step   int
	phase  tourPhase
	active bool

	typed    int           // runes of script[step].command already applied to m.input
	elapsed  time.Duration // time banked in the current phase
	escFired bool          // this step's escAfter has already fired
	lastTick time.Time     // previous stepTour's clock reading — tourTickDelta's anchor

	caption string // the caption card's current text — tourCaptionView
}

// tourActive reports whether --tour is still driving. false either
// because --tour was never passed (empty script — WithTour's own no-op
// guard) or because the script finished or was aborted (Esc/ctrl+c,
// Model.onKey) — either way the model behaves exactly as if WithTour
// had never been applied: no caption, no extra tick-gate pressure, no
// key interception.
func (m Model) tourActive() bool {
	return len(m.tour.script) > 0 && m.tour.active
}

// stepTour advances the tour exactly one animation tick — a no-op while
// the startup splash is still on screen (the script never starts typing
// underneath it) and a no-op once the tour has ended. Any command it
// returns rides the SAME batch Model.onAnimTick already returns; without
// that, the real send/fetch it triggers would never actually run.
func (m *Model) stepTour() tea.Cmd {
	if !m.tourActive() || m.splashActive() {
		return nil
	}
	delta := m.tourTickDelta()
	switch m.tour.phase {
	case tourTyping:
		return m.stepTourTyping(delta)
	case tourSubmitting:
		return m.stepTourSubmitting(delta)
	case tourDwelling:
		return m.stepTourDwelling(delta)
	case tourClosing:
		m.stepTourClosing(delta)
	}
	return nil
}

// tourTickDelta measures the REAL time this tick gets to bank, clamped
// to [shimmerTick, tourMaxTickDelta]. tea.Tick schedules the NEXT tick
// only after the previous Update+View completes, so under a slow
// terminal (vhs's ttyd/Chrome capture, an ssh session, a loaded box) the
// nominal 40ms cadence stretches — live-measured past 200ms once several
// shimmer-marked turns repaint every tick. Banking a fixed shimmerTick
// per tick (the counter convention every OTHER windowed effect here
// uses) would slow the whole script down by that same factor — a ~55s
// tour was observed still mid-script after 220s inside vhs. Real deltas
// keep the tour wall-clock-accurate everywhere; the min-clamp keeps it
// exactly at the old nominal pace on a fast terminal AND under the test
// harness's pinned WithNow clock (delta 0 → clamped up to shimmerTick,
// so tests stay tick-deterministic); the max-clamp stops one
// pathological stall from swallowing a whole dwell. The other effects
// keep their plain counters on purpose: for a decorative 600ms ripple,
// stretching under load is fine — only the tour has a script to keep.
func (m *Model) tourTickDelta() time.Duration {
	now := m.now()
	last := m.tour.lastTick
	m.tour.lastTick = now
	if last.IsZero() {
		return shimmerTick
	}
	d := now.Sub(last)
	if d < shimmerTick {
		return shimmerTick
	}
	if d > tourMaxTickDelta {
		return tourMaxTickDelta
	}
	return d
}

// stepTourTyping banks this tick's real time and appends however many
// characters that time affords (one or two on a healthy 40ms cadence,
// more when a slow terminal made the tick late — the input simply
// catches up) directly onto m.input, rather than synthesizing a
// KeyPressMsg per rune — which would also trip onChatKey's empty-input
// single-letter bindings ('?', 'R', 'j', 'k', 'g', 'G') the moment a
// script command happened to start with one of them. Once the command is
// fully typed it hands off to stepTourSubmitting rather than firing
// Enter in this same tick — the audience needs at least a beat to
// actually read the finished command before it's gone.
func (m *Model) stepTourTyping(delta time.Duration) tea.Cmd {
	step := m.tour.script[m.tour.step]
	command := []rune(step.command)
	m.tour.elapsed += delta
	changed := false
	for m.tour.elapsed >= tourCharInterval && m.tour.typed < len(command) {
		m.tour.elapsed -= tourCharInterval
		m.tour.typed++
		changed = true
	}
	if changed {
		m.input.SetValue(string(command[:m.tour.typed]))
		m.input.CursorEnd()
	}
	if m.tour.typed < len(command) {
		return nil
	}
	m.tour.phase = tourSubmitting
	m.tour.elapsed = 0
	return nil
}

// stepTourSubmitting holds the fully-typed command on screen for
// tourSubmitHold, then submits it FOR REAL: a synthetic Enter
// KeyPressMsg through the model's own Update — the exact path a real
// keystroke takes (onChatKey's "enter" case, including its
// /notifications intercept) — which is the whole reason there is no
// second, parallel call into sendCmd anywhere in this file.
func (m *Model) stepTourSubmitting(delta time.Duration) tea.Cmd {
	m.tour.elapsed += delta
	if m.tour.elapsed < tourSubmitHold {
		return nil
	}
	m.tour.phase = tourDwelling
	m.tour.elapsed = 0
	m.tour.escFired = false
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	*m = next.(Model)
	return cmd
}

// stepTourDwelling banks time toward the current step's dwell, firing
// the /notifications step's own synthetic Esc partway through
// (escAfter) — dispatched straight into onNotificationsKey rather than
// through onKey/Update: onKey's own tour-abort branch treats ANY esc as
// "stop the tour," and would swallow this one instead of closing the
// panel. Once dwell elapses it advances to the next step, or — past the
// last one — into the closing caption.
func (m *Model) stepTourDwelling(delta time.Duration) tea.Cmd {
	step := m.tour.script[m.tour.step]
	m.tour.elapsed += delta
	var cmd tea.Cmd
	if step.escAfter > 0 && !m.tour.escFired && m.tour.elapsed >= step.escAfter {
		m.tour.escFired = true
		if m.mode == modeNotifications {
			next, c := m.onNotificationsKey(tea.KeyPressMsg{Code: tea.KeyEscape})
			*m = next.(Model)
			cmd = c
		}
	}
	if m.tour.elapsed < step.dwell {
		return cmd
	}
	m.tour.step++
	if m.tour.step >= len(m.tour.script) {
		m.tour.phase = tourClosing
		m.tour.elapsed = 0
		m.tour.caption = tourClosingCaption
		return cmd
	}
	m.tour.phase = tourTyping
	m.tour.typed = 0
	m.tour.elapsed = 0
	m.tour.caption = m.tour.script[m.tour.step].caption
	m.input.Reset()
	m.suggest = nil
	return cmd
}

// stepTourClosing holds the sign-off caption for tourClosingHold, then
// ends the tour — active flips false and the caption clears, leaving
// the model in exactly the state normal interactive use starts from.
func (m *Model) stepTourClosing(delta time.Duration) {
	m.tour.elapsed += delta
	if m.tour.elapsed >= tourClosingHold {
		m.tour.active = false
		m.tour.caption = ""
	}
}

// tourCaptionView renders the tour's caption card: one centered,
// brand-gradient line sitting on a dim rule — the keymap footer's own
// quiet chrome language (chrome.go), applied to a single banner instead
// of a strip of chips. Off-truecolor still shows the caption (the
// mechanics must survive a plain terminal, only the effects are
// gated) — plain dim-bold text instead of the gradient. Contained to
// contentWidth(), same as every other piece of conversation chrome.
func (m Model) tourCaptionView() string {
	if !m.tourActive() || m.tour.caption == "" {
		return ""
	}
	width := m.contentWidth()
	text := m.tour.caption
	if lipgloss.Width(text) > width {
		text = truncateEllipsis(text, width)
	}
	painted := lipgloss.NewStyle().Foreground(render.ColorDim).Bold(true).Render(text)
	if m.truecolor {
		painted = render.PitoShimmer.Colorize(text, m.phase)
	}
	pad := (width - lipgloss.Width(text)) / 2
	if pad < 0 {
		pad = 0
	}
	rule := lipgloss.NewStyle().Foreground(render.ColorFaint).Render(strings.Repeat("─", width))
	return strings.Repeat(" ", pad) + painted + "\n" + rule
}
