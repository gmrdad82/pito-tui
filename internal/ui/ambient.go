package ui

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// The ambient layer: three whisper-intensity effects that make the
// screen feel alive without ever reading as noise (owner brief,
// 2026-07-12). Each is independently kill-switchable below — flip one
// to false and that effect vanishes; nothing else needs to change.
//
//   - marginStarfieldEnabled: sparse braille dots twinkling in the
//     terminal's empty right margin, painted as a post-processing pass
//     over the assembled frame — see paintMarginStars, called from
//     Model.viewContent.
//   - shimmerConductorEnabled: every ~30s of ALIVE time (animation
//     ticks, never wall-clock idle), a ~2s window where every shimmer
//     painter's per-element stagger collapses onto one shared traveling
//     phase — see conductorWeight, pushed from the animation frame into
//     render.R via SetConductor.
//   - cableActivityPulseEnabled: the status bar's presence dot takes
//     one green breath whenever the cable delivers a message (owner
//     2026-07-12: no idle breathing; activity only) — see beginDotPulse
//     and Model.activityPulseDot.
//
// All three are additionally gated on m.truecolor at their call sites —
// non-truecolor terminals, and every golden test (none of which set
// WithTruecolor), get none of this.
const (
	marginStarfieldEnabled    = true
	starSkyEnabled            = true // the moving sky on blank rows (owner eye-candy mandate)
	shimmerConductorEnabled   = true
	cableActivityPulseEnabled = true
)

// ── The ambient heartbeat ────────────────────────────────────────────────
//
// One self-rescheduling tick loop drives EVERYTHING that animates while
// nothing is actively happening: the star sky's drift, the done-@ai
// gradient bars, shimmer-marked shiny words, and the pulsing @ai prompt
// ">" (the ambient class — Model.ambient + aiPromptLive). It replaces
// the 3.x arrangement where the sky ran its own permanent 16ms chain AND
// one finished @ai reply held the fast animation gate open forever,
// stacking a second 16ms chain on top for the process lifetime.
//
// The heartbeat and the fast chain (animTick/onAnimTick) never run at
// the same time: while the fast gate is open (pending sends, springs,
// shakes, splash, tour) onAnimTick advances the sky and the ambient
// class itself at the same 16ms cadence and the heartbeat parks;
// the moment the gate closes, Update's post-dispatch re-arm (see
// Model.Update) starts the heartbeat again. At most ONE 16ms chain
// exists at any moment.
//
// The heartbeat is also where the activity gating lives (owner mandate
// 2026-07-21, the 103%-CPU background-tab kill — fx pause when idle or
// unfocused, pre-approved):
//
//   - BLURRED (terminal reported focus-out, fx.pause_on_blur): parks
//     entirely. skyPhase is frozen, not wall-clock-advanced, so focus-in
//     resumes the sky exactly where it paused.
//   - ACTIVE (input/cable activity within fx.idle_grace_seconds): full
//     16ms ≈ 60fps rate.
//   - IDLE-FOCUSED (grace expired): throttled to fx.idle_fps. The phase
//     step scales with the interval, so the sky's wall-clock drift speed
//     is identical at every rate — same motion, fewer frames.
//   - DEEP-IDLE (fx.deep_idle_minutes with no input and no cable
//     traffic; 0 = never): parks entirely, frame frozen, until anything
//     wakes it. This is the only idle path on terminals that never
//     report focus (GNU screen, unconfigured tmux, the Linux console).
//
// Any keystroke, mouse event, cable delivery, or focus-in stamps
// Model.lastActivity and re-arms the loop through the one Update seam —
// heartbeatTicking makes the restart idempotent.

// heartbeatInterval: the heartbeat's full rate — 16ms ≈ 60fps (owner
// 2026-07-12: the first cut's 120ms/8fps read as lag on a 280Hz OLED;
// "can we use 60fps?" — yes). Bubble Tea v2's cell-diff renderer only
// writes the cells that changed, so the full-rate loop stays cheap while
// it actually runs; the activity gating above decides when it runs.
const heartbeatInterval = 16 * time.Millisecond

// skyPhaseStep: drift per 16ms tick — the same drift speed the original
// 120ms loop had (0.35/tick), spread across 7.5× the frames.
const skyPhaseStep = 0.047

func heartbeatTick(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg { return HeartbeatTickMsg{} })
}

// startHeartbeat arms the ambient heartbeat when anything needs it and
// the activity plan allows it — a no-op otherwise, and idempotent while
// a chain is already live (heartbeatTicking, cleared only by
// onHeartbeatTick itself, the fpsTicking shape). Called from ONE seam:
// Model.Update's post-dispatch re-arm, so every message that changes the
// answer (a keystroke ending deep-idle, a focus-in, a done-@ai reply
// arming the ambient class, the fast gate closing) reconsiders it.
func (m *Model) startHeartbeat() tea.Cmd {
	if !m.heartbeatNeeded() || m.heartbeatTicking || m.animating {
		return nil
	}
	if _, park := m.heartbeatPlan(m.now()); park {
		return nil
	}
	m.heartbeatTicking = true
	m.hbInterval = heartbeatInterval
	return heartbeatTick(heartbeatInterval)
}

// heartbeatNeeded reports whether the heartbeat has anything at all to
// animate: the star sky, or the ambient class (done-@ai bars, shiny
// words, the pulsing @ai prompt). Truecolor gates the lot — exactly the
// terminals where none of these effects exist.
func (m Model) heartbeatNeeded() bool {
	if !m.truecolor {
		return false
	}
	return (starSkyEnabled && m.skyOn) || m.ambientAlive()
}

// heartbeatPlan picks the heartbeat's next interval from the activity
// state (see the state ladder in the package comment above): park on
// blur or deep-idle, 16ms inside the grace window, the fx.idle_fps
// throttle beyond it. Pure — now is a parameter so tests never sleep.
func (m Model) heartbeatPlan(now time.Time) (interval time.Duration, park bool) {
	if m.fxPauseOnBlur && !m.focused {
		return 0, true
	}
	idle := now.Sub(m.lastActivity)
	if m.fxDeepIdle > 0 && idle >= m.fxDeepIdle {
		return 0, true
	}
	if idle < m.fxIdleGrace {
		return heartbeatInterval, false
	}
	fps := m.fxIdleFPS
	if fps < 1 {
		fps = 1
	} else if fps > 60 {
		fps = 60
	}
	interval = time.Second / time.Duration(fps)
	if interval < heartbeatInterval {
		interval = heartbeatInterval
	}
	return interval, false
}

func (m Model) onHeartbeatTick() (tea.Model, tea.Cmd) {
	if !m.heartbeatNeeded() || m.animating {
		// Nothing left to animate — or the fast chain is open and owns
		// the 16ms cadence (onAnimTick advances sky + ambient itself;
		// Update's re-arm restarts this loop the moment its gate
		// closes). Either way: park, don't reschedule.
		m.heartbeatTicking = false
		return m, nil
	}
	interval, park := m.heartbeatPlan(m.now())
	if park {
		// Blur or deep-idle: the frame freezes exactly here — no step,
		// so focus-in/wake resumes the sky where it paused.
		m.heartbeatTicking = false
		return m, nil
	}
	// Step by the interval THIS tick was scheduled with (the elapsed
	// time), then reschedule at whatever the plan now says — the phase
	// advances at the same wall-clock speed at every rate.
	m.stepAmbient(float64(m.hbInterval) / float64(heartbeatInterval))
	m.hbInterval = interval
	return m, heartbeatTick(interval)
}

// stepAmbient advances everything the heartbeat animates by one tick of
// `scale` × the full-rate step. Shared by the heartbeat (scale = its
// interval ratio) and nothing else — the fast chain's onAnimTick has its
// own step of the same fields at scale 1, plus all the windowed effects.
func (m *Model) stepAmbient(scale float64) {
	if starSkyEnabled && m.skyOn {
		m.skyPhase += skyPhaseStep * scale
	}
	if !m.ambientAlive() {
		return
	}
	// aliveTicks counts ticks, not wall-clock — under throttle the
	// conductor's ~30s cycle stretches, which is its contract ("alive
	// time"; ambient.go's conductorWeight doc).
	m.aliveTicks++
	m.phase += shimmerStep * scale
	if m.phase >= 1 {
		m.phase -= 1
	}
	m.pushAnimFrame()
	// The 30fps shimmer beat (owner split-rate order): every second tick
	// at the full rate; every tick once the interval itself is at or
	// below the beat rate.
	if scale > 1 || m.aliveTicks%transcriptBeatDivisor == 0 {
		m.touchAnimatedTurns()
	}
}

// ── Effect 1: margin star-field ──────────────────────────────────────────

// starMinMargin is the brief's own threshold: the terminal must be at
// least this many columns wider than the content column before any
// star gets painted — narrower than that and "the margin" doesn't read
// as a margin at all, just a ragged edge.
const starMinMargin = 12

// starDensity: roughly one star candidate per this many margin cells —
// sparse dust, not a wall.
const starDensity = 80

// starGlyphs are the eight single-dot braille cells (one dot lit, seven
// blank). Which glyph a given star draws is itself hashed from its
// position, so neighbors don't all draw the identical dot.
var starGlyphs = [8]rune{'⠁', '⠂', '⠄', '⡀', '⢀', '⠈', '⠐', '⠠'}

// The natural sky (owner 2026-07-12: "breathing, color purple, blue,
// yellow, like real natural stars, different dots sizes"): starTints
// are stellar classes weighted like a real field — most stars
// near-white, then blue-white, warm yellow, and the house purple.
// starSizeFor is the size ladder — dust dominates, doubles are common
// enough to notice, real star glyphs stay rare so they mean something.
var starTints = [4]render.RGB{
	{R: 0xd8, G: 0xd8, B: 0xe8}, // near-white
	{R: 0x9d, G: 0xb8, B: 0xff}, // blue-white
	{R: 0xff, G: 0xe9, B: 0xa3}, // warm yellow
	{R: 0xbb, G: 0x9a, B: 0xf7}, // purple
}

func starTintFor(h uint32) render.RGB {
	switch h % 10 {
	case 0, 1, 2, 3:
		return starTints[0]
	case 4, 5, 6:
		return starTints[1]
	case 7, 8:
		return starTints[2]
	default:
		return starTints[3]
	}
}

// starSizeFor: 0 = dust (70%), 1 = double dot (20%), 2 = bright ✧ (8%),
// 3 = brilliant ✦ (2%).
func starSizeFor(h uint32, fallback rune) (rune, int) {
	switch v := h % 50; {
	case v < 35:
		return fallback, 0
	case v < 45:
		return [4]rune{'⠃', '⠘', '⠰', '⡈'}[h%4], 1
	case v < 49:
		return '✧', 2
	default:
		return '✦', 3
	}
}

// ambientStarBG/ambientStarFaint bound every star's brightness: the
// twinkle's low point (near-background, almost invisible) and its high
// point — capped at roughly ColorFaint's own weight (owner rule: no
// star ever brighter than ColorFaint), never anything louder.
var (
	ambientStarBG    = render.RGB{R: 0x16, G: 0x16, B: 0x1a}
	ambientStarFaint = render.RGB{R: 0x62, G: 0x62, B: 0x62} // ≈ ColorFaint (256-color 241)
)

// starHash is an inline FNV-1a over the byte sequence "%d<sep>%d" — the
// EXACT bytes the retired fmt.Fprintf(fnv.New32a(), …) hashed, so every
// star keeps its position, glyph, tint, size, and breathing period
// (TestStarHashMatchesFnv pins the equivalence over a full grid). The
// fmt/fnv round-trip was ~12% of all idle CPU plus one allocation per
// probe; this is a stack buffer and a few multiplies.
func starHash(a, b int, sep byte) uint32 {
	var buf [40]byte
	s := strconv.AppendInt(buf[:0], int64(a), 10)
	s = append(s, sep)
	s = strconv.AppendInt(s, int64(b), 10)
	h := uint32(2166136261) // FNV-1a offset basis
	for _, c := range s {
		h ^= uint32(c)
		h *= 16777619 // FNV-1a prime
	}
	return h
}

// skyStar is one cached star of a row. abs is the ABSOLUTE sampled
// column (screen col + drift base) — the coordinate star identity hashes
// from — so a cached star stays valid as the drift base slides; the
// paint pass derives its screen column per frame (abs − base).
type skyStar struct {
	abs    int
	glyph  rune
	offset float64
	tint   render.RGB
	size   int     // 0 dust … 3 brilliant (starSizeFor)
	period float64 // per-star breathing period multiplier (0.6..1.6)
}

// starForAbs builds the full skyStar at an absolute column, or reports
// no star there — the ONE place a sky star's derived fields (sized
// glyph, tint, period) are computed, shared by the fresh sweep and the
// incremental slide so they cannot disagree.
func starForAbs(saltedRow, abs int) (skyStar, bool) {
	present, glyph, offset := starAt(saltedRow, abs)
	if !present {
		return skyStar{}, false
	}
	sum := starHash(saltedRow, abs, '/')
	sized, size := starSizeFor(sum>>4, glyph)
	return skyStar{
		abs: abs, glyph: sized, offset: offset,
		tint:   starTintFor(sum >> 12),
		size:   size,
		period: 0.6 + float64(sum%97)/97,
	}, true
}

// skyRowEntry is one salted row's live star list, kept sorted by
// ascending absolute column and maintained INCREMENTALLY as the drift
// base slides: each +1 base step drops at most one star off the bottom
// of the window and probes exactly one new column at the top, instead
// of re-sweeping every column of the row (which the old (row, base,
// termWidth)-keyed memo did on every base advance — layer 2's base moves
// every ~2.7 frames, so that cache never amortized). The UI is
// single-goroutine — no lock.
type skyRowEntry struct {
	base      int
	termWidth int
	stars     []skyStar
}

var skyRowCache = map[int]*skyRowEntry{}

// sweepRowStars is the full-scan builder: every screen column of the
// window at this base, in ascending absolute-column order. The rebuild
// path (fresh row, resize, backward/large base jump) and the reference
// the incremental slide is tested against.
func sweepRowStars(saltedRow, base, termWidth int) []skyStar {
	stars := []skyStar{}
	for col := 2; col < termWidth-1; col++ {
		if st, ok := starForAbs(saltedRow, col+base); ok {
			stars = append(stars, st)
		}
	}
	return stars
}

// skyRowStars returns the row's stars for the window at `base` —
// incrementally slid forward when possible, rebuilt otherwise. The
// returned slice is the cache's own; callers only read it within the
// same frame.
func skyRowStars(saltedRow, base, termWidth int) []skyStar {
	e := skyRowCache[saltedRow]
	if e == nil || e.termWidth != termWidth || base < e.base || base-e.base > termWidth {
		if len(skyRowCache) > 4096 {
			// Bound leftover rows from old sizes/salts (a resize churn's
			// worth is tiny; this is belt and braces, not a hot path).
			skyRowCache = map[int]*skyRowEntry{}
		}
		e = &skyRowEntry{base: base, termWidth: termWidth, stars: sweepRowStars(saltedRow, base, termWidth)}
		skyRowCache[saltedRow] = e
		return e.stars
	}
	for e.base < base {
		e.base++
		// The window is absolute cols [2+base, termWidth-2+base]: one
		// column leaves at the bottom edge…
		for len(e.stars) > 0 && e.stars[0].abs < 2+e.base {
			e.stars = e.stars[1:]
		}
		// …and exactly one candidate enters at the top.
		if st, ok := starForAbs(saltedRow, termWidth-2+e.base); ok {
			e.stars = append(e.stars, st)
		}
	}
	return e.stars
}

// starAt deterministically decides whether (row, col) carries a star:
// same coordinates always produce the same answer, and if present, the
// same glyph and the same phase offset — the star field's positions
// never reshuffle between frames. Only a present star's brightness ever
// moves, and only when phase itself moves.
func starAt(row, col int) (present bool, glyph rune, offset float64) {
	sum := starHash(row, col, ':')
	if sum%starDensity != 0 {
		return false, 0, 0
	}
	glyph = starGlyphs[(sum>>8)%8]
	offset = float64((sum>>16)%997) / 997
	return true, glyph, offset
}

// writeSGRGlyph appends one truecolor-foreground glyph to b as a direct
// escape sequence: "\x1b[38;2;R;G;Bm<glyph>\x1b[m" — byte-identical to
// lipgloss.NewStyle().Foreground(hex).Render(string(glyph)) (pinned by
// TestWriteSGRGlyphMatchesLipgloss), with none of the box-model
// machinery. The lipgloss round-trip here was the single hottest line
// of the idle profile: a fresh Style + hexColor's fmt.Sprintf per star
// cell per frame — ~a third of ALL CPU.
func writeSGRGlyph(b *strings.Builder, c render.RGB, glyph rune) {
	var num [4]byte
	b.WriteString("\x1b[38;2;")
	b.Write(strconv.AppendUint(num[:0], uint64(c.R), 10))
	b.WriteByte(';')
	b.Write(strconv.AppendUint(num[:0], uint64(c.G), 10))
	b.WriteByte(';')
	b.Write(strconv.AppendUint(num[:0], uint64(c.B), 10))
	b.WriteByte('m')
	b.WriteRune(glyph)
	b.WriteString("\x1b[m")
}

// skyScratch: reusable per-row accumulation buffers for paintStarSky —
// column-indexed slices replacing the three map[int] allocations the
// old paint pass made per blank row per frame (most of the idle GC
// share). Single-goroutine UI, same no-lock rule as skyRowCache; glyph
// 0 is the "no star landed here" sentinel (no starGlyph is ever 0), and
// touched lists exactly the columns to repaint AND to zero afterwards.
var skyScratch struct {
	cells   []float64
	glyphs  []rune
	tints   []render.RGB
	touched []int
}

func ensureSkyScratch(termWidth int) {
	if len(skyScratch.cells) < termWidth {
		skyScratch.cells = make([]float64, termWidth)
		skyScratch.glyphs = make([]rune, termWidth)
		skyScratch.tints = make([]render.RGB, termWidth)
	}
}

// paintStarSky is the star field's 2.0.0 upgrade (owner eye-candy
// mandate, 2026-07-12: "on spare space use some starfield / star sky
// stars moving"): the spare space is no longer a capped margin — it is
// every EMPTY row of the frame's content region. Two parallax layers of
// the same deterministic stars drift horizontally at different speeds
// (the whole sampling grid slides with skyPhase, so positions stay
// stable relative to the field while the field itself glides), each
// star still twinkling on its own offset. Blank rows only — a row with
// ANY real content is never touched, so the sky lives strictly in the
// void below short conversations and behind the splash.
func paintStarSky(body string, contentRows, termWidth int, skyPhase float64) string {
	if termWidth < 20 {
		return body
	}
	lines := strings.Split(body, "\n")
	if contentRows > len(lines) {
		contentRows = len(lines)
	}
	ensureSkyScratch(termWidth)
	// Layer drifts in CONTINUOUS cells: slow background, faster
	// foreground. The fractional part drives sub-cell motion blending —
	// a star fades out of its cell and into the next as it glides, so
	// 60fps reads as sliding, not stepping (the first cut's whole-cell
	// jumps were half the perceived lag).
	type layerDrift struct {
		base int
		frac float64
	}
	var layers [2]layerDrift
	for i, speed := range [2]float64{3, 8} {
		d := skyPhase * speed
		base := math.Floor(d)
		layers[i] = layerDrift{base: int(base), frac: d - base}
	}
	cells, glyphs, tints := skyScratch.cells, skyScratch.glyphs, skyScratch.tints
	for row := 0; row < contentRows; row++ {
		if strings.TrimSpace(lines[row]) != "" {
			continue
		}
		// intensity per column: blended contributions land in the
		// scratch slices first, then paint in one left-to-right pass.
		touched := skyScratch.touched[:0]
		for layer, ld := range layers {
			for _, st := range skyRowStars(row+layer*3691, ld.base, termWidth) {
				col := st.abs - ld.base
				// Each star breathes on its OWN period (organic — a
				// field, not a metronome); size deepens the breath and
				// raises the ceiling: dust whispers, ✦ glows.
				breath := (math.Sin((skyPhase*0.13*st.period+st.offset)*2*math.Pi) + 1) / 2
				depth := 0.35 + 0.65*breath
				ceiling := [4]float64{0.45, 0.6, 0.8, 1.0}[st.size]
				pulse := depth * ceiling
				// The star sits between col and col-1 by frac: split its
				// light across both cells (linear crossfade).
				if cells[col] < pulse*(1-ld.frac) {
					if glyphs[col] == 0 {
						touched = append(touched, col)
					}
					cells[col] = pulse * (1 - ld.frac)
					glyphs[col] = st.glyph
					tints[col] = st.tint
				}
				if col-1 >= 2 && cells[col-1] < pulse*ld.frac {
					if glyphs[col-1] == 0 {
						touched = append(touched, col-1)
					}
					cells[col-1] = pulse * ld.frac
					glyphs[col-1] = st.glyph
					tints[col-1] = st.tint
				}
			}
		}
		if len(touched) == 0 {
			skyScratch.touched = touched
			continue
		}
		sort.Ints(touched)
		var b strings.Builder
		cursor := 0
		for _, col := range touched {
			for ; cursor < col; cursor++ {
				b.WriteByte(' ')
			}
			writeSGRGlyph(&b, lerpRGB(ambientStarBG, tints[col], cells[col]), glyphs[col])
			cursor++
		}
		lines[row] = b.String()
		// Reset exactly the cells this row lit, ready for the next row.
		for _, col := range touched {
			cells[col], glyphs[col] = 0, 0
		}
		skyScratch.touched = touched[:0]
	}
	return strings.Join(lines, "\n")
}

// paintMarginStars overlays the twinkling star field on an assembled
// frame's empty right margin — a post-processing pass (pad-to-margin +
// overlay), never a change to how any section rendered its own content.
// Two hard boundaries, both enforced by construction rather than
// trusted from the caller:
//
//   - never inside the content column: a row's star columns always
//     start at max(contentWidth, that row's own real rendered width),
//     so even a line that overflows contentWidth can't get a star
//     stamped over real content.
//   - never on the input/status lines: viewContent always appends
//     exactly those two as the frame's FINAL two entries, so the last
//     two lines of the split are unconditionally skipped.
//
// phase rides whatever the model's existing shimmer phase is — stars
// have no tick of their own (the fast gate must still close when
// only ambience remains); they simply draw with whatever phase was
// last pushed by a real animation tick.
func paintMarginStars(body string, contentWidth, termWidth int, phase float64) string {
	if termWidth-contentWidth < starMinMargin {
		return body
	}
	lines := strings.Split(body, "\n")
	contentRows := len(lines) - 2 // input + status, always last (see doc comment above)
	if contentRows < 1 {
		return body
	}
	type star struct {
		col    int
		glyph  rune
		offset float64
	}
	for row := 0; row < contentRows; row++ {
		line := lines[row]
		w := lipgloss.Width(line)
		start := contentWidth
		if w > start {
			start = w
		}
		if start >= termWidth {
			continue
		}
		var stars []star
		for col := start; col < termWidth; col++ {
			if present, glyph, offset := starAt(row, col); present {
				stars = append(stars, star{col, glyph, offset})
			}
		}
		if len(stars) == 0 {
			continue
		}
		var b strings.Builder
		b.WriteString(line)
		cursor := w
		for _, s := range stars {
			for ; cursor < s.col; cursor++ {
				b.WriteByte(' ')
			}
			pulse := (math.Sin((phase+s.offset)*2*math.Pi) + 1) / 2 // 0..1..0
			writeSGRGlyph(&b, lerpRGB(ambientStarBG, ambientStarFaint, pulse), s.glyph)
			cursor++
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}

// ── Effect 2: shimmer conductor ──────────────────────────────────────────

// conductorCycleTicks/conductorWindowTicks translate the brief's "every
// ~30s of ALIVE time, a 2s sweep window" into shimmerTick (40ms) tick
// counts — both exact divisions (750 and 50 ticks), no rounding.
const (
	conductorCycleTicks  = int64(30 * time.Second / shimmerTick)
	conductorWindowTicks = int64(2 * time.Second / shimmerTick)
)

// conductorWeight is aliveTicks' pure projection onto the shimmer
// conductor's blend weight: 0 for the first ~28s of every ~30s cycle,
// then a half-sine ramp 0→1→0 across the final ~2s window — the moment
// every shimmer painter's stagger (render.R's staggered/staggered20)
// collapses onto the SAME r.phase and the screen reads as one
// traveling wave, before scattering back to normal. Always 0 when
// shimmerConductorEnabled is false — the kill switch.
func conductorWeight(aliveTicks int64) float64 {
	if !shimmerConductorEnabled {
		return 0
	}
	pos := aliveTicks % conductorCycleTicks
	windowStart := conductorCycleTicks - conductorWindowTicks
	if pos < windowStart {
		return 0
	}
	progress := float64(pos-windowStart) / float64(conductorWindowTicks)
	return math.Sin(progress * math.Pi)
}

// ── Effect 3: cable-activity pulse ───────────────────────────────────────
// Owner re-spec 2026-07-12: "have the idle have no breathing. breathing
// should be green only when activity appears on cable." The old idle
// breathing (its own 500ms loop, gradient hues) is GONE; in its place
// the healthy dot takes one green breath every time the cable actually
// delivers something — a heartbeat on traffic, silence at rest.

// dotPulseTicksTotal: one inhale-exhale, ~1.2s of fast-loop ticks.
const dotPulseTicksTotal = int64(1200 * time.Millisecond / shimmerTick)

// beginDotPulse arms the pulse window (called from onCableEvent for
// every delivered message). The caller keeps ticks flowing via
// animate(), same as the ripple.
func (m *Model) beginDotPulse() {
	if !cableActivityPulseEnabled || !m.truecolor {
		return
	}
	m.dotPulseTicks = dotPulseTicksTotal
}

// activityPulseDot renders the presence dot mid-pulse: a sine breath
// between dim green and the full OK green — green only (owner ruling),
// never the gradient hues the retired idle breath used.
func (m Model) activityPulseDot() string {
	progress := 1 - float64(m.dotPulseTicks)/float64(dotPulseTicksTotal)
	pulse := math.Sin(progress * math.Pi) // 0→1→0 over the window
	const floor = 0.45
	t := floor + pulse*(1-floor)
	c := render.RGB{
		R: uint8(0x5f * t / 1),
		G: uint8(float64(0xd7) * t),
		B: uint8(float64(0x87) * t),
	}
	var b strings.Builder
	writeSGRGlyph(&b, c, '■')
	return b.String()
}

// ── shared helpers ────────────────────────────────────────────────────────

// lerpRGB blends a→b at t (clamped to [0,1] — callers may hand in a
// sine that overshoots at the float edges).
func lerpRGB(a, b render.RGB, t float64) render.RGB {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return render.RGB{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B)}
}

// hexColor adapts a render.RGB for lipgloss styles — still what the
// windowed effects (ripple, toast, thumb) use; the per-frame star paths
// above bypass it entirely via writeSGRGlyph.
func hexColor(c render.RGB) color.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B))
}
