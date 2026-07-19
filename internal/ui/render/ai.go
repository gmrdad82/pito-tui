// The @ai event renderer (pito 2.0.0's `ai` kind, W2's integration point).
// A pending turn wears the plain system chrome — the same bar every
// system/enhanced message uses — with whatever blocks have already
// streamed in, plus a shimmering ellipsis marking where the server's own
// copy will land; the client never invents pending-state prose. A done
// turn wears the AI's own chrome: a left bar that SLIDES the purple→
// pito-blue gradient with the animation phase (aiAccentBar's static echo
// treatment, given motion), every block the server sent in order, a
// right-aligned model/cost badge, and the reply affordance. The server
// defines nine block types (text, kv_table, table, media, sparkline,
// chart, score, ttb, suggestion — the `case b["type"]` in Ai::Blocks
// #normalize, lib/ai/blocks.rb; MAX_BLOCKS=12 is the unrelated per-message
// block COUNT cap). Two of the nine (score, ttb) have no painter
// elsewhere in the W2 split — small adapters here feed them into the
// shared ScoreBar/bar primitives (bars.go) the same way detail.go's show
// cards do. Dispatch
// never trusts a single block: an unrecognized type or an empty result
// falls back to the raw-JSON degrade line, and a payload this client
// can't even decode degrades exactly like any other unknown-shaped event.
package render

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// aiEvent renders one kind "ai" event. DecodeAiPayload only errors when
// the payload isn't a JSON object at all (per its own contract) — that
// case degrades through the same r.fallback every other unknown-shaped
// event uses, rather than a bespoke two-line copy, since r.fallback is
// right here in the same package.
func (r *R) aiEvent(ev api.Event) string {
	p, err := api.DecodeAiPayload(ev.Payload)
	if err != nil {
		return r.fallback(ev)
	}
	// Painters get the message column minus the left bar + its padding —
	// the same budget aiAccentBar's own content area resolves to (width-3
	// outer, minus 1 more for PaddingLeft). Charts self-cap at 42 inside
	// their own painters regardless of what they're handed here.
	width := r.width - 4
	if width < 1 {
		width = 1
	}
	if p.Status == "done" {
		return r.aiDoneEvent(ev, p, width)
	}
	// Any non-"done" status (contract: "pending", but a future server
	// value degrades the same way rather than crashing) reads as pending.
	return r.aiPendingEvent(ev, p, width)
}

// aiPendingEvent mirrors messageBlock's own shape (system chrome, stamp
// glued to the body) — a pending @ai turn is, chrome-wise, just a system
// message whose body is still arriving. Whatever blocks have already
// streamed in render normally; the turn's own answer text is not
// synthesized here, only the shimmering "…" marking that more is coming.
func (r *R) aiPendingEvent(ev api.Event, p api.AiPayload, width int) string {
	parts := r.renderAiBlocks(p.Blocks, width)
	content := r.stamp(ev) + strings.Join(parts, "\n\n")
	if len(parts) > 0 {
		content += "\n\n"
	}
	content += r.aiPendingLine(ev.Payload)
	return r.systemBar.Render(content) + "\n"
}

// aiPendingLine is the pending chrome's placeholder line: the server's own
// status_line (streamed live via event.ai_status, e.g. "Scouring the
// internet…" — model.go's client-side payload-key convention) when one
// has arrived, or else the bare "…" in place of invented copy. Either
// text rides the AI gradient's OWN colors as r.phase advances
// (Gradient.Colorize samples the ramp per rune at t = i/(n-1) + phase; a
// lone "…" rune's t collapses to just `phase`) — the "network shimmer"
// the spec asks for, with zero invented text of its own.
func (r *R) aiPendingLine(payload json.RawMessage) string {
	text := "…"
	var s struct {
		StatusLine string `json:"status_line"`
	}
	if json.Unmarshal(payload, &s) == nil && strings.TrimSpace(s.StatusLine) != "" {
		text = s.StatusLine
	}
	if r.truecolor {
		return AIAccent.Colorize(text, r.phase)
	}
	return r.dim(text)
}

// aiDoneEvent renders a settled @ai answer: stamp glued to the block
// body (matching every other chrome's stamp+body idiom), each block
// separated by exactly one blank line, then the model/cost badge and the
// reply affordance each on their own line — the whole thing behind
// aiChrome's sliding left bar.
func (r *R) aiDoneEvent(ev api.Event, p api.AiPayload, width int) string {
	parts := r.renderAiBlocks(p.Blocks, width)
	content := r.stamp(ev) + strings.Join(parts, "\n\n")
	// ONE footer row (owner 2026-07-13: "shift+r should be on the same
	// row as the model. Not 2 rows needed"): the reply affordance sits
	// left, the ✦ badge right — with a one-cell right margin so the
	// model name never touches the tile's edge.
	left := ""
	if p.ReplyHandle != "" && !p.ReplyConsumed {
		// The house replyAffordance builder (render.go) — shared so this
		// and render.go's replyHintFor/confirmation can't drift apart.
		left = r.replyAffordance(p.ReplyHandle)
	}
	right := ""
	if model := strings.TrimSpace(p.Model); model != "" {
		text := "✦ " + model
		if cost := formatAiCost(p.CostAmount, p.CostCurrency); cost != "" {
			if p.CostEstimated {
				cost = "~" + cost // pito-computed, not a provider receipt — see formatAiCost
			}
			text += " · " + cost
		}
		right = r.dim(text)
	}
	if left != "" || right != "" {
		pad := width - lipgloss.Width(left) - lipgloss.Width(right) - 1
		if pad < 1 {
			pad = 1
		}
		content += "\n" + left + strings.Repeat(" ", pad) + right
	}
	return r.aiChrome(content) + "\n"
}

// aiChrome wraps done-turn content behind a left bar riding the AI
// gradient — aiAccentBar's own look (render.go), but the band SLIDES
// with r.phase instead of sitting static: an echo is a one-shot block
// that never needs to keep breathing, but a settled answer stays on
// screen and the web's own data-accent="ai" surfaces keep animating
// while they're in view. Off-truecolor terminals fall back to a flat
// ColorPrimary bar, same as every other truecolor/plain split here.
// The bar is the WHOLE chrome: the tile background it once carried
// (the web's pito-ai-surface analog) was tried in lavender, faded
// pink, indigo, and dark teal across 2026-07-12/13 and purged by
// owner ruling — "We drop the background for the AI message and we
// leave only the flash border."
func (r *R) aiChrome(content string) string {
	body := lipgloss.NewStyle().PaddingLeft(1).Width(r.width - 3).Render(content)
	lines := strings.Split(body, "\n")
	var b strings.Builder
	for i, line := range lines {
		style := lipgloss.NewStyle().Foreground(ColorPrimary)
		if r.truecolor {
			t := 0.0
			if len(lines) > 1 {
				t = float64(i) / float64(len(lines)-1)
			}
			t += r.phase * AIPulseSpeed
			t -= math.Floor(t) // wrap into [0,1) — the band travels down the block
			style = style.Foreground(hex(AIAccent.At(t)))
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(style.Render("┃") + line)
	}
	return b.String()
}

// formatAiCost renders CostAmount/CostCurrency for the badge. A nil
// amount (no price yet, or an unpriced model) omits the whole "·
// <cost>" segment — the caller checks for "" and skips the separator
// entirely, never a bare "·". USD gets the web's attached "$0.03" look
// (two decimals, no space); any other or unrecognized currency code
// falls back to "<amount> <CODE>" so an unfamiliar currency still reads,
// just without pretending to know its symbol. This function never sees
// CostEstimated — it renders a bare amount whether the number is a
// genuine provider receipt or a pito-side estimate (stamped by the
// server when the provider itself reported no cost, e.g. OpenCode Zen);
// aiDoneEvent, the sole caller, is the one that tells the two apart and
// prefixes a "~" onto this function's return when CostEstimated is set.
func formatAiCost(amount *float64, currency string) string {
	if amount == nil {
		return ""
	}
	if strings.EqualFold(currency, "USD") {
		return fmt.Sprintf("$%.2f", *amount)
	}
	return fmt.Sprintf("%.2f %s", *amount, currency)
}

// renderAiBlocks dispatches every block to its painter in order,
// threading a running suggestion index across the whole message (the
// first suggestion block is 0, the second 1, … — position among
// suggestion-typed blocks, counted whether or not that particular one
// ends up rendering). media blocks are skipped before dispatch even runs
// — the owner's "no images, not even a placeholder" rule — so they never
// reach the degrade fallback either. Every other block that comes back
// "" (an unknown type, or a recognized type whose own decode failed)
// degrades to its raw-JSON dump; only a genuinely empty degrade (blank
// raw bytes) drops silently too.
func (r *R) renderAiBlocks(blocks []api.AiBlock, width int) []string {
	var out []string
	suggestionIdx := 0
	for _, b := range blocks {
		if b.Type == "media" {
			continue
		}
		var rendered string
		switch b.Type {
		case "text":
			rendered = r.aiTextBlock(b.Raw, width)
		case "kv_table":
			rendered = r.aiKvTableBlock(b.Raw, width)
		case "table":
			rendered = r.aiTableBlock(b.Raw, width)
		case "suggestion":
			rendered = r.aiSuggestionBlock(b.Raw, suggestionIdx, width)
			suggestionIdx++
		case "sparkline":
			rendered = r.aiSparklineBlock(b.Raw, width)
		case "chart":
			rendered = r.aiChartBlock(b.Raw, width)
		case "score":
			rendered = r.aiScoreBlock(b.Raw, width)
		case "ttb":
			rendered = r.aiTtbBlock(b.Raw, width)
		default:
			rendered = "" // unknown type: falls through to the degrade dump below
		}
		if rendered == "" {
			rendered = r.aiDegradeBlock(b.Raw, width)
		}
		if rendered != "" {
			out = append(out, rendered)
		}
	}
	return out
}

// ── score ────────────────────────────────────────────────────────────────

// aiScoreBlock renders {value, label?} through the shared ScoreBar engine
// (bars.go) — the same 0-100 gauge detail.go wires up for show cards'
// critic scores, just fed straight from the AI's flat JSON instead of a
// parsed HTML fragment. value is clamped into 0..100 rather than passed
// through to ScoreBar's own score<0 "muted, no data" branch: an AI score
// block is never "no data" by contract, worst case an out-of-range
// number that clamps to an extreme.
func (r *R) aiScoreBlock(raw json.RawMessage, width int) string {
	var p struct {
		Value float64 `json:"value"`
		Label string  `json:"label"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return ""
	}
	v := p.Value
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	label := strings.TrimSpace(p.Label)
	if label == "" {
		label = "Score"
	}
	// CONTAINED width (owner rule, W3 containment law): the bar caps at
	// scoreBarCap cells even when the message column is much wider.
	if width > scoreBarCap {
		width = scoreBarCap
	}
	// The trailing space is the web's CSS gap between label and bracket —
	// detail.go's ScoreBar callers add it the same way.
	return r.ScoreBar(label+" ", int(v+0.5), width)
}

// ── ttb ──────────────────────────────────────────────────────────────────

type aiTtbLevel struct {
	Label string  `json:"label"`
	Hours float64 `json:"hours"`
}

type aiTtbCurrent struct {
	Label string  `json:"label"`
	Hours float64 `json:"hours"`
}

type aiTtbPayload struct {
	Levels  []aiTtbLevel  `json:"levels"`
	Current *aiTtbCurrent `json:"current"`
	Label   string        `json:"label"`
}

// aiTtbMaxLevels bounds a defensive read of `levels` — the contract caps
// it, but a hand-built or future payload gets clamped rather than
// rejected outright (the house degrade-not-crash rule).
const aiTtbMaxLevels = 4

// aiTtbLevelColors extends detail.go's tickColor (main/extras/
// completionist → green/lime/pink, in that positional order) with a 4th
// slot for a payload that pushes past the game preset's usual three
// pillars: amber, the HEAT_THRESHOLDS "commitment" tier that sits
// between lime and pink on the web's own heat ramp.
var aiTtbLevelColors = []RGB{heatGreen, heatLime, heatPink, heatAmber}

// aiTtbCurrentColor mirrors the footage tick's fg-default treatment: the
// CURRENT progress tracker is never a milestone color, just "you are
// here" against the milestones.
var aiTtbCurrentColor = RGB{0xda, 0xda, 0xda}

// aiTtbBlock renders {levels: [{label,hours}], current: {label,hours}?,
// label?} as the web's TimeToBeatComponent bar: a green→lime→amber→pink
// heat fill scaled to the largest hour value in play, a `|` tick per
// level at its hour's position with the hour value stamped in a row
// below, and — when `current` is given — a footage-style tick carrying
// its own inline value chip beside it. detail.go's ttbBlock renders the
// identical three-row anatomy off a ttbTickData/tickColor pair keyed by
// fixed strings ("main"/"extras"/"completionist"/"footage") parsed out
// of server HTML; that key coupling doesn't fit a caller-supplied,
// arbitrarily-labeled level list, so this builds the same look directly
// from the shared primitives (BarTick, r.barLine, r.positionRow,
// StopGradient — all bars.go) instead of forcing data through it.
func (r *R) aiTtbBlock(raw json.RawMessage, width int) string {
	var p aiTtbPayload
	if json.Unmarshal(raw, &p) != nil || len(p.Levels) == 0 {
		return ""
	}
	levels := p.Levels
	if len(levels) > aiTtbMaxLevels {
		levels = levels[:aiTtbMaxLevels]
	}

	axis := 0.0
	for _, lvl := range levels {
		if lvl.Hours > axis {
			axis = lvl.Hours
		}
	}
	if p.Current != nil && p.Current.Hours > axis {
		axis = p.Current.Hours
	}
	if axis <= 0 {
		return ""
	}

	// COPY LAW: the label is the server's; none sent → none shown.
	label := strings.TrimSpace(p.Label)
	if label != "" {
		label += " " // the web's CSS gap, matching every other bar label here
	}

	var ticks []BarTick
	var values []positionedText
	var legendNames []string
	var legendColors []RGB
	for i, lvl := range levels {
		if lvl.Hours <= 0 {
			continue // an absent/zero level draws no tick, mirrors the game preset
		}
		pct := lvl.Hours / axis * 100
		color := aiTtbLevelColors[i%len(aiTtbLevelColors)]
		ticks = append(ticks, BarTick{Pct: pct, Color: color, Bold: true})
		values = append(values, positionedText{
			Text: formatAiTtbLevelHours(lvl.Hours), Pct: pct, AtEnd: pct > 90,
		})
		if name := strings.TrimSpace(lvl.Label); name != "" {
			legendNames = append(legendNames, name)
			legendColors = append(legendColors, color)
		}
	}
	if p.Current != nil {
		pct := p.Current.Hours / axis * 100
		if pct < 0 {
			pct = 0
		}
		ticks = append(ticks, BarTick{
			Pct: pct, Color: aiTtbCurrentColor, Bold: true,
			Chip:     formatAiTtbCurrentHours(p.Current.Hours),
			ChipLeft: pct >= 50,
		})
	}

	// A caller-supplied level list has no fixed cell budget the way the
	// game preset's 40-cell BAR_CELLS does — cap the bar's own width so a
	// wide message column doesn't stretch the fill into an absurdly long
	// run (scoreBarCap — the owner rule every score/TTB bar respects).
	barWidth := width
	if barWidth > scoreBarCap {
		barWidth = scoreBarCap
	}

	fill := StopGradient{Stops: aiTtbGradientStops(axis)}
	lines := []string{r.barLine(label, fill, ticks, barWidth, false)}
	if len(values) > 0 {
		lines = append(lines, r.positionRow(label, values, barWidth, lipgloss.NewStyle().Foreground(ColorDim)))
	}
	if len(legendNames) > 0 {
		items := make([]string, len(legendNames))
		for i, name := range legendNames {
			tickStyle := lipgloss.NewStyle().Bold(true)
			if r.truecolor {
				tickStyle = tickStyle.Foreground(hex(legendColors[i]))
			}
			items[i] = tickStyle.Render("|") + r.dim(" "+name)
		}
		lines = append(lines, strings.Repeat(" ", lipgloss.Width(label)+1)+strings.Join(items, "  "))
	}
	// The bar/values rows are already sized off barWidth, but the legend's
	// own natural width has no such ceiling — caller-supplied level names
	// (unlike the game preset's short main/extras/completionist labels)
	// can run long enough to overflow the block's column and force an
	// unwanted line-wrap further up the chain (aiChrome's own Width()
	// wraps rather than clips). Truncate every line here defensively, the
	// same ellipsis-not-wrap degrade aiTableBlock uses for oversize cells.
	for i, line := range lines {
		if width > 0 {
			lines[i] = ansi.Truncate(line, width, "…")
		}
	}
	return strings.Join(lines, "\n")
}

// aiTtbGradientStops projects the web's fixed HEAT_THRESHOLDS hour
// breakpoints (0/10/40/100h → green/lime/amber/pink) onto the bar's own
// axis (TimeToBeatComponent#gradient_stops, simplified: the full 4-stop
// ramp always renders here rather than truncating by the highest present
// pillar — a caller-supplied level list has no fixed pillar identity to
// truncate against). A short axis compresses the hot colors toward the
// left edge and clamps them there; the ramp always reaches its last
// color by position 1 so the fill never fades out before the bracket.
func aiTtbGradientStops(axis float64) []GradientStop {
	if axis < 10 {
		axis = 10 // TimeToBeatComponent#color_axis_max's own floor
	}
	thresholds := []struct {
		hours float64
		color RGB
	}{
		{0, heatGreen}, {10, heatLime}, {40, heatAmber}, {100, heatPink},
	}
	stops := make([]GradientStop, len(thresholds))
	for i, th := range thresholds {
		pos := th.hours / axis
		if pos > 1 {
			pos = 1
		}
		stops[i] = GradientStop{Color: th.color, Pos: pos}
	}
	if stops[len(stops)-1].Pos < 1 {
		stops = append(stops, GradientStop{Color: stops[len(stops)-1].Color, Pos: 1})
	}
	return stops
}

// formatAiTtbLevelHours ports label_for/hours_short ("%{n}h") — a
// level's hours truncate toward zero before stamping, matching Rails'
// levels_data building each entry with `.to_i`.
func formatAiTtbLevelHours(hours float64) string {
	return fmt.Sprintf("%dh", int(hours))
}

// formatAiTtbCurrentHours ports Pito::Formatter::FootageHours: whole
// numbers drop the decimal ("5h"), fractional values keep one decimal
// ("12.5h"); negative collapses to "0h" (footage never reads negative).
func formatAiTtbCurrentHours(hours float64) string {
	if hours < 0 {
		hours = 0
	}
	if hours == math.Trunc(hours) {
		return fmt.Sprintf("%dh", int64(hours))
	}
	return fmt.Sprintf("%.1fh", hours)
}
