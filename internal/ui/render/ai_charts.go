package render

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// The AI chart-family painters (@ai `chart`/`sparkline` blocks, pito
// 2.0.0). Every renderer decodes its own private struct out of the
// block's raw JSON and is nil-safe: malformed or unrenderable data
// returns "" so the caller (ai_blocks.go's degrade line) can fall back,
// never a panic. Braille plotting reuses braille.go's BrailleArea /
// paintBraille verbatim — this file only decides WHAT series/colors
// feed the shared engine, mirroring pito's own split between
// Visualizers::Base (the canvas) and its concrete subclasses (the data).

// aiChartCols / aiChartRows are the AI chart canvas dims — the Go
// mirror of Pito::Analytics::Visualizers::Base::COLS/ROWS (a vid
// thumbnail's width × a 16:9 box), shared by every full-size viz
// (area/bar/heatmap) and the heart canvas next door.
const (
	aiChartCols = 42
	aiChartRows = 11
)

// ---------------------------------------------------------------------
// sparkline — a bare 2-row braille strip, no ticks/axis/gradient.

type aiSparklineData struct {
	Series    []float64 `json:"series"`
	Label     string    `json:"label"`
	SeriesMax float64   `json:"series_max"`
}

// aiSparklineBlock renders {series, label?, series_max?} as the web's
// Analytics::Visualizers::Sparkline: 2 braille rows, flat fg-default
// fill under the shared pito-blue sweep — no health gradient (unlike
// the full area chart, this is shape-over-detail only). series_max
// (when given) fixes the scale's top; BrailleArea derives it from the
// series peak otherwise.
func (r *R) aiSparklineBlock(raw json.RawMessage, width int) string {
	var p aiSparklineData
	if json.Unmarshal(raw, &p) != nil || len(p.Series) == 0 {
		return ""
	}
	cellW := aiChartCols
	if width > 0 && width < cellW {
		cellW = width
	}
	rows := BrailleArea(p.Series, cellW, 2, p.SeriesMax)
	lines := r.paintBraille(rows, cellW, false, 1.0)
	if p.Label != "" {
		return r.dim(p.Label) + "\n" + strings.Join(lines, "\n")
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------
// chart — dispatches on viz: area | bar | heatmap | heart.

type aiChartPayload struct {
	Viz   string `json:"viz"`
	Label string `json:"label"`
}

// aiChartBlock decodes {viz, label?, ...} and hands the WHOLE block's raw
// bytes off to the matching viz renderer, which decodes its own subset of
// fields straight off the top level. There is no nested "data" envelope on
// the wire: Ai::Blocks#chart (lib/ai/blocks.rb) flattens the model's
// {viz, data: {...}} shape into {viz, <viz's own fields>, label?} before
// persisting, and EventJson ships that payload verbatim (lib/pito/stream/
// event_json.rb) — so this client reads flat too. An unknown or missing
// viz — and a malformed envelope — degrade to "" (nil-safe, per the block
// contract); the caller's raw-JSON degrade line is what the owner actually
// sees.
func (r *R) aiChartBlock(raw json.RawMessage, width int) string {
	var p aiChartPayload
	if json.Unmarshal(raw, &p) != nil {
		return ""
	}
	switch p.Viz {
	case "area":
		return r.aiAreaChart(p.Label, raw, width)
	case "bar":
		return r.aiBarChart(p.Label, raw, width)
	case "heatmap":
		return r.aiHeatmapChart(p.Label, raw, width)
	case "heart":
		return r.aiHeartChart(p.Label, raw, width)
	default:
		return ""
	}
}

// ---------------------------------------------------------------------
// viz=area — the full COLS×ROWS braille chart with y/x tick values.

type aiAreaData struct {
	Series []float64 `json:"series"`
	Dates  []string  `json:"dates"`
	Target float64   `json:"target"`
	Format string    `json:"format"` // count | duration | percent
	XAxis  string    `json:"x_axis"` // "" (dates/day-index, default) | "percent"
}

// areaXAxisPercent is aiAreaData.XAxis's one non-default value — the
// web's fixed 0%→100% POSITION x-axis (Visualizers::Area#preset_x_axis'
// `:percent` branch), used instead of dates/day-index regardless of
// whether the payload carries dates. analyze.go's avg_viewed_pct stash
// metric is the one caller today; the JSON field rides on the generic
// aiAreaData envelope so any @ai chart=area block can opt in too, per
// Rails' own generic `x_axis:` kwarg on Visualizers::Area.
const areaXAxisPercent = "percent"

// aiAreaChart renders a chart=area block's {series, dates?, target?,
// format?, x_axis?} — flat on the block itself, no "data" envelope — on
// the shared 42×11 braille canvas (BrailleArea/paintBraille — the same
// engine analyze.go's areaChart-backed callers drive too: the D8
// breakdowns extras AND every stash scalar metric via spark()), plus the
// web's discrete tick VALUES: ~3 y-ticks stamped inside-left onto their
// data-height row (no axis line, per pito's locked spec) and ~5 x-ticks
// below (real dates when given, day-index otherwise, or the fixed
// 0%→100% position axis when XAxis == areaXAxisPercent).
func (r *R) aiAreaChart(label string, raw json.RawMessage, width int) string {
	var d aiAreaData
	if json.Unmarshal(raw, &d) != nil || len(d.Series) == 0 {
		return ""
	}
	return r.areaChart(label, d, width)
}

// areaChart is aiAreaChart's core, split out so analyze.go's callers —
// the D8 breakdowns retention/comments charts and the stash's scalar
// metrics (spark()), each decoding their own JSON shapes rather than the
// generic aiAreaData envelope — can drive the exact same braille+ticks
// engine instead of duplicating it. Every caller ticks its dates through
// the SAME house rule (formatDateTick, keyed off r.now()) — there is no
// more per-caller layout override; the generic @ai chart=area block and
// every analyze surface (breakdowns' retention/comments, every stash
// scalar via spark()) render identically, matching the web's own single
// Visualizers::Area component underneath all of them.
func (r *R) areaChart(label string, d aiAreaData, width int) string {
	cellW := aiChartCols
	if width > 0 && width < cellW {
		cellW = width
	}
	ceiling := d.Target
	for _, v := range d.Series {
		if v > ceiling {
			ceiling = v
		}
	}
	if ceiling <= 0 {
		ceiling = 1
	}
	// The web's data-driven green anchor: target/ceiling (Thresholds.
	// green_anchor_fraction) — no target ⇒ 0 ⇒ all green.
	anchor := 0.0
	if d.Target > 0 {
		anchor = d.Target / ceiling
	}
	rows := BrailleArea(d.Series, cellW, aiChartRows, ceiling)
	overlayYTicks(rows, ceiling, d.Format)
	lines := r.paintBraille(rows, cellW, false, anchor)

	var b strings.Builder
	if label != "" {
		b.WriteString(r.dim(label) + "\n")
	}
	b.WriteString(strings.Join(lines, "\n"))
	if xline := layoutXTickLine(areaXTicks(d, r.now()), cellW); xline != "" {
		b.WriteString("\n" + r.dim(xline))
	}
	return b.String()
}

// overlayYTicks stamps the ceiling / 66% / 33% value labels onto the
// LEFT edge of their data-height row, mutating rows in place. The
// stamped runes are ordinary text, not braille dots — paintBraille
// treats any non-blank rune as "data" and rides it on the same
// chart-blue sweep as the plot around it, so no separate color path is
// needed here (the "y inside-left" ticks, per Visualizers::Base).
func overlayYTicks(rows []string, ceiling float64, format string) {
	n := len(rows)
	if n == 0 || ceiling <= 0 {
		return
	}
	for _, frac := range []float64{1, 0.66, 0.33} {
		topPct := (1 - frac) * 100
		ri := int(topPct / 100 * float64(n))
		if ri >= n {
			ri = n - 1
		}
		if ri < 0 {
			ri = 0
		}
		stampLabel(rows, ri, formatTickValue(ceiling*frac, format))
	}
}

// stampLabel overwrites row ri's leading runes with label, rune for
// rune — the canvas stays exactly cellW cells wide since one braille
// glyph and one ASCII digit are both a single terminal cell.
func stampLabel(rows []string, ri int, label string) {
	if ri < 0 || ri >= len(rows) {
		return
	}
	runes := []rune(rows[ri])
	for i, lr := range []rune(label) {
		if i >= len(runes) {
			break
		}
		runes[i] = lr
	}
	rows[ri] = string(runes)
}

// formatTickValue is the compact tick-value formatter, per value_format
// (Pito::Analytics::Visualizers::Area#fmt): duration → M:SS, percent →
// "XX.XX%", count (default) → compact "12K" form.
func formatTickValue(v float64, format string) string {
	switch format {
	case "duration":
		return formatDuration(v)
	case "percent":
		return fmt.Sprintf("%.2f%%", v)
	default:
		return compactCount(v)
	}
}

// compactCount mirrors Area#compact_count: 12_300 → "12K", sub-1000
// (including negatives, which never scale) render verbatim.
func compactCount(v float64) string {
	n := int(math.Round(v))
	if n < 1000 {
		return strconv.Itoa(n)
	}
	k := float64(n) / 1000.0
	if k >= 10 {
		return strconv.Itoa(int(math.Round(k))) + "K"
	}
	return trimFloat(math.Round(k*10)/10) + "K"
}

// formatDuration mirrors Pito::Formatter::Duration: "M:SS", growing to
// "H:MM:SS" / "D:HH:MM:SS" as the value crosses each boundary.
func formatDuration(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	secs := int64(seconds)
	days := secs / 86400
	hours := (secs % 86400) / 3600
	minutes := (secs % 3600) / 60
	s := secs % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%d:%02d:%02d:%02d", days, hours, minutes, s)
	case hours > 0:
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, s)
	default:
		return fmt.Sprintf("%d:%02d", minutes, s)
	}
}

// areaXTick is one x-axis tick: its fractional position across the
// plot (0..1) and its label text.
type areaXTick struct {
	frac  float64
	label string
}

// areaXTicks mirrors Area#x_ticks: the fixed 0%→100% position axis when
// XAxis == areaXAxisPercent — checked FIRST, exactly like Rails'
// `@x_axis == :percent` early return, so it wins regardless of series
// length or whether dates were supplied — otherwise ~5 evenly-spaced
// ticks (deduped by data index on short series), real dates (rendered by
// formatDateTick, year-aware against now) when `dates` pairs 1:1 with
// `series`, day-index (1-based) otherwise.
func areaXTicks(d aiAreaData, now time.Time) []areaXTick {
	if d.XAxis == areaXAxisPercent {
		return []areaXTick{
			{frac: 0, label: "0%"},
			{frac: 0.25, label: "25%"},
			{frac: 0.5, label: "50%"},
			{frac: 0.75, label: "75%"},
			{frac: 1, label: "100%"},
		}
	}
	n := len(d.Series)
	if n == 0 {
		return nil
	}
	fracs := []float64{0, 0.25, 0.5, 0.75, 1}
	if n == 1 {
		fracs = []float64{0}
	}
	haveDates := len(d.Dates) == n
	seen := map[int]bool{}
	var ticks []areaXTick
	for _, f := range fracs {
		i := int(math.Round(f * float64(n-1)))
		if seen[i] {
			continue
		}
		seen[i] = true
		label := strconv.Itoa(i + 1)
		if haveDates {
			label = formatDateTick(d.Dates[i], now)
		}
		ticks = append(ticks, areaXTick{frac: f, label: label})
	}
	return ticks
}

// formatDateTick renders one ISO date as a compact, year-aware x-axis
// tick — the house rule (owner 2026-07-19), shared by every dated area
// chart alike: the generic @ai chart=area block AND every analyze
// surface (breakdowns' retention/comments, every stash scalar metric via
// spark()) — no more per-caller layout. Mirrors stamp()'s own day-aware
// year-elision (render.go's sameYear): current year renders day-first,
// no leading zero, abbreviated month ("2 Jan" — e.g. "24 Feb"); other
// years drop the day entirely and lead with the month instead ("Jan '06"
// — WITH the space before the quote, e.g. "Jun '25") since the 42-cell
// braille canvas can't fit five day-bearing prior-year ticks the way
// stamp()'s single inline timestamp can. Unparseable strings pass
// through verbatim rather than vanishing.
func formatDateTick(s string, now time.Time) string {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		if t, err = time.Parse(time.RFC3339, s); err != nil {
			return s
		}
	}
	if sameYear(t.Year(), now) {
		return t.Format("2 Jan")
	}
	return t.Format("Jan '06")
}

// layoutXTickLine spreads ticks across a cols-wide blank line at their
// fractional position — the terminal cousin of the web's absolutely
// positioned x-tick spans. The first/last ticks anchor to their edge so
// nothing clips off the canvas.
func layoutXTickLine(ticks []areaXTick, cols int) string {
	if cols < 1 || len(ticks) == 0 {
		return ""
	}
	row := make([]rune, cols)
	for i := range row {
		row[i] = ' '
	}
	for i, tk := range ticks {
		label := []rune(tk.label)
		anchor := int(math.Round(tk.frac * float64(cols-1)))
		start := anchor - len(label)/2
		switch {
		case i == 0:
			start = anchor
		case i == len(ticks)-1:
			start = anchor - len(label) + 1
		}
		if start < 0 {
			start = 0
		}
		if start+len(label) > cols {
			start = cols - len(label)
		}
		if start < 0 {
			start = 0
		}
		for j, ch := range label {
			if p := start + j; p >= 0 && p < cols {
				row[p] = ch
			}
		}
	}
	return strings.TrimRight(string(row), " ")
}

// ---------------------------------------------------------------------
// viz=bar — 1..5 horizontal bar-group, braille ⣿ rows.

// aiBar is the shared bar shape both aiBarChart (decoded off the wire,
// which never carries "color" — see aiBarRamp) and analyze.go's
// breakdowns (built programmatically, Color set from barPresentation's own
// token table) feed into the shared barChart/barRow engine below.
type aiBar struct {
	Label      string  `json:"label"`
	Pct        float64 `json:"pct"`
	Color      string  `json:"color"`
	ValueLabel *string `json:"value_label"` // nil = default "%.1f%%"
}

type aiBarsData struct {
	Bars []aiBar `json:"bars"`
}

// The pito default theme's (tokyo-night, app/assets/tailwind/themes.css
// :root) accent hexes — the SAME values Visualizers::Bar::COLOR_TOKENS
// maps its color symbols onto, and Heatmap's --pito-heat ramp samples
// between (--color-green/--color-red alias accent-green/accent-red).
var (
	aiThemeRed    = RGB{0xf7, 0x76, 0x8e}
	aiThemeGreen  = RGB{0x9e, 0xce, 0x6a}
	aiThemeBlue   = RGB{0x51, 0x70, 0xff} // --brand-pito
	aiThemePurple = RGB{0xbb, 0x9a, 0xf7}
	aiThemeCyan   = RGB{0x7d, 0xcf, 0xff}
	aiThemeYellow = RGB{0xe0, 0xaf, 0x68}
	aiThemeOrange = RGB{0xff, 0x9e, 0x64}
)

// barColor maps a bar's color token onto its theme RGB — mirrors
// Visualizers::Bar::COLOR_TOKENS exactly, including the fallback to
// blue for an unknown/absent token. "pink" has no accent token of its
// own on the web either: it's synthesised as a 50/50 red+purple mix
// (there color-mix in oklch; here a plain component-wise RGB lerp).
func barColor(token string) RGB {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "red":
		return aiThemeRed
	case "green":
		return aiThemeGreen
	case "blue":
		return aiThemeBlue
	case "purple":
		return aiThemePurple
	case "cyan":
		return aiThemeCyan
	case "yellow":
		return aiThemeYellow
	case "orange":
		return aiThemeOrange
	case "pink":
		return mixRGB(aiThemeRed, aiThemePurple, 0.5)
	default:
		return aiThemeBlue
	}
}

// mixRGB component-wise lerps two colors at t ∈ [0,1].
func mixRGB(a, b RGB, t float64) RGB {
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return RGB{lerp(a.R, b.R), lerp(a.G, b.G), lerp(a.B, b.B)}
}

// aiBarRamp is the house bucket ramp for @ai chart=bar blocks — mirrors
// VizBlockComponent::BAR_RAMP (app/components/pito/event/ai/
// viz_block_component.rb) exactly: "the model never picks colors (style
// stays in the app)", so Ai::Blocks#chart's bar branch never puts a
// "color" key on a wire bar in the first place — every bucket gets its
// hue assigned HERE, by its position in the group, cycling every 5.
var aiBarRamp = []string{"green", "cyan", "blue", "purple", "orange"}

// aiBarChart renders a chart=bar block's {bars: [{label,pct,value_label?}]}
// (1..5, extras dropped; no per-bar "color" on the wire — see aiBarRamp)
// as the web's Visualizers::Bar: 2 identical braille rows per bar (1-row
// gaps for ≤4 bars, none at 5, vertically centred on the 11-row canvas),
// each row fully ⣿-filled with a dim lead/tail and the bar's ramp color
// riding the filled span — then a legend line of "● label value_label"
// per bar below.
func (r *R) aiBarChart(label string, raw json.RawMessage, width int) string {
	var d aiBarsData
	if json.Unmarshal(raw, &d) != nil || len(d.Bars) == 0 {
		return ""
	}
	for i := range d.Bars {
		d.Bars[i].Color = aiBarRamp[i%len(aiBarRamp)]
	}
	return r.barChart(label, d.Bars, width)
}

// barChart is aiBarChart's core, split out so analyze.go's breakdowns
// bar groups (subscribed_status/devices/geography/demographics_age/
// demographics_gender — each built from the payload's {key,pct} rows via
// its own label/color parity table, not the generic aiBar JSON shape)
// can drive the exact same braille+legend engine instead of duplicating
// it.
func (r *R) barChart(label string, bars []aiBar, width int) string {
	if len(bars) == 0 {
		return ""
	}
	if len(bars) > 5 {
		bars = bars[:5]
	}
	cellW := aiChartCols
	if width > 0 && width < cellW {
		cellW = width
	}
	pcts := make([]float64, len(bars))
	for i, bar := range bars {
		pct := bar.Pct
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		pcts[i] = pct
	}
	cells := barCellCounts(pcts, cellW)

	topPad, gap := barPlotLayout(len(bars))
	blank := strings.Repeat(" ", cellW)
	var lines []string
	for i := 0; i < topPad; i++ {
		lines = append(lines, blank)
	}
	offset := 0
	for j := range bars {
		if j > 0 && gap > 0 {
			lines = append(lines, blank)
		}
		filled := cells[j]
		row := r.barRow(offset, filled, barColor(bars[j].Color), cellW)
		lines = append(lines, row, row)
		offset += filled
	}
	for len(lines) < aiChartRows {
		lines = append(lines, blank)
	}

	var b strings.Builder
	if label != "" {
		b.WriteString(r.dim(label) + "\n")
	}
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n")

	legend := make([]string, len(bars))
	for i, bar := range bars {
		vl := fmt.Sprintf("%.1f%%", pcts[i])
		if bar.ValueLabel != nil {
			vl = *bar.ValueLabel
		}
		bullet := "●"
		if r.truecolor {
			bullet = lipgloss.NewStyle().Foreground(hex(barColor(bar.Color))).Render(bullet)
		}
		legend[i] = bullet + " " + r.dim(strings.TrimSpace(bar.Label+" "+vl))
	}
	b.WriteString(strings.Join(legend, "  "))
	return b.String()
}

// barPlotLayout mirrors Bar#build_plot_rows' geometry: a 1-row gap
// between bars when the whole group (2 rows/bar + gaps) fits the
// canvas (true through n=4), none at n=5 (10 rows, exactly the content
// budget), and the leftover rows split as top padding (Ruby's integer
// division — any odd remainder lands at the bottom, not centered).
func barPlotLayout(n int) (topPad, gap int) {
	if n <= 0 {
		return 0, 0
	}
	if 2*n+(n-1) <= aiChartRows {
		gap = 1
	}
	content := 2*n + gap*(n-1)
	topPad = (aiChartRows - content) / 2
	if topPad < 0 {
		topPad = 0
	}
	return topPad, gap
}

// barRow renders one bar's braille row: cellW cells of ⣿, dim before
// offset and after offset+filled (the "is-outline" lead/tail), the
// bar's own color — riding the shared chart-blue glint when truecolor —
// across the filled span.
func (r *R) barRow(offset, filled int, color RGB, cellW int) string {
	var b strings.Builder
	faint := lipgloss.NewStyle().Foreground(ColorFaint)
	phase := r.staggered("ai-bar-chart")
	for i := 0; i < cellW; i++ {
		if i < offset || i >= offset+filled {
			b.WriteString(faint.Render("⣿"))
			continue
		}
		st := lipgloss.NewStyle().Bold(true)
		if r.truecolor {
			st = lipgloss.NewStyle().Foreground(hex(bandBoost(color, i, cellW, phase)))
		}
		b.WriteString(st.Render("⣿"))
	}
	return b.String()
}

// barCellCounts is the owner's "SIMPLE MATH" cell normalization (port
// of Bar#normalized_cells): every positive bar starts at its rounded
// share of cols with a 1-cell floor, then the total is nudged onto the
// group's own pct-of-cols target (shave/grow the CURRENT biggest bar
// one cell at a time) — a full 100% breakdown closes the canvas exactly
// at cols; a partial group stays honest about the remainder.
func barCellCounts(pcts []float64, cols int) []int {
	wants := make([]int, len(pcts))
	sum := 0.0
	positive := 0
	for i, pct := range pcts {
		wants[i] = barFilledCells(pct, cols)
		sum += pct
		if wants[i] > 0 {
			positive++
		}
	}
	target := int(math.Round(sum / 100 * float64(cols)))
	if target < 0 {
		target = 0
	}
	if target > cols {
		target = cols
	}
	if positive > target {
		target = positive // rule 1 (every positive bar ≥1 cell) outranks a cut
	}
	total := sumInts(wants)
	for total > target {
		i := argmaxInt(wants)
		wants[i]--
		total--
	}
	for total < target {
		i := argmaxInt(wants)
		wants[i]++
		total++
	}
	return wants
}

// barFilledCells is one bar's raw cell share before group normalization:
// 0% draws nothing, any positive pct draws at least 1 cell.
func barFilledCells(pct float64, cols int) int {
	if pct <= 0 {
		return 0
	}
	c := int(math.Round(pct / 100 * float64(cols)))
	if c < 1 {
		c = 1
	}
	if c > cols {
		c = cols
	}
	return c
}

func sumInts(xs []int) int {
	sum := 0
	for _, x := range xs {
		sum += x
	}
	return sum
}

// argmaxInt returns the index of the FIRST maximum — matching Ruby's
// `array.index(array.max)`, which the owner's balancing loop depends on
// for stable, deterministic normalization.
func argmaxInt(xs []int) int {
	mi := 0
	for i, v := range xs {
		if v > xs[mi] {
			mi = i
		}
	}
	return mi
}

// ---------------------------------------------------------------------
// viz=heatmap — N equal-width, full-height ⣿ columns, color-only.

type aiHeatmapData struct {
	Values []float64 `json:"values"`
	Labels []string  `json:"labels"`
}

// heatmapWeekdayLabels is the Monday-first preset (Visualizers::Heatmap
// ::DAY_LABELS) that kicks in when exactly 7 values arrive with no
// explicit labels.
var heatmapWeekdayLabels = []string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"}

// aiHeatmapChart renders a chart=heatmap block's {values, labels?} — flat
// on the block itself, no "data" envelope — (2..42 values; labels, when
// given, 1:1 with values) as N equal-width, full-height braille
// columns — every cell is ⣿, the VALUE reads only in the column's
// color (a green↔red lerp of the set's min..max, flat data neutral at
// the midpoint), mirroring Visualizers::Heatmap.
func (r *R) aiHeatmapChart(label string, raw json.RawMessage, width int) string {
	var d aiHeatmapData
	if json.Unmarshal(raw, &d) != nil {
		return ""
	}
	return r.heatmapChart(label, d, width)
}

// heatmapChart is aiHeatmapChart's core, split out so analyze.go's
// day_of_week_heatmap breakdown (its own {values, caption} JSON shape,
// not the generic aiHeatmapData envelope) can drive the exact same
// column+color engine instead of duplicating it.
func (r *R) heatmapChart(label string, d aiHeatmapData, width int) string {
	n := len(d.Values)
	if n < 2 || n > aiChartCols {
		return ""
	}
	labels := d.Labels
	if labels == nil && n == len(heatmapWeekdayLabels) {
		labels = heatmapWeekdayLabels
	}
	if labels != nil && len(labels) != n {
		return ""
	}
	cellW := aiChartCols
	if width > 0 && width < cellW {
		cellW = width
	}
	widths := heatColumnWidths(n, cellW)
	colors := heatmapColors(d.Values)

	plotRow := func() string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			st := lipgloss.NewStyle()
			if r.truecolor {
				st = st.Foreground(hex(colors[i]))
			}
			b.WriteString(st.Render(strings.Repeat("⣿", widths[i])))
		}
		return b.String()
	}
	lines := make([]string, aiChartRows)
	for i := range lines {
		lines[i] = plotRow()
	}

	var b strings.Builder
	if label != "" {
		b.WriteString(r.dim(label) + "\n")
	}
	b.WriteString(strings.Join(lines, "\n"))
	if len(labels) > 0 {
		b.WriteString("\n" + r.dim(heatmapLabelLine(labels, widths)))
	}
	return b.String()
}

// heatColumnWidths splits cols evenly across n columns; the remainder
// spreads to the LEFTMOST columns (Heatmap#bar_cols) so the row still
// spans the full canvas width regardless of divisibility.
func heatColumnWidths(n, cols int) []int {
	if n <= 0 {
		return nil
	}
	base := cols / n
	extra := cols % n
	widths := make([]int, n)
	for i := range widths {
		widths[i] = base
		if i < extra {
			widths[i]++
		}
	}
	return widths
}

// heatmapColors normalizes values to the set's own min..max (worst→
// best) and lerps aiThemeRed→aiThemeGreen across it — a flat set
// (including all-zero) maps every column to the neutral midpoint, never
// a false winner/loser (Heatmap#heat_fraction).
func heatmapColors(values []float64) []RGB {
	colors := make([]RGB, len(values))
	if len(values) == 0 {
		return colors
	}
	lo, hi := values[0], values[0]
	for _, v := range values {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	for i, v := range values {
		f := 0.5
		if hi > lo {
			f = (v - lo) / (hi - lo)
		}
		colors[i] = mixRGB(aiThemeRed, aiThemeGreen, f)
	}
	return colors
}

// heatmapLabelLine centers each label under its column's width so the
// joined line still spans exactly sum(widths) cells — "1:1 under
// columns", no separators (the columns already tile edge to edge).
func heatmapLabelLine(labels []string, widths []int) string {
	var b strings.Builder
	for i, lbl := range labels {
		w := widths[i]
		lr := []rune(lbl)
		if len(lr) > w {
			lr = lr[:w]
		}
		pad := w - len(lr)
		left := pad / 2
		right := pad - left
		b.WriteString(strings.Repeat(" ", left))
		b.WriteString(string(lr))
		b.WriteString(strings.Repeat(" ", right))
	}
	return b.String()
}

// ---------------------------------------------------------------------
// viz=heart — delegates to heart.go's shared canvas (D4).

type aiHeartData struct {
	Score    *float64 `json:"score"` // pointer: nil means "absent", not 0
	Likes    int64    `json:"likes"`
	Dislikes int64    `json:"dislikes"`
}

// aiHeartTint is the AI palette's only heart color — VizBlockComponent
// #heart hardcodes `color: :red` (the theme's accent-red) regardless of
// score.
var aiHeartTint = aiThemeRed

// aiHeartChart renders a chart=heart block's {score, likes?, dislikes?} —
// flat on the block itself, no "data" envelope — via the shared
// heartCanvas (heart.go, a sibling dispatch) — this file only decodes
// the block and never draws heart glyphs itself. A missing/malformed
// score degrades to "" rather than a fake 0% heart.
func (r *R) aiHeartChart(label string, raw json.RawMessage, width int) string {
	var d aiHeartData
	if json.Unmarshal(raw, &d) != nil || d.Score == nil {
		return ""
	}
	pct := fmt.Sprintf("%.2f%%", *d.Score)
	canvas := r.heartCanvas(*d.Score, aiHeartTint, int(d.Likes), int(d.Dislikes), pct)
	if label == "" {
		return canvas
	}
	return r.dim(label) + "\n" + canvas
}
