// The prose-family @ai block painters (W2/D2): text, kv_table, table,
// suggestion, and the shared degrade fallback. Inline markup follows pito's
// content ontology verbatim (config/pito/content.yml, the worktree copy) —
// **bold**, *italic*, [cyan]/[red]/[green] color spans, [subject]/[ref]
// semantic tags, kaomoji pass through untouched. Every function decodes its
// own Raw slice and returns "" when it cannot render (nil-safe) — the
// caller (render/ai.go, wired by the orchestrator) dispatches on the
// block's "type", falls back to aiDegradeBlock for an unknown type or an
// empty result, and never lets a bad block error the whole message (the
// server already degrades malformed blocks on its side; the client must
// survive a server it has never seen too).
package render

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

// ── text ─────────────────────────────────────────────────────────────────

// inlineStyle is the active markup state while scanning a text block —
// independent attributes (not a strict tag stack) so overlapping spans like
// "[cyan]a [subject]b[/subject] c[/cyan]" apply sanely without real nesting.
type inlineStyle struct {
	bold, italic, subject, ref bool
	color                      string // "" (default) | cyan | red | green
}

type inlineSpan struct {
	text  string
	style inlineStyle
}

// aiInlineTags are the ONLY bracket notations content.yml grants meaning —
// anything else ("[foo]", a lone "[") is plain text, per the house rule.
var aiInlineTags = map[string]bool{
	"cyan": true, "red": true, "green": true, "subject": true, "ref": true,
}

// aiTextBlock renders a paragraph block: the model's inline notation
// (bold/italic/color spans/subject/ref) painted with the same primitives
// the html path uses (PitoShimmer for subject, ColorCyan for ref), word-
// wrapped to width with newlines preserved as paragraph breaks. Never runs
// through glamour or an HTML parser — text blocks carry no markdown
// headers, code fences, or raw HTML by contract; whatever arrives outside
// the five recognized notations shows up exactly as sent.
func (r *R) aiTextBlock(raw json.RawMessage, width int) string {
	var p struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return ""
	}
	text := sanitizeAiText(strings.TrimSpace(p.Text))
	if text == "" {
		return ""
	}
	if width < 1 {
		width = 1
	}
	spans := parseAiInline(text)
	lines := splitSpansByNewline(spans)
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = r.renderWrappedLine(line, width)
	}
	return strings.Join(out, "\n")
}

// sanitizeAiText strips anything a hostile or buggy payload could abuse to
// reach the terminal raw: C0/DEL control bytes (raw ANSI/escape injection)
// and the html flattener's own private-use marker runes (ShimmerStart et
// al, html.go) — AI text never legitimately carries those, so a crafted
// payload can't forge shimmer/token/coin styling through this path. The
// paragraph newline is the one control character kept.
func sanitizeAiText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, ru := range s {
		switch {
		case ru == '\n':
			b.WriteRune(ru)
		case ru < 0x20, ru == 0x7f:
		case ru >= 0xE000 && ru <= 0xF8FF:
		default:
			b.WriteRune(ru)
		}
	}
	return b.String()
}

// parseAiInline walks text once, byte-indexed (markers are all ASCII, so
// byte slicing never lands mid-rune; non-marker bytes — including every
// continuation byte of a multi-byte rune, kaomoji included — fall through
// to the default case and copy verbatim, which reconstructs them exactly).
func parseAiInline(text string) []inlineSpan {
	var spans []inlineSpan
	var buf strings.Builder
	style := inlineStyle{}
	flush := func() {
		if buf.Len() > 0 {
			spans = append(spans, inlineSpan{text: buf.String(), style: style})
			buf.Reset()
		}
	}
	i, n := 0, len(text)
	for i < n {
		switch {
		case strings.HasPrefix(text[i:], "**"):
			flush()
			style.bold = !style.bold
			i += 2
		case text[i] == '*':
			flush()
			style.italic = !style.italic
			i++
		case text[i] == '[':
			if name, closing, adv, ok := matchAiTag(text[i:]); ok {
				flush()
				applyAiTag(&style, name, closing)
				i += adv
				continue
			}
			buf.WriteByte(text[i])
			i++
		default:
			buf.WriteByte(text[i])
			i++
		}
	}
	flush()
	return spans
}

// matchAiTag recognizes "[name]" / "[/name]" for a known tag at the start
// of rest; ok is false for anything else (unknown tag, no closing ']',
// pathological length) — the caller then treats '[' as a literal rune.
func matchAiTag(rest string) (name string, closing bool, adv int, ok bool) {
	end := strings.IndexByte(rest, ']')
	if end < 0 || end > 12 {
		return "", false, 0, false
	}
	inner := rest[1:end]
	closing = strings.HasPrefix(inner, "/")
	name = strings.TrimPrefix(inner, "/")
	if !aiInlineTags[name] {
		return "", false, 0, false
	}
	return name, closing, end + 1, true
}

func applyAiTag(style *inlineStyle, name string, closing bool) {
	switch name {
	case "subject":
		style.subject = !closing
	case "ref":
		style.ref = !closing
	default: // cyan | red | green
		if closing {
			if style.color == name {
				style.color = ""
			}
		} else {
			style.color = name
		}
	}
}

// splitSpansByNewline breaks a flat span list at embedded '\n's into
// paragraph lines (blank lines survive as empty slices — the join step
// turns them back into blank output lines).
func splitSpansByNewline(spans []inlineSpan) [][]inlineSpan {
	lines := [][]inlineSpan{{}}
	for _, sp := range spans {
		parts := strings.Split(sp.text, "\n")
		for pi, part := range parts {
			if part != "" {
				last := len(lines) - 1
				lines[last] = append(lines[last], inlineSpan{text: part, style: sp.style})
			}
			if pi < len(parts)-1 {
				lines = append(lines, []inlineSpan{})
			}
		}
	}
	return lines
}

type styledWord struct {
	text  string
	style inlineStyle
}

// tokenizeLine splits one paragraph line's spans into words — style
// changes are assumed to land on whitespace boundaries (the model tags
// whole words/phrases, never mid-word), which keeps wrapping simple.
func tokenizeLine(line []inlineSpan) []styledWord {
	var words []styledWord
	for _, sp := range line {
		for _, w := range strings.Fields(sp.text) {
			words = append(words, styledWord{text: w, style: sp.style})
		}
	}
	return words
}

// renderWrappedLine greedy word-wraps one paragraph line to width, using
// each word's PLAIN width for the budget (styling is only rendered after
// the wrap decision, so ANSI codes never confuse the measurement — the
// wrapPlain trick, applied to styled runs instead of a flat string).
func (r *R) renderWrappedLine(line []inlineSpan, width int) string {
	words := tokenizeLine(line)
	if len(words) == 0 {
		return ""
	}
	var wrapped []string
	var cur []styledWord
	curWidth := 0
	flush := func() {
		if len(cur) == 0 {
			return
		}
		parts := make([]string, len(cur))
		for i, w := range cur {
			parts[i] = r.renderInlineWord(w)
		}
		wrapped = append(wrapped, strings.Join(parts, " "))
		cur = nil
		curWidth = 0
	}
	for _, w := range words {
		ww := lipgloss.Width(w.text)
		add := ww
		if len(cur) > 0 {
			add++
		}
		if len(cur) > 0 && curWidth+add > width {
			flush()
			cur = append(cur, w)
			curWidth = ww
		} else {
			cur = append(cur, w)
			curWidth += add
		}
	}
	flush()
	return strings.Join(wrapped, "\n")
}

// renderInlineWord paints one word per its accumulated style. subject wins
// over any simultaneous color tag — the shimmer treatment IS the emphasis,
// matching paintShimmer's own html-sourced rendering (never layered with a
// flat foreground override).
func (r *R) renderInlineWord(w styledWord) string {
	if w.style.subject {
		return r.renderSubjectText(w.text)
	}
	st := lipgloss.NewStyle().Bold(w.style.bold).Italic(w.style.italic)
	switch {
	case w.style.ref:
		st = st.Foreground(ColorCyan)
	case w.style.color == "cyan":
		st = st.Foreground(ColorCyan)
	case w.style.color == "red":
		st = st.Foreground(ColorErr)
	case w.style.color == "green":
		st = st.Foreground(ColorOK)
	}
	return st.Render(w.text)
}

// renderSubjectText is the [subject]…[/subject] treatment — 1:1 with
// paintShimmer's ShimmerEnd branch (render.go): the traveling PitoShimmer
// gradient on truecolor terminals, a static bold accent otherwise.
func (r *R) renderSubjectText(text string) string {
	if r.truecolor {
		return PitoShimmer.Colorize(text, r.phase)
	}
	return lipgloss.NewStyle().Foreground(ColorSubject).Bold(true).Render(text)
}

// ── kv_table ─────────────────────────────────────────────────────────────

// decodedKvValue is a kv_table cell after decoding: either plain text or a
// typed {v, format} pair — the format-specific renderer resolves later.
type decodedKvValue struct {
	typed  bool
	plain  string
	v      string
	format string
}

// aiKvTableBlock renders label→value rows: cyan keys (web parity, the
// house rule), alternating gray VALUE foregrounds (the same Charm-aligned
// look detail.go's kvRows uses — owner 2026-07-12 "align to Charm", no
// background stripe), plain values left-aligned and word-wrapped, typed
// values (price/date/number/score) formatted through pito's own house
// formatters and right-aligned within the value column. Rows are [key,
// value] arrays; anything else (an unparseable row, a blank key) drops
// silently rather than sinking the whole table.
func (r *R) aiKvTableBlock(raw json.RawMessage, width int) string {
	var p struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return ""
	}
	type kvOut struct {
		key        string
		text       string
		vwidth     int
		rightAlign bool
	}
	var rows []kvOut
	for _, rr := range p.Rows {
		key, val, ok := decodeAiKvRow(rr)
		if !ok {
			continue
		}
		text, vwidth, right := formatAiKvValue(val)
		rows = append(rows, kvOut{key: key, text: text, vwidth: vwidth, rightAlign: right})
	}
	if len(rows) == 0 {
		return ""
	}
	if width < 20 {
		width = 20
	}
	keyWidth := 0
	for _, row := range rows {
		if w := lipgloss.Width(row.key); w > keyWidth {
			keyWidth = w
		}
	}
	valWidth := width - keyWidth - 3
	if valWidth < 10 {
		valWidth = 10
	}
	var lines []string
	for i, row := range rows {
		keyStyle := lipgloss.NewStyle().Foreground(ColorCyan)
		valStyle := lipgloss.NewStyle()
		// Charm's own canonical look: no background stripe — alternating
		// gray foregrounds on the value column instead (keys keep the
		// house cyan, same as ever).
		if i%2 == 1 {
			valStyle = valStyle.Foreground(ColorDim)
		} else {
			valStyle = valStyle.Foreground(ColorFaint)
		}
		var valueLines []string
		if row.rightAlign {
			text := row.text
			if pad := valWidth - row.vwidth; pad > 0 {
				text = strings.Repeat(" ", pad) + text
			}
			valueLines = []string{text}
		} else {
			valueLines = wrapPlain(row.text, valWidth)
		}
		for li, vline := range valueLines {
			keyCell := strings.Repeat(" ", keyWidth+3)
			if li == 0 {
				pad := keyWidth - lipgloss.Width(row.key)
				keyCell = " " + row.key + strings.Repeat(" ", pad) + "  "
			}
			line := keyStyle.Render(keyCell) + r.paintTokens(vline, valStyle)
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// decodeAiKvRow reads one [key, value] row. Extra elements (a "command"
// third slot — the web's click-to-prefill affordance, out of this
// deliverable's scope) are ignored, not rejected.
func decodeAiKvRow(raw json.RawMessage) (key string, val decodedKvValue, ok bool) {
	var cells []json.RawMessage
	if err := json.Unmarshal(raw, &cells); err != nil || len(cells) < 2 {
		return "", decodedKvValue{}, false
	}
	var k string
	if err := json.Unmarshal(cells[0], &k); err != nil {
		return "", decodedKvValue{}, false
	}
	k = strings.TrimSpace(k)
	if k == "" {
		return "", decodedKvValue{}, false
	}
	return k, decodeAiKvValue(cells[1]), true
}

// decodeAiKvValue accepts a bare string or a typed {v, format} object.
func decodeAiKvValue(raw json.RawMessage) decodedKvValue {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return decodedKvValue{plain: s}
	}
	var typed struct {
		V      json.RawMessage `json:"v"`
		Format string          `json:"format"`
	}
	if err := json.Unmarshal(raw, &typed); err == nil && typed.Format != "" && len(typed.V) > 0 {
		return decodedKvValue{typed: true, v: rawValueToString(typed.V), format: typed.Format}
	}
	return decodedKvValue{plain: rawValueToString(raw)}
}

// rawValueToString reads a JSON scalar leniently (string, else number,
// else the raw bytes with any surrounding quotes trimmed) — the wire
// always sends "v" as a string, but tests and off-spec servers might not.
func rawValueToString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strings.Trim(string(raw), `"`)
}

// formatAiKvValue resolves a decoded value to its display text, that
// text's VISUAL width (marker runes like the price coins count for their
// glyph, not their byte length), and whether it right-aligns.
func formatAiKvValue(v decodedKvValue) (text string, width int, rightAlign bool) {
	if !v.typed {
		return v.plain, lipgloss.Width(v.plain), false
	}
	switch v.format {
	case "price":
		t, w := formatAiPrice(v.v)
		return t, w, true
	case "date":
		t := formatAiDate(v.v)
		return t, lipgloss.Width(t), true
	case "number":
		t := formatAiNumber(v.v)
		return t, lipgloss.Width(t), true
	case "score":
		t := formatAiScore(v.v)
		return t, lipgloss.Width(t), true
	default:
		// An unrecognized format never reaches a compliant server (Rails
		// degrades it first) — a hand-built payload just shows the raw
		// value rather than losing the row.
		return v.v, lipgloss.Width(v.v), false
	}
}

// formatAiPrice ports Pito::Games::PriceGlyphs 1:1: nil/negative → em-dash,
// an explicit 0 → the FREE star + "0.00", else N gold coins (the show-game
// tier thresholds) + the bare 2-decimal number — no € symbol, the coins
// ARE the currency mark. Renders through CoinMark/StarMark (html.go) so
// paintTokens' existing gold-gloss animation paints them, matching every
// other coin run in the app; width is computed directly (coin count + one
// gap + the number's width) since the marker runes aren't measurable by
// lipgloss.Width on their own.
func formatAiPrice(raw string) (string, int) {
	f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || f < 0 {
		return "—", 1
	}
	number := strconv.FormatFloat(f, 'f', 2, 64)
	if f == 0 {
		text := string(StarMark) + " " + number
		return text, 1 + 1 + lipgloss.Width(number)
	}
	count := aiPriceCoinCount(f)
	text := strings.Repeat(string(CoinMark), count) + " " + number
	return text, count + 1 + lipgloss.Width(number)
}

// aiPriceCoinCount mirrors Pito::Coin::TIERS — 1..5 coins on €9.99/29.99/
// 59.99/79.99 inclusive boundaries, 5 (MAX_TIER) above.
func aiPriceCoinCount(price float64) int {
	switch {
	case price <= 9.99:
		return 1
	case price <= 29.99:
		return 2
	case price <= 59.99:
		return 3
	case price <= 79.99:
		return 4
	default:
		return 5
	}
}

// aiDateLayouts covers the shapes a model is likely to send; anything else
// falls back to the raw string (Ruby's Date.parse rescue, ported: never
// error, just show what arrived).
var aiDateLayouts = []string{
	"2006-01-02",
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"01/02/2006",
	"2006/01/02",
	"Jan 2, 2006",
	"January 2, 2006",
	"Jan 2 2006",
}

// formatAiDate ports kv_table_block_component's Date#strftime("%b %-d,
// %Y") — short month, no leading zero on the day, full year ("Jul 10,
// 2026").
func formatAiDate(raw string) string {
	raw = strings.TrimSpace(raw)
	for _, layout := range aiDateLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format("Jan 2, 2006")
		}
	}
	return raw
}

// formatAiNumber ports Pito::Formatter::CompactCount: round to the nearest
// int (Ruby's to_f.round, half away from zero — math.Round matches), then
// K/M/B-compact, always FLOORED so the shown value never overstates the
// real count.
func formatAiNumber(raw string) string {
	f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		f = 0 // String#to_f's own leniency: unparseable → 0, never an error
	}
	return aiKvCompactCount(int64(math.Round(f)))
}

// aiKvCompactCount ports Pito::Formatter::CompactCount specifically (kept
// distinct from ai_charts.go's compactCount, which ports the UNRELATED
// Analytics::Visualizers::Area#compact_count axis-tick formatter — same
// shape, different Ruby module, different rules: this one floors and
// scales through K/M/B, that one rounds and stops at K).
func aiKvCompactCount(n int64) string {
	if n == 0 {
		return "0"
	}
	// Ruby's guard is `n < 1_000` with no lower bound — ANY negative also
	// satisfies it, so negatives of any magnitude print as plain digits,
	// never compacted. Porting that quirk verbatim, not "fixing" it.
	if n < 1_000 {
		return strconv.FormatInt(n, 10)
	}
	switch {
	case n < 1_000_000:
		return aiKvCompactTier(n, 1_000, "K")
	case n < 1_000_000_000:
		return aiKvCompactTier(n, 1_000_000, "M")
	default:
		return aiKvCompactTier(n, 1_000_000_000, "B")
	}
}

func aiKvCompactTier(n, unit int64, suffix string) string {
	scaled := float64(n) / float64(unit)
	if scaled < 10 {
		tenths := int64(math.Floor(scaled * 10))
		whole, frac := tenths/10, tenths%10
		if frac == 0 {
			return strconv.FormatInt(whole, 10) + suffix
		}
		return strconv.FormatInt(whole, 10) + "." + strconv.FormatInt(frac, 10) + suffix
	}
	return strconv.FormatInt(int64(math.Floor(scaled)), 10) + suffix
}

// formatAiScore ports kv_table_block_component's `value["v"].to_i.to_s` —
// a bare integer, NOT clamped to 0..100 (unlike the standalone score/heart
// blocks) and with no "/100" suffix.
func formatAiScore(raw string) string {
	f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		f = 0 // String#to_i's leniency: unparseable → 0
	}
	return strconv.FormatInt(int64(f), 10) // truncates toward zero, like Ruby's #to_i
}

// ── table ────────────────────────────────────────────────────────────────

// aiTableBlock renders a header+rows grid — the same rounded, horizontal-
// rules-only frame the system list tables use (render.go's r.table), but
// sized off the explicit width param rather than r.width: AI blocks can
// render narrower than the full message column, so the width this
// function honors is whatever the caller hands it, not the renderer's own
// fixed content width. Oversize content degrades exactly like the system
// tables do — lipgloss/table truncates cells with an ellipsis rather than
// wrapping a spilled column outside the frame.
func (r *R) aiTableBlock(raw json.RawMessage, width int) string {
	var p struct {
		Header []string   `json:"header"`
		Rows   [][]string `json:"rows"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return ""
	}
	if len(p.Header) == 0 || len(p.Rows) == 0 {
		return ""
	}
	rows := make([][]string, len(p.Rows))
	for i, row := range p.Rows {
		cells := make([]string, len(p.Header))
		copy(cells, row)
		rows[i] = cells
	}
	return renderAiTable(p.Header, rows, width, r.truecolor)
}

// renderAiTable builds an AI table block through lipgloss/table — the same
// rounded, horizontal-rules-only frame the system list tables use
// (render.go's r.table), Charm's own canonical bold-purple-header /
// alternating-gray-rows look (owner 2026-07-12 "align to Charm"), no
// background zebra.
func renderAiTable(header []string, rows [][]string, width int, truecolor bool) string {
	build := func(w int) string {
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(tablePurple(truecolor))).
			BorderColumn(false).
			BorderRow(false).
			BorderLeft(false).
			BorderRight(false).
			BorderHeader(true).
			// Never wrap: constrained cells truncate with … — the same
			// choice r.table makes for the system list tables.
			Wrap(false).
			StyleFunc(func(row, col int) lipgloss.Style {
				st := lipgloss.NewStyle().Padding(0, 1)
				if row == table.HeaderRow {
					return st.Foreground(tablePurple(truecolor)).Bold(true)
				}
				if row%2 == 1 {
					return st.Foreground(ColorDim)
				}
				return st.Foreground(ColorFaint)
			}).
			Headers(header...)
		for _, row := range rows {
			t = t.Row(row...)
		}
		if w > 0 {
			t = t.Width(w)
		}
		return t.Render()
	}
	avail := width
	if avail < 6 {
		avail = 6
	}
	out := build(0)
	if lipgloss.Width(out) > avail {
		out = build(avail)
	}
	return out
}

// ── suggestion ───────────────────────────────────────────────────────────

// aiSuggestionBlock renders one ready-to-run command as a numbered line
// ("1. show game 12") with its optional note indented beneath — index is
// the 0-based position among the message's suggestion blocks (displayed
// as index+1; ≤5 per message by contract, so the "N. " prefix is always 3
// columns wide).
func (r *R) aiSuggestionBlock(raw json.RawMessage, index int, width int) string {
	var p struct {
		Command string `json:"command"`
		Note    string `json:"note"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return ""
	}
	cmd := strings.TrimSpace(p.Command)
	if cmd == "" {
		return ""
	}
	line := r.dim(strconv.Itoa(index+1)+". ") + r.accent(cmd)
	note := strings.TrimSpace(p.Note)
	if note == "" {
		return line
	}
	noteWidth := width - 3
	if noteWidth < 10 {
		noteWidth = 10
	}
	for _, wline := range wrapPlain(note, noteWidth) {
		line += "\n   " + r.dim(wline)
	}
	return line
}

// ── degrade ──────────────────────────────────────────────────────────────

// aiDegradeBlock renders raw block JSON as a dim, pretty-printed line —
// the client-side half of "malformed block → dim degrade, never error"
// (the server degrades its own side; the caller here reaches for this
// when a block's type is unrecognized or its dedicated renderer returned
// "").
func (r *R) aiDegradeBlock(raw json.RawMessage, width int) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		pretty.Reset()
		pretty.Write(raw)
	}
	text := strings.TrimSpace(pretty.String())
	if text == "" {
		return ""
	}
	if width < 1 {
		width = 1
	}
	return lipgloss.NewStyle().Foreground(ColorDim).Render(text)
}
