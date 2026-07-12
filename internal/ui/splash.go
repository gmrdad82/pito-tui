package ui

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/ui/render"
	"github.com/gmrdad82/pito-tui/internal/version"
)

// The startup splash (chrome.go effect 1, split into its own file — the
// hand-crafted pixel font and the rise-away physics earned the room). A
// centered PITO wordmark, gradient-swept once by the brand ramp, holds for
// splashHoldTicks then springs away upward; ANY key skips it instantly
// (Model.onKey). Pure chrome over the loading beat: Init() still fires the
// resume/chat fetch unconditionally and in parallel (Model.Init, unchanged
// by this file) — the splash never gates or delays it, it just paints OVER
// whatever's loading underneath until it's done with itself.

// splashHoldTicks: ~800ms of ticks (40ms, the house's shimmerTick) the
// wordmark holds fully lit before rising away — an exact division, 20
// ticks.
const splashHoldTicks = int64(800 * time.Millisecond / shimmerTick)

// splashState is the splash's own tiny state machine: active while
// anything about it should still paint or tick, holdTicks counting up to
// splashHoldTicks, then pos/vel riding overlaySpringPhysics (spring.go)
// toward 1 — the SAME drawer physics the picker/notifications overlays
// open with, run in the opposite visual direction (see splashRise: rising
// pos drops lines off the TOP instead of padding them in from the bottom,
// so the wordmark visibly exits through the ceiling). No changes to
// spring.go were needed: overlaySpringPhysics is already a package-level
// value and springSettled already takes a bare (pos, vel, target) — both
// reusable as-is from any file in this package.
type splashState struct {
	active    bool
	holdTicks int64
	pos, vel  float64
	// tip is this boot's start-screen tip, chosen once at arm time from
	// pito's own pool (COPY LAW: pito.copy.start_screen.tips via
	// copygen; empty pool → no tip line).
	tip string
}

// maybeStartSplash arms the splash the FIRST time the terminal becomes
// ready — Model.onResize's own first-ready moment. Never on a later
// resize: firstReady is computed by the caller
// BEFORE m.ready flips true, so a terminal resize mid-conversation never
// re-arms it. Init() cannot arm this itself: Bubble Tea v2's Init() is a
// value-receiver method that returns only a Cmd, so any Model field it
// set would never reach the program's own stored state — onResize (a
// value-receiver method whose returned Model IS persisted by Update) is
// the earliest seam that actually sticks, and also the earliest point a
// terminal width/height even exists to center the wordmark inside.
func (m Model) maybeStartSplash() Model {
	if !splashEnabled || !m.splashOn || !m.truecolor {
		return m
	}
	m.splash.active = true
	m.splash.tip = render.StartScreenTip(int(m.now().UnixNano() / 1e6))
	return m
}

// splashActive reports whether the splash should still paint THIS frame —
// viewContent's own early-return gate.
func (m Model) splashActive() bool {
	return splashEnabled && m.truecolor && m.splash.active
}

// stepSplash advances the hold counter, then the rise-away spring once the
// hold elapses — called from onAnimTick every tick alongside every other
// windowed effect's own tick block (ripple.go/micro.go's shape). A no-op
// once active is false (settled, or skipped by a keypress).
func (m *Model) stepSplash() {
	if !m.splash.active {
		return
	}
	if m.splash.holdTicks < splashHoldTicks {
		m.splash.holdTicks++
		return
	}
	m.splash.pos, m.splash.vel = overlaySpringPhysics.Update(m.splash.pos, m.splash.vel, 1)
	if springSettled(m.splash.pos, m.splash.vel, 1) {
		m.splash.active = false
	}
}

// skipSplash ends the splash instantly — the brief's own "ANY key skips
// instantly": no rise-away animation plays, the very next frame shows the
// app underneath. See Model.onKey's own early branch.
func (m *Model) skipSplash() {
	m.splash = splashState{}
}

// pitoLogoLines is pito's OWN logo, glyph for glyph — the start
// screen's LOGO_LINES (pito app/components/pito/start_screen/
// component.rb; owner 2026-07-12: "use the logo blocks from pito …
// with those exact blocks, sizes and color pito-blue"). `█` glyphs
// paint pito-blue, the box-drawing connectors ride the dim foreground,
// spaces stay literal — the terminal twin of logo_cell_class.
var pitoLogoLines = []string{
	"██████╗ ██╗████████╗ ██████╗ ",
	"██╔══██╗██║╚══██╔══╝██╔═══██╗",
	"██████╔╝██║   ██║   ██║   ██║",
	"██╔═══╝ ██║   ██║   ██║   ██║",
	"██║     ██║   ██║   ╚██████╔╝",
	"╚═╝     ╚═╝   ╚═╝    ╚═════╝ ",
}

// pitoBlue is the brand token (--brand-pito, identical across all 18 web
// themes) — the logo's one color.
var pitoBlue = lipgloss.Color("#5170ff")

// scrambleGlyphs is the decode-effect's noise pool — braille static and
// block shades, the house's own texture rather than matrix rain.
var scrambleGlyphs = []rune("⠁⠂⠄⡀⢀⠈⠐⠠▓▒░⣿⠿·")

// scrambleReveal is the Crush-style decode-in (owner eye-candy mandate
// 2026-07-12: "fast scrambling texts"): characters resolve left to
// right as progress climbs 0→1; unresolved cells churn through
// scrambleGlyphs, re-rolling every tick (the tick seed), so the tail
// visibly boils until it locks. Spaces never scramble — word shapes
// hold while the letters decode.
func scrambleReveal(text string, progress float64, tick int64) string {
	if progress >= 1 {
		return text
	}
	runes := []rune(text)
	settled := int(progress * float64(len(runes)))
	var b strings.Builder
	for i, ru := range runes {
		switch {
		case i < settled || ru == ' ':
			b.WriteRune(ru)
		default:
			h := fnv.New32a()
			fmt.Fprintf(h, "%d:%d", i, tick)
			b.WriteRune(scrambleGlyphs[h.Sum32()%uint32(len(scrambleGlyphs))])
		}
	}
	return b.String()
}

// splashPaintLogoRow paints one logo row: solid blocks pito-blue,
// connectors dim — exactly the web's per-glyph classes, no gradient
// (the web logo is solid brand blue; the sweep belonged to the retired
// hand-drawn wordmark).
func splashPaintLogoRow(row string) string {
	blue := lipgloss.NewStyle().Foreground(pitoBlue)
	dim := lipgloss.NewStyle().Foreground(render.ColorDim)
	var b strings.Builder
	for _, ru := range row {
		switch ru {
		case ' ':
			b.WriteRune(' ')
		case '█':
			b.WriteString(blue.Render(string(ru)))
		default:
			b.WriteString(dim.Render(string(ru)))
		}
	}
	return b.String()
}

// splashView composes the full-screen startup splash: the painted PITO
// wordmark centered in the terminal, version + the dim tagline beneath it,
// the whole block padded to exactly m.height lines, then clipped by
// splashRise for the rise-away.
func (m Model) splashView() string {
	width, height := m.width, m.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	lines := make([]string, 0, len(pitoLogoLines)+3)
	for _, row := range pitoLogoLines {
		lines = append(lines, splashPaintLogoRow(row))
	}
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(render.ColorDim).Render(version.String()))
	// The web start screen's tip line, word for word: an orange "!", the
	// bold yellow prefix, an em-dash, the tip — all from pito's copy
	// (Tip::Component + pito.copy.start_screen.tips). COPY LAW: with no
	// pool mirrored (older pito ref) there is no tip line at all — the
	// old client-made tagline is gone.
	if m.splash.tip != "" {
		// The tip DECODES in across the hold's first ~60% (scrambleReveal
		// above) — the boil settles well before the rise-away starts.
		progress := float64(m.splash.holdTicks) / (float64(splashHoldTicks) * 0.6)
		tipText := scrambleReveal(m.splash.tip, progress, m.splash.holdTicks)
		tip := lipgloss.NewStyle().Foreground(render.ColorSubject).Render("!") + " " +
			lipgloss.NewStyle().Foreground(render.ColorWarn).Bold(true).Render(render.PitoCopy.StartScreen.TipPrefix) +
			lipgloss.NewStyle().Foreground(render.ColorFaint).Render(" — "+tipText)
		lines = append(lines, "", tip)
	}

	top := (height - len(lines)) / 2
	if top < 0 {
		top = 0
	}

	var b strings.Builder
	for i := 0; i < top; i++ {
		b.WriteString("\n")
	}
	for _, line := range lines {
		pad := (width - lipgloss.Width(line)) / 2
		if pad < 0 {
			pad = 0
		}
		b.WriteString(strings.Repeat(" ", pad) + line + "\n")
	}
	bottom := height - top - len(lines)
	for i := 0; i < bottom; i++ {
		b.WriteString("\n")
	}
	body := strings.TrimSuffix(b.String(), "\n")
	return splashRise(body, m.splash.pos, height)
}

// splashRise is clipOverlayBottom's (spring.go) mirror direction: as pos
// climbs 0→1 the TOP n = pos×height lines drop (the wordmark exits through
// the ceiling) and the bottom pads back in with blank lines — the opposite
// of spring.go's own bottom-clip, which pads ABOVE and drops the bottom
// (a panel rising INTO view from the bottom edge). That direction
// difference is the whole reason this lives here as its own function
// rather than a third case bolted onto clipOverlayBottom.
func splashRise(body string, pos float64, screenHeight int) string {
	if pos <= 0 {
		return body
	}
	if pos > 1 {
		pos = 1
	}
	lines := strings.Split(body, "\n")
	total := len(lines)
	n := int(pos*float64(total) + 0.5)
	if n > total {
		n = total
	}
	visible := lines[n:]
	pad := screenHeight - len(visible)
	if pad < 0 {
		pad = 0
	}
	return strings.Join(visible, "\n") + strings.Repeat("\n", pad)
}
