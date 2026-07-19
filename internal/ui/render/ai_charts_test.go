package render

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

// ---------------------------------------------------------------------
// sparkline

func TestAiSparklineBlockShapeAndScaling(t *testing.T) {
	out := plain().aiSparklineBlock([]byte(`{"series":[0,1,2,3,4,5]}`), 60)
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("sparkline must be exactly 2 rows, got %d:\n%s", len(lines), out)
	}
	for _, line := range lines {
		if n := len([]rune(stripANSI(line))); n != aiChartCols {
			t.Errorf("sparkline row must be %d cells wide, got %d: %q", aiChartCols, n, line)
		}
	}
	assertBrailleOnly(t, out)

	// series_max scaling: a tiny ceiling should fill more of the top row
	// than a huge one for the SAME series.
	tightCeiling := plain().aiSparklineBlock([]byte(`{"series":[5,5,5],"series_max":5}`), 60)
	looseCeiling := plain().aiSparklineBlock([]byte(`{"series":[5,5,5],"series_max":5000}`), 60)
	tightTop := strings.Split(tightCeiling, "\n")[0]
	looseTop := strings.Split(looseCeiling, "\n")[0]
	if tightTop == looseTop {
		t.Errorf("series_max must change the plotted height:\ntight=%q\nloose=%q", tightTop, looseTop)
	}
	// The loose ceiling barely clears the baseline floor: its top row
	// stays blank, which paintBraille shows as the dotted-paper '⠂'
	// background — never an actual plotted dot.
	if !strings.ContainsRune(looseTop, '⠂') {
		t.Errorf("a series dwarfed by series_max should leave the top row on the blank paper grid: %q", looseTop)
	}
}

func TestAiSparklineBlockLabelRendersAboveThePlot(t *testing.T) {
	out := plain().aiSparklineBlock([]byte(`{"series":[1,2,3],"label":"views/day"}`), 60)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("labelled sparkline must be label + 2 rows, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "views/day") {
		t.Errorf("label must lead the block: %q", lines[0])
	}
}

func TestAiSparklineBlockDegrades(t *testing.T) {
	cases := map[string]string{
		"malformed json": `not json`,
		"empty series":   `{"series":[]}`,
		"missing series": `{"label":"x"}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if out := plain().aiSparklineBlock([]byte(payload), 60); out != "" {
				t.Errorf("%s must degrade to empty string, got %q", name, out)
			}
		})
	}
}

// ---------------------------------------------------------------------
// chart viz=area

func TestAiChartAreaRendersBrailleNeverSolidBlocks(t *testing.T) {
	out := plain().aiChartBlock([]byte(`{"viz":"area","series":[1,4,2,8,5,9,3]}`), 60)
	if out == "" {
		t.Fatal("area chart rendered nothing")
	}
	assertBrailleOnly(t, out)
}

func TestAiChartAreaHasElevenPlotRowsPlusXTicks(t *testing.T) {
	out := plain().aiChartBlock([]byte(`{"viz":"area","series":[1,2,3,4,5]}`), 60)
	lines := strings.Split(out, "\n")
	// No label here: aiChartRows plot lines + 1 x-tick line underneath.
	if len(lines) != aiChartRows+1 {
		t.Fatalf("area chart must render %d plot rows + 1 x-tick row, got %d:\n%s", aiChartRows+1, len(lines), out)
	}
	for i := 0; i < aiChartRows; i++ {
		if n := len([]rune(stripANSI(lines[i]))); n != aiChartCols {
			t.Errorf("plot row %d must be %d cells, got %d: %q", i, aiChartCols, n, lines[i])
		}
	}
}

func TestAiChartAreaYTicksStampCeilingValue(t *testing.T) {
	// A flat 100-value series ceilings at 100; compactCount(100) == "100"
	// and it must land on the TOP plot row (the ceiling tick).
	out := plain().aiChartBlock([]byte(`{"viz":"area","series":[100,100,100]}`), 60)
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[0], "100") {
		t.Errorf("top row must carry the ceiling tick value 100: %q", lines[0])
	}
}

func TestAiChartAreaXTicksDayIndexWhenNoDates(t *testing.T) {
	out := plain().aiChartBlock([]byte(`{"viz":"area","series":[1,2,3,4,5,6,7,8,9]}`), 60)
	lines := strings.Split(out, "\n")
	xline := lines[len(lines)-1]
	if !strings.Contains(xline, "1") || !strings.Contains(xline, "9") {
		t.Errorf("x-ticks must fall back to day-index (1..9) when no dates given: %q", xline)
	}
}

func TestAiChartAreaXTicksUseDatesWhenGiven(t *testing.T) {
	// Pinned now (2026-07-19) matches the fixture dates' year — the house
	// rule's current-year branch: day-first, no leading zero ("1 Jan"),
	// never the old month-first "Jan 1".
	fixedNow := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	r := New(60, WithPlain(), WithNow(func() time.Time { return fixedNow }))
	out := r.aiChartBlock([]byte(`{"viz":"area",
		"series":[1,2,3,4,5],
		"dates":["2026-01-01","2026-01-02","2026-01-03","2026-01-04","2026-01-05"]
	}`), 60)
	lines := strings.Split(out, "\n")
	xline := lines[len(lines)-1]
	if !strings.Contains(xline, "1 Jan") {
		t.Errorf("x-ticks must render real dates day-first in the current year: %q", xline)
	}
	if strings.Contains(xline, "Jan 1") {
		t.Errorf("x-ticks must not render the old month-first style: %q", xline)
	}
}

func TestAiChartAreaXTicksPriorYearRenderMonthOnly(t *testing.T) {
	// A year boundary: the fixture's dates fall in 2025 while "now" is
	// 2026 — the house rule's other-year branch drops the day entirely
	// and leads with month + space + 2-digit year ("Jun '25"), since the
	// 42-cell braille canvas can't fit five day-bearing prior-year ticks.
	fixedNow := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	r := New(60, WithPlain(), WithNow(func() time.Time { return fixedNow }))
	out := r.aiChartBlock([]byte(`{"viz":"area",
		"series":[1,2,3,4,5],
		"dates":["2025-06-01","2025-06-08","2025-06-15","2025-06-22","2025-06-29"]
	}`), 60)
	lines := strings.Split(out, "\n")
	xline := lines[len(lines)-1]
	if !strings.Contains(xline, "Jun '25") {
		t.Errorf("prior-year x-ticks must render month-only (\"Jun '25\", with the space): %q", xline)
	}
	for _, dayBearing := range []string{"1 Jun", "8 Jun", "15 Jun", "22 Jun", "29 Jun"} {
		if strings.Contains(xline, dayBearing) {
			t.Errorf("prior-year x-ticks must never carry a day: %q", xline)
		}
	}
}

func TestAiChartAreaXTicksPercentAxisIgnoresDates(t *testing.T) {
	// x_axis:"percent" is the generic kwarg mirroring Rails' Visualizers::
	// Area#x_axis (:dates | :percent) — analyze.go's avg_viewed_pct stash
	// metric (analyzeStashPreset, analyze.go) is the one caller today, but
	// the JSON field rides on the shared aiAreaData envelope so any @ai
	// chart=area block can opt into the same fixed 0%->100% axis. Real
	// dates are supplied here and must be ignored entirely.
	out := plain().aiChartBlock([]byte(`{"viz":"area",
		"series":[1,2,3,4,5],
		"dates":["2026-01-01","2026-01-02","2026-01-03","2026-01-04","2026-01-05"],
		"x_axis":"percent"
	}`), 60)
	lines := strings.Split(out, "\n")
	xline := lines[len(lines)-1]
	for _, want := range []string{"0%", "25%", "50%", "75%", "100%"} {
		if !strings.Contains(xline, want) {
			t.Errorf("percent x-axis missing tick %q: %q", want, xline)
		}
	}
	if strings.Contains(xline, "Jan") {
		t.Errorf("percent x-axis must ignore dates entirely: %q", xline)
	}
}

func TestAiChartAreaDegrades(t *testing.T) {
	cases := map[string]string{
		"malformed json": `{"viz":"area",`,
		"empty series":   `{"viz":"area","series":[]}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if out := plain().aiChartBlock([]byte(payload), 60); out != "" {
				t.Errorf("%s must degrade to empty string, got %q", name, out)
			}
		})
	}
}

// ---------------------------------------------------------------------
// chart viz=bar

func TestBarCellCountsSumToThePctOfHundredTarget(t *testing.T) {
	cases := []struct {
		name string
		pcts []float64
		cols int
		want int // expected sum of cells
	}{
		{"full breakdown closes the canvas", []float64{50, 30, 20}, 42, 42},
		{"partial group stays honest", []float64{40, 20}, 42, 25}, // round(60% of 42) = 25
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cells := barCellCounts(tc.pcts, tc.cols)
			if got := sumInts(cells); got != tc.want {
				t.Errorf("barCellCounts(%v, %d) sums to %d, want %d (cells=%v)", tc.pcts, tc.cols, got, tc.want, cells)
			}
		})
	}
}

func TestBarCellCountsEveryPositiveBarGetsAtLeastOneCell(t *testing.T) {
	// A near-invisible 0.5% share must still floor to 1 cell even after
	// the group is normalized against a much bigger sibling.
	cells := barCellCounts([]float64{0.5, 99.5}, 42)
	if len(cells) != 2 || cells[0] < 1 {
		t.Fatalf("tiny positive bar must floor to >=1 cell: %v", cells)
	}
	if sumInts(cells) != 42 {
		t.Errorf("cells must still sum to the 100%% target: %v", cells)
	}
	// A zero-pct bar draws nothing.
	cells = barCellCounts([]float64{0, 100}, 42)
	if cells[0] != 0 {
		t.Errorf("a 0%% bar must draw 0 cells, got %d", cells[0])
	}
}

func TestAiChartBarPlotRowsStayWithinTheCanvasWidth(t *testing.T) {
	out := plain().aiChartBlock([]byte(`{"viz":"bar","bars":[
		{"label":"Main","pct":50},
		{"label":"Extras","pct":30},
		{"label":"Other","pct":20}
	]}`), 60)
	lines := strings.Split(out, "\n")
	for i := 0; i < aiChartRows; i++ {
		if n := len([]rune(stripANSI(lines[i]))); n != aiChartCols {
			t.Errorf("bar plot row %d must be %d cells, got %d: %q", i, aiChartCols, n, lines[i])
		}
	}
	if strings.ContainsAny(out, "▁▂▃▄▅▆▇█") {
		t.Errorf("bar chart must use braille ⣿, never solid block runes:\n%s", out)
	}
}

func TestAiChartBarLegendShowsLabelAndValue(t *testing.T) {
	out := plain().aiChartBlock([]byte(`{"viz":"bar","bars":[
		{"label":"Main story","pct":62.5,"value_label":"62.5% done"}
	]}`), 60)
	if !strings.Contains(out, "Main story") || !strings.Contains(out, "62.5% done") {
		t.Errorf("legend must carry the bar's label and value_label:\n%s", out)
	}
}

func TestAiChartBarDefaultsValueLabelToFormattedPct(t *testing.T) {
	out := plain().aiChartBlock([]byte(`{"viz":"bar","bars":[{"label":"X","pct":33}]}`), 60)
	if !strings.Contains(out, "33.0%") {
		t.Errorf("missing value_label must default to \"%%.1f%%%%\" of pct:\n%s", out)
	}
}

func TestAiChartBarCapsAtFiveBars(t *testing.T) {
	payload := `{"viz":"bar","bars":[
		{"label":"A","pct":10},
		{"label":"B","pct":10},
		{"label":"C","pct":10},
		{"label":"D","pct":10},
		{"label":"E","pct":10},
		{"label":"F","pct":10}
	]}`
	out := plain().aiChartBlock([]byte(payload), 60)
	if strings.Contains(out, "F") {
		t.Errorf("a 6th bar must be dropped (max 5):\n%s", out)
	}
	if !strings.Contains(out, "E") {
		t.Errorf("the 5th bar must still render:\n%s", out)
	}
}

// TestAiChartBarAssignsRampColorsByIndex proves aiBarChart assigns
// aiBarRamp's house colors by bar POSITION — the wire never carries a
// per-bar "color" (Ai::Blocks#chart strips it), so a bare {label,pct} bar
// still has to land on green/cyan/blue/… per VizBlockComponent::BAR_RAMP.
func TestAiChartBarAssignsRampColorsByIndex(t *testing.T) {
	out := New(80, WithTruecolor(true)).aiChartBlock([]byte(`{"viz":"bar","bars":[
		{"label":"Mobile","pct":77.5},
		{"label":"Desktop","pct":18.8},
		{"label":"TV","pct":3.7}
	]}`), 60)
	for i, token := range aiBarRamp[:3] {
		bullet := lipgloss.NewStyle().Foreground(hex(barColor(token))).Render("●")
		if !strings.Contains(out, bullet) {
			t.Errorf("bar %d must ride ramp color %q (no per-bar color on the wire):\n%s", i, token, out)
		}
	}
}

// TestAiChartBarRepro feeds the exact wire payload pito served to the
// owner — a bare {label,pct} bar group with no "color" and no "data"
// envelope — straight through the chart path. Before the flat-decode fix
// this degraded to a raw-JSON dump (aiChartPayload's now-removed nested
// "data" field left every wire bar chart undecodable); it must now render
// actual braille bars.
func TestAiChartBarRepro(t *testing.T) {
	payload := `{"type":"chart","viz":"bar","label":"Audience device split (lifetime)","bars":[{"pct":77.5,"label":"Mobile"},{"pct":18.8,"label":"Desktop"},{"pct":3.7,"label":"TV"}]}`
	out := plain().aiChartBlock([]byte(payload), 60)
	if out == "" {
		t.Fatal("bar chart rendered nothing")
	}
	assertBrailleOnly(t, out)
	if !strings.Contains(out, "Audience device split (lifetime)") {
		t.Errorf("label line missing:\n%s", out)
	}
	for _, want := range []string{"Mobile", "Desktop", "TV"} {
		if !strings.Contains(out, want) {
			t.Errorf("legend missing bar label %q:\n%s", want, out)
		}
	}
	// A raw-JSON degrade dump (ai.go's aiDegradeBlock) carries the literal
	// quoted key — a real render never does.
	if strings.Contains(out, `"pct"`) {
		t.Errorf("output looks like a raw-JSON degrade dump, not a rendered chart:\n%s", out)
	}
}

func TestBarColorTokensMapToThemeHexes(t *testing.T) {
	cases := map[string]RGB{
		"red":    aiThemeRed,
		"green":  aiThemeGreen,
		"blue":   aiThemeBlue,
		"purple": aiThemePurple,
		"cyan":   aiThemeCyan,
		"yellow": aiThemeYellow,
		"orange": aiThemeOrange,
	}
	for token, want := range cases {
		if got := barColor(token); got != want {
			t.Errorf("barColor(%q) = %+v, want %+v", token, got, want)
		}
	}
	// Unknown/absent token falls back to blue (Bar::COLOR_TOKENS.fetch default).
	if got := barColor("nonsense"); got != aiThemeBlue {
		t.Errorf("unknown color token must fall back to blue, got %+v", got)
	}
	if got := barColor(""); got != aiThemeBlue {
		t.Errorf("empty color token must fall back to blue, got %+v", got)
	}
}

func TestBarColorPinkIsAFiftyFiftyRedPurpleMix(t *testing.T) {
	red, purple, pink := barColor("red"), barColor("purple"), barColor("pink")
	wantR := uint8((int(red.R) + int(purple.R)) / 2)
	wantG := uint8((int(red.G) + int(purple.G)) / 2)
	wantB := uint8((int(red.B) + int(purple.B)) / 2)
	if pink.R != wantR || pink.G != wantG || pink.B != wantB {
		t.Errorf("pink = %+v, want a 50/50 red+purple mix = {%d %d %d}", pink, wantR, wantG, wantB)
	}
}

func TestAiChartBarDegrades(t *testing.T) {
	cases := map[string]string{
		"malformed json": `{"viz":"bar",`,
		"empty bars":     `{"viz":"bar","bars":[]}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if out := plain().aiChartBlock([]byte(payload), 60); out != "" {
				t.Errorf("%s must degrade to empty string, got %q", name, out)
			}
		})
	}
}

// ---------------------------------------------------------------------
// chart viz=heatmap

func TestHeatColumnWidthsSpreadTheRemainderLeft(t *testing.T) {
	cases := []struct {
		n, cols int
		want    []int
	}{
		{3, 42, []int{14, 14, 14}},
		{5, 42, []int{9, 9, 8, 8, 8}},
		{7, 42, []int{6, 6, 6, 6, 6, 6, 6}},
	}
	for _, tc := range cases {
		got := heatColumnWidths(tc.n, tc.cols)
		if len(got) != len(tc.want) {
			t.Fatalf("heatColumnWidths(%d, %d) = %v, want %v", tc.n, tc.cols, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("heatColumnWidths(%d, %d) = %v, want %v", tc.n, tc.cols, got, tc.want)
				break
			}
		}
		if sumInts(got) != tc.cols {
			t.Errorf("heatColumnWidths(%d, %d) sums to %d, want %d", tc.n, tc.cols, sumInts(got), tc.cols)
		}
	}
}

func TestHeatmapColorsHitTheExtremesAtMinAndMax(t *testing.T) {
	colors := heatmapColors([]float64{0, 10})
	if colors[0] != aiThemeRed {
		t.Errorf("the minimum value must be pure red: %+v", colors[0])
	}
	if colors[1] != aiThemeGreen {
		t.Errorf("the maximum value must be pure green: %+v", colors[1])
	}
}

func TestHeatmapColorsFlatDataIsNeutral(t *testing.T) {
	colors := heatmapColors([]float64{5, 5, 5, 5})
	neutral := mixRGB(aiThemeRed, aiThemeGreen, 0.5)
	for i, c := range colors {
		if c != neutral {
			t.Errorf("flat data must map to the neutral midpoint, column %d = %+v, want %+v", i, c, neutral)
		}
	}
	// All-zero is a flat set too (min==max==0) — must not divide by zero
	// or crash; still neutral.
	zeroColors := heatmapColors([]float64{0, 0, 0})
	for i, c := range zeroColors {
		if c != neutral {
			t.Errorf("all-zero data must map to the neutral midpoint, column %d = %+v", i, c)
		}
	}
}

func TestAiChartHeatmapRendersFullHeightColumnsWithWeekdayPreset(t *testing.T) {
	out := plain().aiChartBlock([]byte(`{"viz":"heatmap","values":[1,2,3,4,5,6,7]}`), 60)
	lines := strings.Split(out, "\n")
	if len(lines) != aiChartRows+1 { // 11 plot rows + 1 label row
		t.Fatalf("heatmap must render %d plot rows + 1 label row, got %d:\n%s", aiChartRows+1, len(lines), out)
	}
	for i := 0; i < aiChartRows; i++ {
		if n := len([]rune(lines[i])); n != aiChartCols {
			t.Errorf("heatmap plot row %d must be %d cells, got %d: %q", i, aiChartCols, n, lines[i])
		}
	}
	labelLine := lines[aiChartRows]
	for _, day := range heatmapWeekdayLabels {
		if !strings.Contains(labelLine, day) {
			t.Errorf("weekday preset label line missing %q: %q", day, labelLine)
		}
	}
	if strings.ContainsAny(out, "▁▂▃▄▅▆▇█") {
		t.Errorf("heatmap must use braille ⣿, never solid block runes:\n%s", out)
	}
}

func TestAiChartHeatmapCustomLabelsRenderOneToOne(t *testing.T) {
	out := plain().aiChartBlock([]byte(`{"viz":"heatmap","values":[3,9,1],"labels":["9am","2pm","9pm"]}`), 60)
	lastLine := strings.Split(out, "\n")[aiChartRows]
	for _, want := range []string{"9am", "2pm", "9pm"} {
		if !strings.Contains(lastLine, want) {
			t.Errorf("custom label %q missing from label row: %q", want, lastLine)
		}
	}
}

func TestAiChartHeatmapDegrades(t *testing.T) {
	cases := map[string]string{
		"malformed json":        `{"viz":"heatmap",`,
		"too few values":        `{"viz":"heatmap","values":[1]}`,
		"too many values":       `{"viz":"heatmap","values":` + repeatFloats(43) + `}`,
		"labels don't pair 1:1": `{"viz":"heatmap","values":[1,2,3],"labels":["a","b"]}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if out := plain().aiChartBlock([]byte(payload), 60); out != "" {
				t.Errorf("%s must degrade to empty string, got %q", name, out)
			}
		})
	}
}

func repeatFloats(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "1"
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// ---------------------------------------------------------------------
// chart viz=heart — delegates to heart.go's heartCanvas (a sibling
// dispatch); this only exercises the decode/degrade contract on this
// side, not the heart glyphs themselves (heart_test.go owns those).

func TestAiChartHeartDelegatesToHeartCanvas(t *testing.T) {
	out := plain().aiChartBlock([]byte(`{"viz":"heart","score":88.8,"likes":14453,"dislikes":1806}`), 60)
	want := plain().heartCanvas(88.8, aiHeartTint, 14453, 1806, "88.80%")
	if out != want {
		t.Errorf("heart chart must delegate verbatim to heartCanvas:\ngot  %q\nwant %q", out, want)
	}
}

func TestAiChartHeartLabelRendersAboveTheCanvas(t *testing.T) {
	out := stripANSI(plain().aiChartBlock([]byte(`{"viz":"heart","label":"How loved is it","score":50,"likes":1,"dislikes":1}`), 60))
	if !strings.HasPrefix(out, "How loved is it\n") {
		t.Errorf("heart chart label must lead the canvas: %q", out)
	}
}

func TestAiChartHeartDegrades(t *testing.T) {
	cases := map[string]string{
		"malformed json": `{"viz":"heart",`,
		"missing score":  `{"viz":"heart","likes":1,"dislikes":1}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if out := plain().aiChartBlock([]byte(payload), 60); out != "" {
				t.Errorf("%s must degrade to empty string, got %q", name, out)
			}
		})
	}
}

// ---------------------------------------------------------------------
// unknown viz / malformed chart envelope

func TestAiChartBlockUnknownVizDegrades(t *testing.T) {
	out := plain().aiChartBlock([]byte(`{"viz":"pie","whatever":1}`), 60)
	if out != "" {
		t.Errorf("an unknown viz must degrade to empty string, got %q", out)
	}
}

func TestAiChartBlockMalformedEnvelopeDegrades(t *testing.T) {
	out := plain().aiChartBlock([]byte(`not json at all`), 60)
	if out != "" {
		t.Errorf("a malformed chart envelope must degrade to empty string, got %q", out)
	}
}

// ---------------------------------------------------------------------
// shared helpers

// assertBrailleOnly fails the test if out carries a rune outside the
// braille block (0x2800-0x28FF) or plain space/newline/digit/letter
// (tick text) — and, per the owner's hard rule, NEVER the old solid
// block runes.
func assertBrailleOnly(t *testing.T, out string) {
	t.Helper()
	if strings.ContainsAny(out, "▁▂▃▄▅▆▇█") {
		t.Errorf("solid block runes must never appear in chart output:\n%s", out)
	}
	braille := false
	for _, ru := range out {
		if ru >= 0x2800 && ru <= 0x28FF {
			braille = true
			break
		}
	}
	if !braille {
		t.Errorf("no braille runes found in chart output:\n%s", out)
	}
}
