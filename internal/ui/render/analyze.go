package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// The analyze payload (live-captured 2026-07-05, extended 2026-07-11
// with a `breakdowns channel` capture): an `analyze` key carrying, per
// role:
//   - system:   per-metric scalar series under `stash` (subs/views/
//     watched_hours/avg_viewed_pct/avg_view_duration) + a top-level
//     `likes` heart metric.
//   - enhanced: the SAME `stash` shape (chart-slot entries mirror into
//     it) plus its own typed top-level fields — `bars` (subscribed_
//     status/devices/geography/demographics_age/demographics_gender),
//     `bar_captions`, `day_of_week_heatmap`, `retention`, `comments` —
//     and `metric_keys`, the server's own fan-out order.
//
// The web draws these with its visualizer components; the terminal
// drives the SAME shared painters as the @ai chart family (ai_charts.go)
// off this file's own decode shapes — full ticked area canvases for
// EVERY scalar stash metric AND the breakdowns' retention/comments
// (parity: the web puts all of them through the same Visualizers::Area,
// never the bare Sparkline — that one's reserved for `glance`), full bar/
// heatmap canvases for the rest of the breakdowns extras, and heart.go's
// braille heart for likes.

type analyzePayload struct {
	Analyze *analyzeData `json:"analyze"`
}

type analyzeData struct {
	Intro string                   `json:"intro"`
	Level string                   `json:"level"`
	Stash map[string]analyzeMetric `json:"stash"`
	Likes *heartMetric             `json:"likes"`

	// Breakdowns extras (enhanced role only; nil/empty on plain payloads).
	Bars             map[string][]analyzeBarRow `json:"bars"`
	BarCaptions      map[string]string          `json:"bar_captions"`
	DayOfWeekHeatmap *analyzeHeatmapData        `json:"day_of_week_heatmap"`
	Retention        *analyzeAreaData           `json:"retention"`
	Comments         *analyzeAreaData           `json:"comments"`
	MetricKeys       []string                   `json:"metric_keys"`
}

type analyzeMetric struct {
	// Data stays raw: slots differ per metric (charts carry a series
	// object, the likes slot carries a bare list) and one alien entry
	// must not poison the whole stash decode.
	Data json.RawMessage `json:"data"`
	Slot string          `json:"slot"`
}

type analyzeSeries struct {
	Dates       []string  `json:"dates"`
	Series      []float64 `json:"series"`
	Total       float64   `json:"total"`
	TotalPct    *float64  `json:"total_pct"`
	Previous    *float64  `json:"previous"`
	TargetDaily float64   `json:"target_daily"`
}

type analyzeCaption struct {
	Caption string `json:"caption"`
}

// analyzeBarRow is one breakdowns bar row as the payload actually ships
// it: the raw dimension key + its share of the group. Label text and
// (absent an explicit token) the fallback color come from barPresentation
// — the payload never carries a human label itself.
type analyzeBarRow struct {
	Key   string  `json:"key"`
	Pct   float64 `json:"pct"`
	Color string  `json:"color"` // rare; payload-carried override, honored when present
}

// analyzeHeatmapData is the day_of_week_heatmap breakdown's own shape —
// distinct from the generic aiHeatmapData envelope (ai_charts.go), which
// heatmapChart shares underneath it.
type analyzeHeatmapData struct {
	Values  []float64 `json:"values"`
	Caption string    `json:"caption"`
}

// analyzeAreaData is the shared shape of the retention/comments
// breakdown series — distinct from the generic aiAreaData envelope
// (ai_charts.go), which areaChart shares underneath both of them.
type analyzeAreaData struct {
	Series      []float64 `json:"series"`
	Dates       []string  `json:"dates"` // comments only; retention has none → day-index x-ticks
	TargetDaily float64   `json:"target_daily"`
	Caption     string    `json:"caption"`
}

type heartEntry struct {
	Color    string  `json:"color"`
	Score    float64 `json:"score"`
	Likes    int64   `json:"likes"`
	Dislikes int64   `json:"dislikes"`
}

type heartMetric struct {
	Caption string       `json:"caption"`
	Hearts  []heartEntry `json:"hearts"`
}

// Every analyze area chart — the breakdowns extras (retention/comments)
// and every stash scalar metric via spark() alike — ticks its x-axis
// through the SAME year-aware house rule as the generic @ai chart=area
// block: ai_charts.go's formatDateTick, driven off r.now() with no
// per-caller layout override (Rails' analytics cell ALWAYS drives the
// exact same Visualizers::Area component regardless of which metric or
// breakdown is on screen).

// analyzeBarMetrics is the set of metric names that render as a bar
// group (Pito::MessageBuilder::Analyze::Message::BAR_METRIC_KEYS).
var analyzeBarMetrics = map[string]bool{
	"subscribed_status":   true,
	"devices":             true,
	"geography":           true,
	"demographics_age":    true,
	"demographics_gender": true,
}

// analyzeGeoRamp / analyzeAgeRamp are the position-indexed color ramps
// pito's bar_presentation (lib/pito/message_builder/analyze/message.rb)
// applies to geography / demographics_age — colored by ORDER, not by
// key, since both dimensions carry open-ended key sets (country codes,
// age brackets) a fixed key→color map can't cover.
var (
	analyzeGeoRamp = []string{"green", "cyan", "blue", "purple", "orange"}
	analyzeAgeRamp = []string{"cyan", "blue", "purple", "pink", "yellow"}
)

// analyzeBlock renders the charts for an analyze payload; "" when the
// payload carries none (callers fall through to plain body rendering).
func (r *R) analyzeBlock(payload []byte) string {
	var p analyzePayload
	if json.Unmarshal(payload, &p) != nil || p.Analyze == nil {
		return ""
	}
	a := p.Analyze

	// Captions for the old-style scalar stash metrics live beside the
	// stash under the metric's own top-level key (the breakdowns extras
	// carry their caption inline instead — bar_captions, or a `caption`
	// field on their own struct).
	var captions map[string]analyzeCaption
	_ = json.Unmarshal(func() []byte {
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(payload, &raw)
		return raw["analyze"]
	}(), &captions)

	width := r.width - 3

	// Stable metric order: metric_keys — the server's own fan-out order —
	// when the payload carries it (every breakdowns extras payload does);
	// else the web's legacy scalar chart order (plain payloads never set
	// metric_keys). Either way, any stash entry the order list misses
	// still renders, alphabetically, at the end.
	order := a.MetricKeys
	if len(order) == 0 {
		order = []string{"subs", "views", "watched_hours", "avg_viewed_pct", "avg_view_duration", "ctr"}
	}
	seen := make(map[string]bool, len(order))
	names := append([]string{}, order...)
	for _, name := range order {
		seen[name] = true
	}
	var rest []string
	for name := range a.Stash {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	names = append(names, rest...)

	// Each metric renders as one caption+chart unit; wide terminals lay
	// the units TWO-UP (owner 2026-07-12: a single left column wasted
	// half the screen — the web's analytics panel is multi-column too).
	// analyzeCellW fits the 42-cell canvas plus its widest y-axis gutter;
	// captions wrap inside the cell so a long sentence can't shove its
	// neighbor.
	const analyzeCellW = 54
	var units []string
	for _, name := range names {
		body, caption := r.analyzeMetricBody(name, a, captions, width)
		if body == "" {
			continue
		}
		head := strings.ReplaceAll(name, "_", " ")
		if caption != "" {
			head = r.paintShimmer(htmlToText(caption))
		}
		if width >= analyzeCellW*2+4 {
			head = lipgloss.NewStyle().Width(analyzeCellW).Render(head)
		}
		units = append(units, head+"\n"+body)
	}
	var b strings.Builder
	if width >= analyzeCellW*2+4 {
		for i := 0; i < len(units); i += 2 {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			if i+1 < len(units) {
				left := lipgloss.NewStyle().Width(analyzeCellW).Render(units[i])
				b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", units[i+1]) + "\n")
			} else {
				b.WriteString(units[i] + "\n")
			}
		}
	} else {
		for _, unit := range units {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(unit + "\n")
		}
	}

	if a.Likes != nil && len(a.Likes.Hearts) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		if a.Likes.Caption != "" {
			b.WriteString(r.paintShimmer(htmlToText(a.Likes.Caption)) + "\n")
		}
		b.WriteString(r.analyzeHeartsRow(a.Likes.Hearts, width))
	}
	return b.String()
}

// analyzeMetricBody dispatches one metric name to its chart body +
// caption: a bar group, the weekday heatmap, a ticked retention/comments
// area chart, or (the fallback, matching every plain analyze payload) the
// stash's own ticked area chart (spark() — the SAME area engine as the
// three cases above it, just fed the stash's per-metric value_format/
// x_axis preset). body == "" means no data yet — the caller's existing
// pending/no-data treatment (skip silently).
func (r *R) analyzeMetricBody(name string, a *analyzeData, captions map[string]analyzeCaption, width int) (body, caption string) {
	switch {
	case analyzeBarMetrics[name]:
		return r.analyzeBarBlock(name, a.Bars[name], width), a.BarCaptions[name]
	case name == "day_of_week_heatmap" && a.DayOfWeekHeatmap != nil:
		return r.analyzeHeatmapBlock(a.DayOfWeekHeatmap, width), a.DayOfWeekHeatmap.Caption
	case name == "retention" && a.Retention != nil:
		return r.analyzeAreaBlock(a.Retention, width), a.Retention.Caption
	case name == "comments" && a.Comments != nil:
		return r.analyzeAreaBlock(a.Comments, width), a.Comments.Caption
	default:
		m, ok := a.Stash[name]
		if !ok || m.Slot != "charts" {
			return "", ""
		}
		var data analyzeSeries
		if json.Unmarshal(m.Data, &data) != nil || len(data.Series) == 0 {
			return "", ""
		}
		return r.spark(name, data, width), captions[name].Caption
	}
}

// analyzeBarBlock renders one breakdown's bar group: the payload's
// {key,pct[,color]} rows resolved through barPresentation into the AI
// chart family's generic aiBar shape, then drawn by the shared
// braille+legend engine (ai_charts.go's barChart) — the exact same
// canvas an @ai chart=bar block gets.
func (r *R) analyzeBarBlock(metric string, rows []analyzeBarRow, width int) string {
	if len(rows) == 0 {
		return ""
	}
	bars := make([]aiBar, len(rows))
	for i, row := range rows {
		label, color := barPresentation(metric, row.Key, i)
		if row.Color != "" {
			color = row.Color // the payload's own color token wins over the fallback table
		}
		bars[i] = aiBar{Label: label, Pct: row.Pct, Color: color}
	}
	return r.barChart("", bars, width)
}

// analyzeHeatmapBlock renders the day_of_week_heatmap breakdown via the
// shared column+color engine (ai_charts.go's heatmapChart) — its 7
// values trip the weekday-label preset (Mo..Su) automatically.
func (r *R) analyzeHeatmapBlock(d *analyzeHeatmapData, width int) string {
	if d == nil || len(d.Values) == 0 {
		return ""
	}
	return r.heatmapChart("", aiHeatmapData{Values: d.Values}, width)
}

// analyzeAreaBlock renders one retention/comments breakdown series via
// the shared ticked braille engine (ai_charts.go's areaChart) — retention
// carries no `dates`, so its x-ticks fall back to day-index (1..N)
// automatically; comments' dates tick through the same year-aware house
// rule (formatDateTick) as every other dated area chart.
func (r *R) analyzeAreaBlock(d *analyzeAreaData, width int) string {
	if d == nil || len(d.Series) == 0 {
		return ""
	}
	return r.areaChart("", aiAreaData{Series: d.Series, Dates: d.Dates, Target: d.TargetDaily}, width)
}

// analyzeHeartsRow lays out the likes slot's hearts — one for a channel,
// two for a vid (its own heart + the parent channel's) — side by side
// with the web's GAP_CELLS = 3 gutter, each drawn by the shared braille
// heart canvas (heart.go). Falls back to a vertical stack, blank-line
// separated, when the row would overflow width.
func (r *R) analyzeHeartsRow(hearts []heartEntry, width int) string {
	canvases := make([]string, len(hearts))
	rowWidth := 0
	for i, h := range hearts {
		pct := fmt.Sprintf("%.2f%%", h.Score)
		canvases[i] = r.heartCanvas(h.Score, barColor(h.Color), int(h.Likes), int(h.Dislikes), pct)
		rowWidth += lipgloss.Width(canvases[i]) // the legend line, not the 17-cell canvas, is usually the widest line
	}
	const gap = "   " // GAP_CELLS = 3
	rowWidth += len(gap) * (len(canvases) - 1)
	if len(canvases) <= 1 || rowWidth > width {
		return strings.Join(canvases, "\n\n")
	}
	joined := canvases[0]
	for _, c := range canvases[1:] {
		joined = lipgloss.JoinHorizontal(lipgloss.Top, joined, gap, c)
	}
	return joined
}

// barPresentation ports pito's bar_presentation (lib/pito/message_
// builder/analyze/message.rb): the {label, color token} for one
// breakdown row, keyed by the payload's raw dimension `key` (+ its
// index, for the ramp-colored metrics geography/demographics_age).
// Falls back to the raw key as the label and blue as the color for an
// unrecognized metric name.
func barPresentation(metric, key string, index int) (label, color string) {
	switch metric {
	case "subscribed_status":
		if key == "SUBSCRIBED" {
			return "Subscribed", "green"
		}
		return "Not subscribed", "red"
	case "devices":
		switch key {
		case "DESKTOP":
			return "Computer", "purple"
		case "TV":
			return "TV", "cyan"
		default: // "MOBILE" and any unrecognized bucket
			return "Mobile", "blue"
		}
	case "demographics_gender":
		switch key {
		case "male":
			return "Male", "blue"
		case "female":
			return "Female", "pink"
		default: // "gender_other" and any unrecognized bucket
			return "Other", "purple"
		}
	case "geography":
		c := analyzeGeoRamp[index%len(analyzeGeoRamp)]
		if key == "OTHER" {
			return "Other", c
		}
		return countryName(key), c
	case "demographics_age":
		c := analyzeAgeRamp[index%len(analyzeAgeRamp)]
		if key == "OTHER" {
			return "Other", c
		}
		return formatAgeBracket(key), c
	default:
		return key, "blue"
	}
}

// countryName resolves an ISO-3166 alpha-2 code to its English display
// name (pito's Pito::Geo.country_name, backed by the same ISO-3166 data
// as Go's own golang.org/x/text/language region tables). An unrecognized
// code passes through verbatim rather than vanishing.
func countryName(code string) string {
	region, err := language.ParseRegion(code)
	if err != nil {
		return code
	}
	if name := display.English.Regions().Name(region); name != "" {
		return name
	}
	return code
}

// formatAgeBracket mirrors pito's format_age: "age25-34" → "25–34" (en
// dash), "age65-" → "65+".
func formatAgeBracket(key string) string {
	s := strings.TrimPrefix(key, "age")
	if strings.HasSuffix(s, "-") {
		return strings.TrimSuffix(s, "-") + "+"
	}
	return strings.ReplaceAll(s, "-", "–")
}

// spark renders one stash scalar metric (views/watched_hours/subs/
// avg_view_duration/avg_viewed_pct/…) through the SAME 42×11 ticked
// braille engine the D8 breakdowns extras use (ai_charts.go's areaChart)
// — parity fix: the web draws EVERY stash metric through its own
// Pito::Analytics::Visualizers::Area instance too (analyze_cell_
// component.html.erb's `chart?` branch is the SAME branch retention/
// comments render through), never the bare 2-row Sparkline visualizer
// (that one's reserved for `glance`'s Slots::Compact, a different
// surface — app/components/pito/analytics/visualizers/sparkline.rb) — so
// the terminal was under-drawing the stash relative to the web's real
// per-metric widget: no y-ticks, no x-axis, at half the row count.
// y-ticks + x-ticks (dates in the year-aware house style — day-first
// within the current year, month-only across a year boundary — else
// day-index) come from the shared engine automatically; analyzeStashPreset
// supplies the per-metric value_format/x_axis pair (ported from
// Area#preset_value_format/#preset_x_axis), and the total/prev/target
// legend line below is UNCHANGED from before this pass — it's a TUI-only
// addition over the web (no such line exists there) and stays exactly as
// it formatted.
func (r *R) spark(name string, s analyzeSeries, width int) string {
	format, xAxis := analyzeStashPreset(name)
	chart := r.areaChart("", aiAreaData{
		Series: s.Series,
		Dates:  s.Dates,
		Target: s.TargetDaily,
		Format: format,
		XAxis:  xAxis,
	}, width)

	total := trimFloat(s.Total)
	if s.TotalPct != nil {
		total = trimFloat(*s.TotalPct) + "%"
	}
	meta := "total " + total
	if s.Previous != nil {
		meta += " · prev " + trimFloat(*s.Previous)
	}
	if s.TargetDaily > 0 {
		meta += " · target " + trimFloat(s.TargetDaily) + "/d"
	}
	return chart + "\n" + r.dim(meta)
}

// analyzeStashPreset ports Visualizers::Area#preset_value_format /
// #preset_x_axis for the stash's scalar metrics: avg_view_duration is the
// only stash metric with a duration (M:SS) y-axis; avg_viewed_pct is the
// only one the web plots against its fixed 0%→100% POSITION axis rather
// than calendar dates (Rails checks `@metric == :avg_viewed_pct` BEFORE
// ever looking at whether dates were supplied — ai_charts.go's
// areaXAxisPercent mirrors that same early-return order) and gets
// percent (XX.XX%) y-ticks to match; every other stash metric (views/
// watched_hours/subs/ctr/anything the server adds later) gets the `else`
// fallback both Rails methods share: count y-ticks, date/day-index x-ticks.
func analyzeStashPreset(name string) (format, xAxis string) {
	switch name {
	case "avg_view_duration":
		return "duration", ""
	case "avg_viewed_pct":
		return "percent", areaXAxisPercent
	default:
		return "count", ""
	}
}

func trimFloat(v float64) string {
	out := fmt.Sprintf("%.2f", v)
	out = strings.TrimRight(strings.TrimRight(out, "0"), ".")
	if out == "" || out == "-" {
		out = "0"
	}
	return out
}

// hasAnalyze reports whether the payload is an analyze message at all —
// including the pending state before any metric data lands.
func hasAnalyze(payload []byte) bool {
	var p analyzePayload
	return json.Unmarshal(payload, &p) == nil && p.Analyze != nil
}

// analyzeIntro pulls the intro line out of a chart payload.
func analyzeIntro(payload []byte) string {
	var p analyzePayload
	if json.Unmarshal(payload, &p) != nil || p.Analyze == nil {
		return ""
	}
	return p.Analyze.Intro
}
