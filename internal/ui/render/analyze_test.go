package render

import (
	"os"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ---------------------------------------------------------------------
// bar presentation — label + color parity table (barPresentation),
// verified against the live `breakdowns channel` capture's own rendered
// legend HTML (var(--accent-*) tokens + label text).

func TestBarPresentationMatchesPitoParityTable(t *testing.T) {
	cases := []struct {
		metric, key          string
		index                int
		wantLabel, wantColor string
	}{
		{"subscribed_status", "SUBSCRIBED", 0, "Subscribed", "green"},
		{"subscribed_status", "UNSUBSCRIBED", 1, "Not subscribed", "red"},
		{"devices", "MOBILE", 0, "Mobile", "blue"},
		{"devices", "DESKTOP", 1, "Computer", "purple"},
		{"devices", "TV", 2, "TV", "cyan"},
		{"demographics_gender", "male", 0, "Male", "blue"},
		{"demographics_gender", "female", 1, "Female", "pink"},
		{"demographics_gender", "gender_other", 2, "Other", "purple"},
		{"geography", "US", 0, "United States", "green"},
		{"geography", "ES", 1, "Spain", "cyan"},
		{"geography", "UZ", 2, "Uzbekistan", "blue"},
		{"geography", "GB", 3, "United Kingdom", "purple"},
		{"geography", "OTHER", 4, "Other", "orange"},
		{"demographics_age", "age25-34", 0, "25–34", "cyan"},
		{"demographics_age", "age35-44", 1, "35–44", "blue"},
		{"demographics_age", "age18-24", 2, "18–24", "purple"},
		{"demographics_age", "age13-17", 3, "13–17", "pink"},
	}
	for _, tc := range cases {
		label, color := barPresentation(tc.metric, tc.key, tc.index)
		if label != tc.wantLabel || color != tc.wantColor {
			t.Errorf("barPresentation(%q,%q,%d) = (%q,%q), want (%q,%q)",
				tc.metric, tc.key, tc.index, label, color, tc.wantLabel, tc.wantColor)
		}
	}
}

func TestFormatAgeBracket(t *testing.T) {
	cases := map[string]string{
		"age25-34": "25–34",
		"age13-17": "13–17",
		"age65-":   "65+",
	}
	for in, want := range cases {
		if got := formatAgeBracket(in); got != want {
			t.Errorf("formatAgeBracket(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCountryNameResolvesIsoCodes(t *testing.T) {
	cases := map[string]string{
		"US": "United States",
		"ES": "Spain",
		"UZ": "Uzbekistan",
		"GB": "United Kingdom",
	}
	for code, want := range cases {
		if got := countryName(code); got != want {
			t.Errorf("countryName(%q) = %q, want %q", code, got, want)
		}
	}
	if got := countryName("ZZ_NOT_A_CODE"); got != "ZZ_NOT_A_CODE" {
		t.Errorf("an unrecognized code must pass through verbatim, got %q", got)
	}
}

// ---------------------------------------------------------------------
// spark / analyzeStashPreset — parity fix: every stash scalar metric
// (views/watched_hours/subs/avg_view_duration/avg_viewed_pct/…) now draws
// through the SAME 42x11 ticked area engine as the D8 breakdowns extras
// (ai_charts.go's areaChart), matching analyze_cell_component.html.erb's
// `chart?` branch — which routes EVERY stash chart cell through
// Pito::Analytics::Visualizers::Area, never the bare 2-row Sparkline
// (that visualizer is reserved for `glance`'s Slots::Compact). Before
// this pass spark() drove BrailleArea directly at ROWS=2 with no ticks at
// all — a regression back to that shape wouldn't have failed any of the
// old Contains()-only assertions in render_test.go, hence the explicit
// row-count/tick-value pins below.

func TestAnalyzeStashPresetMatchesRailsAreaVisualizer(t *testing.T) {
	// Ports Visualizers::Area#preset_value_format / #preset_x_axis
	// (pito's app/components/pito/analytics/visualizers/area.rb) metric
	// by metric: avg_view_duration and avg_viewed_pct are the ONLY two
	// stash metrics with a non-default preset in Rails; every other key —
	// including ones the server might add later — falls through to the
	// `else` branch both Rails methods share (count / dates-or-day-index).
	cases := []struct {
		name                  string
		wantFormat, wantXAxis string
	}{
		{"avg_view_duration", "duration", ""},
		{"avg_viewed_pct", "percent", areaXAxisPercent},
		{"views", "count", ""},
		{"watched_hours", "count", ""},
		{"subs", "count", ""},
		{"ctr", "count", ""},
		{"some_future_metric", "count", ""},
	}
	for _, tc := range cases {
		format, xAxis := analyzeStashPreset(tc.name)
		if format != tc.wantFormat || xAxis != tc.wantXAxis {
			t.Errorf("analyzeStashPreset(%q) = (%q,%q), want (%q,%q)",
				tc.name, format, xAxis, tc.wantFormat, tc.wantXAxis)
		}
	}
}

func TestSparkRendersTheFullTickedAreaChart(t *testing.T) {
	// Pinned now (2026-07-19) matches the fixture dates' year — the "29 Jun"
	// assertion below rides the house rule's current-year branch and would
	// flip to "Jun '26" the day the real clock crossed into 2027.
	fixedNow := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	r := New(60, WithPlain(), WithNow(func() time.Time { return fixedNow }))
	prev := 37.0
	out := stripANSI(r.spark("views", analyzeSeries{
		Dates:       []string{"2026-06-29", "2026-06-30", "2026-07-01", "2026-07-02", "2026-07-03"},
		Series:      []float64{5, 12, 3, 1, 0},
		Total:       21,
		Previous:    &prev,
		TargetDaily: 322.42857142857144,
	}, 60))
	lines := strings.Split(out, "\n")
	// 11 plot rows + 1 x-tick row + 1 meta row — the old bare sparkline
	// was 2 plot rows + 1 meta row with no x-tick line at all.
	if len(lines) != aiChartRows+2 {
		t.Fatalf("stash chart must render %d rows (11 plot + x-ticks + meta), got %d:\n%s",
			aiChartRows+2, len(lines), out)
	}
	if !strings.Contains(lines[0], "322") {
		t.Errorf("top row must carry the ceiling y-tick (compactCount of target_daily 322.43): %q", lines[0])
	}
	if !strings.Contains(lines[aiChartRows], "29 Jun") {
		t.Errorf("x-tick row must render day-first dates (web's Area#format_date \"%%-d %%b\"): %q", lines[aiChartRows])
	}
	if got, want := lines[aiChartRows+1], "total 21 · prev 37 · target 322.43/d"; got != want {
		t.Errorf("meta caption line (a TUI-only addition over the web) must be unchanged by this upgrade: got %q, want %q", got, want)
	}
}

func TestSparkAvgViewDurationYTicksRenderMSS(t *testing.T) {
	out := stripANSI(plain().spark("avg_view_duration", analyzeSeries{
		Series:      []float64{90, 120, 60},
		TargetDaily: 120,
	}, 60))
	// ceiling = max(series.max=120, target=120) = 120s → top tick "2:00".
	if !strings.Contains(out, "2:00") {
		t.Errorf("avg_view_duration's y-tick must be M:SS-formatted (ceiling 120s -> \"2:00\"):\n%s", out)
	}
}

func TestSparkAvgViewedPctUsesFixedPercentXAxisRegardlessOfDates(t *testing.T) {
	// Study-Rails-first finding: Visualizers::Area#preset_x_axis checks
	// `@metric == :avg_viewed_pct` BEFORE ever looking at whether dates
	// were supplied — avg_viewed_pct is the web's one area chart that
	// always plots a fixed 0%->100% POSITION axis (spec: "returns the
	// retention 0%->100% labels regardless of dates", area_spec.rb). The
	// payload below carries real dates; they must be ignored entirely.
	out := stripANSI(plain().spark("avg_viewed_pct", analyzeSeries{
		Dates:       []string{"2026-06-29", "2026-06-30", "2026-07-01"},
		Series:      []float64{50.8, 25.1, 1.8},
		TargetDaily: 50.0,
	}, 60))
	lines := strings.Split(out, "\n")
	xline := lines[aiChartRows]
	for _, want := range []string{"0%", "25%", "50%", "75%", "100%"} {
		if !strings.Contains(xline, want) {
			t.Errorf("percent x-axis missing tick %q: %q", want, xline)
		}
	}
	if strings.Contains(xline, "Jun") || strings.Contains(xline, "Jul") {
		t.Errorf("avg_viewed_pct's x-axis must ignore dates entirely, calendar month leaked: %q", xline)
	}
	if !strings.Contains(lines[0], "50.80%") {
		t.Errorf("top y-tick must be percent-formatted (XX.XX%%), not the raw \"50.8\": %q", lines[0])
	}
}

// ---------------------------------------------------------------------
// analyzeBarBlock / analyzeHeatmapBlock / analyzeAreaBlock — the shared
// AI-chart-family engines (ai_charts.go), driven off the breakdowns
// payload's own JSON shapes.

func TestAnalyzeBarBlockRendersLabelAndPctLegend(t *testing.T) {
	out := plain().analyzeBarBlock("subscribed_status", []analyzeBarRow{
		{Key: "UNSUBSCRIBED", Pct: 96.1},
		{Key: "SUBSCRIBED", Pct: 3.9},
	}, 60)
	for _, want := range []string{"Not subscribed 96.1%", "Subscribed 3.9%"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	assertBrailleOnly(t, out)
}

func TestAnalyzeBarBlockPayloadColorOverridesTheFallbackTable(t *testing.T) {
	// If a bar row ever carries its own `color` token, it wins over the
	// presentation table's default (subscribed_status.SUBSCRIBED → green
	// normally) — verified via the rendered legend's bullet color, which
	// only shows a token difference in the underlying RGB, so we assert
	// through analyzeBarBlock not crashing/misdecoding an explicit token.
	rows := []analyzeBarRow{{Key: "SUBSCRIBED", Pct: 50, Color: "orange"}}
	out := plain().analyzeBarBlock("subscribed_status", rows, 60)
	if !strings.Contains(out, "Subscribed 50.0%") {
		t.Errorf("label still comes from the table even with a payload color override:\n%s", out)
	}
}

func TestAnalyzeBarBlockEmptyRowsDegradesToNoData(t *testing.T) {
	if out := plain().analyzeBarBlock("devices", nil, 60); out != "" {
		t.Errorf("no rows must degrade to \"\" (pending/no-data), got %q", out)
	}
}

func TestAnalyzeHeatmapBlockUsesWeekdayPreset(t *testing.T) {
	out := plain().analyzeHeatmapBlock(&analyzeHeatmapData{
		Values: []float64{60.6, 100.5, 178.6, 70.2, 33.4, 40.3, 94.3},
	}, 60)
	for _, day := range heatmapWeekdayLabels {
		if !strings.Contains(out, day) {
			t.Errorf("missing weekday label %q:\n%s", day, out)
		}
	}
	assertBrailleOnly(t, out)
}

func TestAnalyzeAreaBlockRetentionFallsBackToDayIndexTicks(t *testing.T) {
	// retention never carries `dates` — its x-ticks must fall back to
	// day-index (1..N), same as any @ai chart=area block with no dates.
	d := &analyzeAreaData{Series: []float64{10, 20, 30, 40, 50}}
	out := plain().analyzeAreaBlock(d, 60)
	lines := strings.Split(out, "\n")
	xline := lines[len(lines)-1]
	if !strings.Contains(xline, "1") || !strings.Contains(xline, "5") {
		t.Errorf("day-index x-ticks (1..5) missing: %q", xline)
	}
}

func TestAnalyzeAreaBlockCommentsRendersDayFirstDateTicksInTheCurrentYear(t *testing.T) {
	// Pinned now (2026-07-19) matches the fixture dates' year — the house
	// rule's current-year branch: day-first, no leading zero, abbreviated
	// month ("10 Mar"-style).
	fixedNow := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	r := New(60, WithPlain(), WithNow(func() time.Time { return fixedNow }))
	d := &analyzeAreaData{
		Series: []float64{1, 2, 3, 4, 5},
		Dates:  []string{"2026-03-10", "2026-03-11", "2026-03-12", "2026-03-13", "2026-03-14"},
	}
	out := r.analyzeAreaBlock(d, 60)
	lines := strings.Split(out, "\n")
	xline := lines[len(lines)-1]
	if !strings.Contains(xline, "10 Mar") {
		t.Errorf("comments dates must render day-first (\"10 Mar\"-style) in the current year: %q", xline)
	}
}

func TestAnalyzeAreaBlockCommentsPriorYearRendersMonthOnlyTicks(t *testing.T) {
	// A year boundary: the fixture's dates fall in 2025 while "now" is
	// 2026 — the house rule's other-year branch drops the day entirely
	// and leads with month + space + 2-digit year ("Mar '25"), since the
	// 42-cell canvas can't fit five day-bearing prior-year ticks.
	fixedNow := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	r := New(60, WithPlain(), WithNow(func() time.Time { return fixedNow }))
	d := &analyzeAreaData{
		Series: []float64{1, 2, 3, 4, 5},
		Dates:  []string{"2025-03-10", "2025-03-11", "2025-03-12", "2025-03-13", "2025-03-14"},
	}
	out := r.analyzeAreaBlock(d, 60)
	lines := strings.Split(out, "\n")
	xline := lines[len(lines)-1]
	if !strings.Contains(xline, "Mar '25") {
		t.Errorf("prior-year comments x-ticks must render month-only (\"Mar '25\", with the space): %q", xline)
	}
	if strings.Contains(xline, "10 Mar") {
		t.Errorf("prior-year x-ticks must never carry a day: %q", xline)
	}
}

func TestAnalyzeAreaBlockEmptySeriesDegradesToNoData(t *testing.T) {
	if out := plain().analyzeAreaBlock(nil, 60); out != "" {
		t.Errorf("a nil breakdown must degrade to \"\" (pending/no-data), got %q", out)
	}
	if out := plain().analyzeAreaBlock(&analyzeAreaData{}, 60); out != "" {
		t.Errorf("an empty series must degrade to \"\" (pending/no-data), got %q", out)
	}
}

// ---------------------------------------------------------------------
// analyzeHeartsRow — 1–2 braille hearts (heart.go's heartCanvas), side
// by side with a 3-cell gutter, mirroring however many the payload carries.

func TestAnalyzeHeartsRowSingleHeartDelegatesVerbatim(t *testing.T) {
	out := plain().analyzeHeartsRow([]heartEntry{
		{Color: "purple", Score: 88.8, Likes: 14453, Dislikes: 1828},
	}, 60)
	want := plain().heartCanvas(88.8, aiThemePurple, 14453, 1828, "88.80%")
	if out != want {
		t.Errorf("a single heart must delegate verbatim to heartCanvas:\ngot  %q\nwant %q", out, want)
	}
}

func TestAnalyzeHeartsRowTwoHeartsJoinSideBySideWhenTheyFit(t *testing.T) {
	// The pair's widest line is its LEGEND text (~69 cells combined,
	// wider than the 2*17+3 canvas alone), so the fit check needs a
	// generous width — 80 comfortably fits, 60 (asserted below) doesn't.
	out := plain().analyzeHeartsRow([]heartEntry{
		{Color: "red", Score: 100, Likes: 3, Dislikes: 0},
		{Color: "purple", Score: 88.8, Likes: 14423, Dislikes: 1828},
	}, 80)
	lines := strings.Split(out, "\n")
	if n := len([]rune(lines[0])); n <= heartCols {
		t.Errorf("two hearts must join side by side (row width > one heart's %d cells), got %d: %q", heartCols, n, lines[0])
	}
	for _, want := range []string{"3 Likes / 0 Dislikes (100.00%)", "14423 Likes / 1828 Dislikes (88.80%)"} {
		if !strings.Contains(out, want) {
			t.Errorf("joined row must still carry both legends intact: missing %q\n%s", want, out)
		}
	}
}

func TestAnalyzeHeartsRowStacksVerticallyWhenTooNarrow(t *testing.T) {
	// Same pair as above, but the two legend lines combined (~69 cells)
	// overflow a 60-cell width — must stack instead of overflowing.
	out := plain().analyzeHeartsRow([]heartEntry{
		{Color: "red", Score: 100, Likes: 3, Dislikes: 0},
		{Color: "purple", Score: 88.8, Likes: 14423, Dislikes: 1828},
	}, 60)
	lines := strings.Split(out, "\n")
	if n := len([]rune(lines[0])); n > heartCols {
		t.Errorf("hearts must stack vertically when the row won't fit, got row width %d > %d: %q", n, heartCols, lines[0])
	}
	for _, want := range []string{"3 Likes / 0 Dislikes (100.00%)", "14423 Likes / 1828 Dislikes (88.80%)"} {
		if !strings.Contains(out, want) {
			t.Errorf("stacked hearts must still carry both legends intact: missing %q\n%s", want, out)
		}
	}
}

// ---------------------------------------------------------------------
// end to end — the real `breakdowns channel` capture.

func TestAnalyzeBreakdownsChannelRendersEveryExtra(t *testing.T) {
	raw, err := os.ReadFile("testdata/breakdowns_channel.json")
	if err != nil {
		t.Fatal(err)
	}
	// Pinned now (2026-07-19) matches the fixture dates' year — the "10 Mar"
	// assertion below rides the house rule's current-year branch and would
	// flip to "Mar '26" the day the real clock crossed into 2027.
	fixedNow := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	r := New(60, WithPlain(), WithNow(func() time.Time { return fixedNow }))
	out := stripANSI(r.Event(event("enhanced", string(raw))))
	assertBrailleOnly(t, out)

	for _, want := range []string{
		// bar groups + legends: "● label pct%"
		"Not subscribed 96.1%", "Subscribed 3.9%", // subscribed_status
		"Mobile 77.5%", "Computer 18.7%", "TV 3.7%", // devices
		"United States 34.9%", "Spain 32.7%", "Uzbekistan 10.9%", "United Kingdom 5.7%", "Other 15.8%", // geography
		"25–34 58.3%", "35–44 20.6%", "18–24 15.7%", "13–17 5.4%", // demographics_age
		"Male 95.4%", "Female 4.6%", // demographics_gender
		// retention + comments captions and comments' day-first tick
		"mean retention", "below average", // retention caption
		"settled at 5", // comments caption
		"10 Mar",       // comments' first date, day-first x-tick
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	// weekday heatmap columns
	for _, day := range heatmapWeekdayLabels {
		if !strings.Contains(out, day) {
			t.Errorf("missing weekday heatmap label %q:\n%s", day, out)
		}
	}
}

func TestAnalyzeBreakdownsChannelOrderFollowsMetricKeys(t *testing.T) {
	raw, err := os.ReadFile("testdata/breakdowns_channel.json")
	if err != nil {
		t.Fatal(err)
	}
	out := stripANSI(plain().Event(event("enhanced", string(raw))))

	// metric_keys (as served): day_of_week_heatmap, subscribed_status,
	// devices, geography, demographics_age, demographics_gender,
	// retention, comments.
	markers := []string{
		"Wednesday",      // day_of_week_heatmap caption subject
		"Not subscribed", // subscribed_status
		"Mobile",         // devices
		"United States",  // geography
		"25–34",          // demographics_age
		"Male",           // demographics_gender
		"below average",  // retention
		"settled at 5",   // comments
	}
	prev := -1
	for _, m := range markers {
		i := strings.Index(out, m)
		if i < 0 {
			t.Fatalf("marker %q not found in output", m)
		}
		if i <= prev {
			t.Errorf("marker %q out of metric_keys order (index %d <= previous %d)", m, i, prev)
		}
		prev = i
	}
}

// Wide terminals lay analyze metric units two-up (owner 2026-07-12: a
// single column wasted half the screen); narrow ones stay stacked.
func TestAnalyzeChartsTwoUpWhenWide(t *testing.T) {
	raw, err := os.ReadFile("testdata/analyze_vid.json")
	if err != nil {
		t.Fatal(err)
	}
	wide := New(140, WithPlain()).Event(event("system", string(raw)))
	twoUp := false
	for _, line := range strings.Split(wide, "\n") {
		if lipgloss.Width(ansi.Strip(line)) > 70 {
			twoUp = true
			break
		}
	}
	if !twoUp {
		t.Errorf("wide analyze must lay charts two-up:\n%s", ansi.Strip(wide))
	}
	// Narrow render keeps the single column: no visible line reaches
	// past the one-cell width.
	narrow := plain().Event(event("system", string(raw)))
	for _, line := range strings.Split(narrow, "\n") {
		if w := lipgloss.Width(ansi.Strip(line)); w > 66 {
			t.Errorf("narrow analyze line too wide (%d): %q", w, ansi.Strip(line))
		}
	}
}

// ---------------------------------------------------------------------
// plain payloads (no breakdowns extras) — likes-heart upgrade only.// ---------------------------------------------------------------------
// plain payloads (no breakdowns extras) — likes-heart upgrade only.

func TestAnalyzeVidLikesRendersTwoBrailleHeartsForVidAndChannel(t *testing.T) {
	raw, err := os.ReadFile("testdata/analyze_vid.json")
	if err != nil {
		t.Fatal(err)
	}
	out := plain().Event(event("system", string(raw)))
	// analyze_vid.json's likes slot carries 2 hearts: {color:red} (the
	// vid itself) + {color:purple} (its channel) — both must render as
	// full braille heart canvases (heart.go's heartCanvas), not the old
	// "♥ NN.N%" text line.
	for _, want := range []string{
		"3 Likes / 0 Dislikes (100.00%)",
		"14423 Likes / 1828 Dislikes (88.80%)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing heart legend %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "♥") {
		t.Errorf("the old text-heart glyph must be gone, replaced by the braille canvas:\n%s", out)
	}
}
