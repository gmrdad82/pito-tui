package render

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func TestShiniesRailsAndBadges(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "shinies_reply_v2", 110))
	// One lane per metric with rail + legend.
	for _, want := range []string{"Subs", "at 2.2K · next: 5K (Ruby)", "●", "◉", "·"} {
		if !strings.Contains(out, want) {
			t.Errorf("shinies lane missing %q:\n%s", want, out)
		}
	}
	// Badges flow under the rail, faces intact.
	for _, want := range []string{"1 Sub", "2 Subs"} {
		if !strings.Contains(out, want) {
			t.Errorf("badge face missing %q:\n%s", want, out)
		}
	}
	// Shinies-lane badges are the web's EXTENDED form (metric_row_component's
	// default, unlike the compact detail-card strip): the unlock date rides
	// along as a dim suffix, printed verbatim exactly as the fixture sends
	// it ("Jun '26" here) — shinyDateSuffix no longer reformats it locally,
	// the year-drop lives server-side in badge_component.rb now.
	if !strings.Contains(out, "Jun") {
		t.Errorf("extended badges must carry their unlock date:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 110 {
			t.Errorf("line exceeds width (%d): %q", w, line)
		}
	}
}

func TestShinyBadgeComponent(t *testing.T) {
	withTrueColor(t)
	r := New(80, WithTruecolor(true))
	badge := stripANSI(r.ShinyBadge("gold", "100K Subs", ""))
	if !strings.Contains(badge, "100K Subs") {
		t.Errorf("badge face missing: %q", badge)
	}
	long := stripANSI(r.ShinyBadge("jade", "200 Viewsandmore", ""))
	if !strings.Contains(long, "…") {
		t.Errorf("compact form must trim with ellipsis: %q", long)
	}
	if unknown := stripANSI(r.ShinyBadge("vibranium", "5 Things", "")); !strings.Contains(unknown, "5 Things") {
		t.Errorf("unknown material must degrade to the neutral pill: %q", unknown)
	}
}

// TestShinyBadgeExtendedCarriesDateCompactDoesNot exercises the FULL
// pipeline (html.go's marker extraction -> tokens.go's paintTokens) on
// minimal fragments shaped exactly like the web's two badge forms
// (badge_component.rb): a compact detail-card badge never renders a
// __date child at all, so it must stay date-less; an extended badge
// (shinies-tool lanes) does, and the TUI prints it verbatim — no local
// re-formatting, single-source-of-truth rule. The year-drop badge_component
// now does server-side (current year → "%b", other years → "%b '%y") is
// exercised here by feeding both shapes straight through unchanged,
// including under a clock that would have called for the OPPOSITE rule
// under the TUI's old local re-format — proving nothing local touches it.
func TestShinyBadgeExtendedCarriesDateCompactDoesNot(t *testing.T) {
	withTrueColor(t)
	// The __date span's OWN class ("pito-shiny__date block") contains the
	// substring "pito-shiny" — a trap for html.go's badge extraction,
	// which matches on that same substring. A regression here nests a
	// SECOND marker sequence inside the date segment (owner bug caught in
	// the 2026-07-12 vhs capture: tofu glyphs bleeding into the pill), so
	// this test checks the RENDERED TEXT EXACTLY, not just substrings.
	yearless := `<span class="pito-shiny" data-material="wood">1 Sub<span class="pito-shiny__date block">Jun</span></span>`
	yearBearing := `<span class="pito-shiny" data-material="wood">1 Sub<span class="pito-shiny__date block">Jun &#39;25</span></span>`
	compact := `<span class="pito-shiny pito-shiny--compact" data-material="wood">1 Sub</span>`
	requireNoMarkerRunes := func(t *testing.T, s string) {
		t.Helper()
		for _, marker := range []rune{ShinyStart, ShinySep, ShinyEnd, ShinySpace} {
			if strings.ContainsRune(s, marker) {
				t.Errorf("marker rune %U leaked into rendered output: %q", marker, s)
			}
		}
	}

	// Pinned to the SAME year the year-bearing fixture below carries — the
	// old local rule would have dropped "'25" here; verbatim pass-through
	// must not.
	now := time.Date(2025, 6, 20, 12, 0, 0, 0, time.UTC)
	r := New(80, WithTruecolor(true), WithNow(func() time.Time { return now }))

	yearlessOut := stripANSI(r.paintTokens(FlattenHTML(yearless), lipgloss.NewStyle()))
	requireNoMarkerRunes(t, yearlessOut)
	// The server already sent no year: verbatim pass-through renders
	// exactly "1 Sub · Jun" (padded with the pill's own leading/trailing
	// space and edge caps).
	if !strings.Contains(yearlessOut, "1 Sub · Jun ") {
		t.Errorf("extended badge must read exactly %q, got: %q", "1 Sub · Jun", yearlessOut)
	}

	yearOut := stripANSI(r.paintTokens(FlattenHTML(yearBearing), lipgloss.NewStyle()))
	requireNoMarkerRunes(t, yearOut)
	// The server sent a year-bearing date, even though it matches r.now()'s
	// year: verbatim pass-through must keep it exactly as sent, not re-run
	// the old same-year elision.
	if !strings.Contains(yearOut, "1 Sub · Jun '25 ") {
		t.Errorf("extended badge must read exactly %q, got: %q", "1 Sub · Jun '25", yearOut)
	}

	compOut := stripANSI(r.paintTokens(FlattenHTML(compact), lipgloss.NewStyle()))
	requireNoMarkerRunes(t, compOut)
	if !strings.Contains(compOut, "1 Sub") {
		t.Errorf("compact badge face missing: %q", compOut)
	}
	if strings.Contains(compOut, "Jun") || strings.Contains(compOut, "·") {
		t.Errorf("compact badges must stay date-less: %q", compOut)
	}
}

// TestShinyMarkerRoundTripBackwardCompatible pins the pre-glow-up marker
// shape (material + face, no date segment — the only shape any TUI build
// before this pass ever produced) and confirms paintTokens/plainTokens
// still render it exactly as before: SplitN's new limit-3 must not
// disturb a payload that only ever had one separator.
func TestShinyMarkerRoundTripBackwardCompatible(t *testing.T) {
	withTrueColor(t)
	r := New(80, WithTruecolor(true))
	old := string(ShinyStart) + "gold" + string(ShinySep) + "1" + string(ShinySpace) + "Sub" + string(ShinyEnd)

	out := stripANSI(r.paintTokens(old, lipgloss.NewStyle()))
	if !strings.Contains(out, "1 Sub") {
		t.Errorf("two-segment (legacy) shiny marker must still render its face: %q", out)
	}
	for _, marker := range []rune{ShinyStart, ShinySep, ShinyEnd, ShinySpace} {
		if strings.ContainsRune(out, marker) {
			t.Errorf("marker rune %U leaked into rendered output: %q", marker, out)
		}
	}

	if got := plainTokens(old); got != "1 Sub" {
		t.Errorf("plainTokens must degrade a legacy shiny marker to its face text, got %q", got)
	}
}

// TestShinyHaloPhasePulse20Bounded covers effect 1 (breathing halo): the
// sine primitive stays in [-1,1], is deterministic for a pinned r.phase,
// gives distinct badges distinct phases, and scaleRGB — the ±25%
// brightness envelope it drives — never over/undershoots its bounds.
func TestShinyHaloPhasePulse20Bounded(t *testing.T) {
	r := New(80, WithTruecolor(true))
	for _, phase := range []float64{0, 0.1, 0.33, 0.5, 0.77, 0.999} {
		r.SetPhase(phase)
		if p := r.phasePulse20("halo-1 Sub"); p < -1 || p > 1 {
			t.Fatalf("phasePulse20(%v) = %v out of [-1,1]", phase, p)
		}
	}
	r.SetPhase(0.42)
	a := r.phasePulse20("halo-100K Subs")
	if b := r.phasePulse20("halo-100K Subs"); a != b {
		t.Errorf("phasePulse20 must be deterministic for the same seed: %v != %v", a, b)
	}
	if c := r.phasePulse20("halo-1 Like"); a == c {
		t.Errorf("distinct badges should not share the exact same halo phase (own phase per badge): %v", a)
	}

	edge := RGB{0xa0, 0x6a, 0x35}
	if lo := scaleRGB(edge, 0.75); lo.R > edge.R || lo.G > edge.G || lo.B > edge.B {
		t.Errorf("0.75 factor must darken every channel: %+v -> %+v", edge, lo)
	}
	if hi := scaleRGB(edge, 1.25); hi.R < edge.R || hi.G < edge.G || hi.B < edge.B {
		t.Errorf("1.25 factor must brighten every channel: %+v -> %+v", edge, hi)
	}
	if bright := scaleRGB(RGB{250, 250, 250}, 1.25); bright.R != 255 || bright.G != 255 || bright.B != 255 {
		t.Errorf("scaleRGB must clamp at 255: %+v", bright)
	}
}

// TestShinyInkGlintFactorBounded covers effect 2's text-brightening half:
// inkGlintFactor never returns outside [0, inkGlintStrength], is exactly
// 0 below the threshold (most of a badge's face, given the tightened
// gleam sigma), and saturates at inkGlintStrength rather than climbing
// past it as the raw gleam intensity approaches its own peak of 1.
func TestShinyInkGlintFactorBounded(t *testing.T) {
	for _, g := range []float64{-1, 0, 0.1, inkGlintThreshold - 0.01, inkGlintThreshold, inkGlintThreshold + 0.01, 0.7, 1, 2} {
		f := inkGlintFactor(g)
		if f < 0 || f > inkGlintStrength {
			t.Fatalf("inkGlintFactor(%v) = %v out of [0,%v]", g, f, inkGlintStrength)
		}
		if g < inkGlintThreshold && f != 0 {
			t.Errorf("inkGlintFactor(%v) below threshold must be 0, got %v", g, f)
		}
	}
	if f := inkGlintFactor(1); f != inkGlintStrength {
		t.Errorf("inkGlintFactor should saturate at inkGlintStrength for g=1, got %v", f)
	}
}

// TestShinySparkleCadenceDeterministicUnderPinnedPhase covers effect 3:
// the iridescent trailing twinkle fires in exact, deterministic 3-tick
// bursts once every sparkleCycleTicks (100 ticks ⇒ ~4s), riding r.ticks
// (model.go's aliveTicks, forwarded via SetTicks) rather than a new
// ticker — pinning r.ticks directly exercises that cadence without
// needing a live animation loop.
func TestShinySparkleCadenceDeterministicUnderPinnedPhase(t *testing.T) {
	r := New(80, WithTruecolor(true))
	seed := "sparkle-1K Subs"

	trace := func() []int64 {
		var on []int64
		for tick := int64(0); tick < 3*sparkleCycleTicks; tick++ {
			r.SetTicks(tick)
			if r.sparkleActive(seed) {
				on = append(on, tick)
			}
		}
		return on
	}

	on := trace()
	if len(on) != 3*int(sparkleWindowTicks) {
		t.Fatalf("want exactly %d active ticks across 3 cycles (%d each), got %d: %v",
			3*sparkleWindowTicks, sparkleWindowTicks, len(on), on)
	}
	for cycle := 0; cycle < 3; cycle++ {
		run := on[cycle*int(sparkleWindowTicks) : (cycle+1)*int(sparkleWindowTicks)]
		for i := 1; i < len(run); i++ {
			if run[i] != run[i-1]+1 {
				t.Errorf("cycle %d sparkle run not contiguous: %v", cycle, run)
			}
		}
		if cycle > 0 && run[0] != on[0]+int64(cycle)*sparkleCycleTicks {
			t.Errorf("cycle %d sparkle window drifted: want start %d, got %d",
				cycle, on[0]+int64(cycle)*sparkleCycleTicks, run[0])
		}
	}

	// Deterministic: the same pinned tick always gives the same answer.
	r.SetTicks(on[0])
	if !r.sparkleActive(seed) {
		t.Errorf("sparkleActive must be deterministic for a pinned tick")
	}
	r.SetTicks(on[0])
	if !r.sparkleActive(seed) {
		t.Errorf("sparkleActive must reproduce the same answer on replay")
	}

	// Own phase per badge: a different face hashes to a different offset
	// within the cycle, so its window doesn't start where seed's does.
	other := "sparkle-1 Like"
	if phaseOffset(seed) == phaseOffset(other) {
		t.Fatalf("test fixture collision: pick seeds with different phaseOffset hashes")
	}
}

func TestStatsAndShiniesSitInTheirOwnTable(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "show_channel_v2", 100))
	lines := strings.Split(out, "\n")
	statsIdx, handleIdx, blankBetween := -1, -1, false
	for i, line := range lines {
		if strings.Contains(line, "Stats") && statsIdx < 0 {
			statsIdx = i
		}
		if strings.Contains(line, "Handle") && handleIdx < 0 {
			handleIdx = i
		}
	}
	if statsIdx < 0 || handleIdx < 0 || statsIdx > handleIdx {
		t.Fatalf("Stats block must precede the details table (stats=%d handle=%d):\n%s", statsIdx, handleIdx, out)
	}
	for _, line := range lines[statsIdx:handleIdx] {
		if strings.TrimSpace(stripANSI(line)) == "┃" || strings.TrimSpace(stripANSI(line)) == "" {
			blankBetween = true
		}
	}
	if !blankBetween {
		t.Errorf("a blank row must separate the two tables:\n%s", out)
	}
}
