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
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/ansi"
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

// aiKvKeyMaxWidth is the KV table's KEY column PRESSURE FLOOR, not a cap
// (owner decree 2026-07-19, overruling the web's unconditional 20-cell cap
// this constant used to mirror — kv_table_block_component.rb's KEY_CLASS,
// "... max-w-[20ch]"): on a wide/desktop terminal a long AI-authored key
// renders in FULL, never truncated — truncation only kicks in under real
// width pressure, and even then the key only shrinks toward this floor by
// exactly as much as the value column actually needs (aiKvKeyWidth). The
// old unconditional 20-cell cap regressed the desktop case the day after
// it landed; aiKvKeyWidth is the fix.
const aiKvKeyMaxWidth = 20

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
// formatters and right-aligned within the value column. The KEY column's
// width is width-AWARE (owner decree 2026-07-19, aiKvKeyWidth): on a wide
// terminal a long key renders in FULL, never truncated; only under real
// width pressure does it shrink toward aiKvKeyMaxWidth (now a pressure
// floor, not a cap — see its own doc comment), and only by as much as the
// value column actually needs. Rows are [key, value] arrays; anything else
// (an unparseable row, a blank key) drops silently rather than sinking the
// whole table.
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
	// The house date format's "today"/"current year" collapse needs a
	// clock — read it once per block, same seam stamp() reads (render.go),
	// then thread it down as a plain parameter (formatAiKvValue/formatAiDate
	// stay pure functions, not methods, so they stay directly testable).
	now := r.now()
	var rows []kvOut
	for _, rr := range p.Rows {
		key, val, ok := decodeAiKvRow(rr)
		if !ok {
			continue
		}
		text, vwidth, right := formatAiKvValue(val, now)
		rows = append(rows, kvOut{key: key, text: text, vwidth: vwidth, rightAlign: right})
	}
	if len(rows) == 0 {
		return ""
	}
	if width < 20 {
		width = 20
	}
	naturalKey, naturalVal := 0, 0
	for _, row := range rows {
		if w := lipgloss.Width(row.key); w > naturalKey {
			naturalKey = w
		}
		// row.vwidth is already the value's own single-line display width
		// for BOTH shapes — formatAiKvValue returns a typed value's
		// FORMATTED width, and a plain value's FULL UNWRAPPED width (it
		// measures v.plain directly, never a wrapped line) — exactly the
		// "naturalVal" the width-aware key rule needs, no extra pass.
		if row.vwidth > naturalVal {
			naturalVal = row.vwidth
		}
	}
	keyWidth := aiKvKeyWidth(naturalKey, naturalVal, width)
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
				// A key over the (already-capped) column width truncates
				// with an ellipsis — same helper/behavior as ai.go's
				// aiTtbBlock legend truncation (github.com/charmbracelet/
				// x/ansi.Truncate) — so it never blows the row past
				// keyWidth and every row still aligns on the capped width.
				key := row.key
				if lipgloss.Width(key) > keyWidth {
					key = ansi.Truncate(key, keyWidth, "…")
				}
				pad := keyWidth - lipgloss.Width(key)
				if pad < 0 {
					pad = 0
				}
				keyCell = " " + key + strings.Repeat(" ", pad) + "  "
			}
			line := keyStyle.Render(keyCell) + r.paintTokens(vline, valStyle)
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// aiKvKeyWidth is the kv_table KEY column's pure, deterministic width
// allocator (owner decree 2026-07-19: the web's unconditional 20-cell cap
// is overruled — on a wide/desktop terminal a long key renders in FULL,
// never truncated; truncation only kicks in under real width pressure).
// Same inputs always produce the same width, so it's directly testable
// apart from any row rendering.
//
// naturalKey is the widest key's own display width, naturalVal the
// widest single-line VALUE width (aiKvTableBlock's own census — see its
// call site). When the row comfortably fits both at their natural size —
// naturalKey + the "key ↔ value" gutter (3 cells) + at least 10 cells for
// the value — the key gets its full natural width and NEVER truncates:
// the desktop case the owner caught regressing.
//
// Otherwise the key shrinks toward aiKvKeyMaxWidth (now a PRESSURE FLOOR,
// not a cap — see its own doc comment) by exactly as much as the value
// column actually needs: never below that floor, and never above the
// key's own natural width (padding the key column past what any key
// needs would waste the row's budget for nothing — aiKvClampKeyWidth). If
// that still leaves the value under its 10-cell floor, the key gives up
// the rest of the room the value's floor demands, down to the same
// 20-cell floor and no further; aiKvTableBlock's existing valWidth
// floor/wrap machinery (wrapPlain) takes it from there.
func aiKvKeyWidth(naturalKey, naturalVal, width int) int {
	if naturalKey+3+max(naturalVal, 10) <= width {
		return naturalKey
	}
	keyWidth := aiKvClampKeyWidth(width-3-naturalVal, naturalKey)
	if width-keyWidth-3 < 10 {
		keyWidth = aiKvClampKeyWidth(width-3-10, naturalKey)
	}
	return keyWidth
}

// aiKvClampKeyWidth bounds v to [aiKvKeyMaxWidth, natural]: never below
// the pressure floor, and never above the key's own widest natural width
// — a key already shorter than the floor needs no padding to reach it,
// natural is the tighter bound in that case.
func aiKvClampKeyWidth(v, natural int) int {
	v = max(v, aiKvKeyMaxWidth)
	return min(v, natural)
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
// glyph, not their byte length), and whether it right-aligns. now is the
// clock the "date" format's house today/current-year collapse decides
// against (R.now, threaded from aiKvTableBlock — see its own doc comment);
// every other format ignores it. A PLAIN (untyped) value right-aligns the
// same way an @ai table cell does — aiTableCellAligns' three shape
// families (numeric, #id, date/time) — so a model-authored plain "#38",
// "19 Jul 12:00", or "7,709" value aligns without ever being promoted to a
// typed cell; keys are never tested, only values.
func formatAiKvValue(v decodedKvValue, now time.Time) (text string, width int, rightAlign bool) {
	if !v.typed {
		return v.plain, lipgloss.Width(v.plain), aiTableCellAligns(v.plain)
	}
	switch v.format {
	case "price":
		t, w := formatAiPrice(v.v)
		return t, w, true
	case "date":
		t := formatAiDate(v.v, now)
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

// aiISODateRe / aiDMYDateRe / aiISODateTimeRe / aiDMYDateTimeRe port
// kv_table_block_component.rb#formatted_date's four strict shape regexes
// (ISO_DATE / ISO_DATETIME / DMY_DATE / DMY_DATETIME) 1:1 — Ai::Blocks.
// kv_value only emits/promotes these four shapes on the wire, so anything
// else was never a real date and must show verbatim, same as the web.
var (
	aiISODateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	aiDMYDateRe = regexp.MustCompile(`^\d{2}-\d{2}-\d{4}$`)
	// Captures: 1=date+"T"hh:mm, 2=optional ":ss", 3=optional ".frac"
	// (dropped — no house date shape carries sub-minute precision),
	// 4=optional zone ("Z", or a +/-hh[:]mm offset with or without the
	// colon — both wire forms the Ruby regex admits).
	aiISODateTimeRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2})(:\d{2})?(\.\d+)?(Z|[+-]\d{2}:?\d{2})?$`)
	aiDMYDateTimeRe = regexp.MustCompile(`^\d{2}-\d{2}-\d{4} \d{2}:\d{2}$`)
)

// formatAiDate renders a typed kv_table date value in the house date format
// (owner decree) — the successor to the DD-MM-YYYY shape this port shipped
// yesterday (itself ported from a web formatter — kv_table_block_
// component.rb#formatted_date — the owner has since superseded). The house
// rule generalizes TimestampPrefixComponent (pito,
// timestamp_prefix_component.rb:38-47) to every date on every surface:
//
//   - date-only (no time component): "2 Jan" when the year matches now's,
//     "2 Jan '06" for any other year, past or future — a date-only value
//     NEVER collapses on "today" (dropping the date would leave nothing to
//     show, unlike a datetime, where the trailing HH:MM survives).
//   - datetime: bare "15:04" when it falls on now's calendar day (the date
//     drops entirely), "2 Jan 15:04" for any other day within now's year,
//     "2 Jan '06 15:04" for any other year.
//
// now is the caller's clock seam (R.now, threaded down from aiKvTableBlock
// through formatAiKvValue — the same seam stamp() reads in render.go)
// rather than a global, so this stays a pure, directly testable function. A
// zone-bearing instant converts to the local zone BEFORE the today/current-
// year decision (parseAiISODateTime's own job — TimestampPrefixComponent's
// `in_time_zone` equivalent); a zoneless datetime keeps its wall clock
// untouched, matching Rails treating a zoneless string as already local.
// Only the four strict shapes above are typed dates on the wire; ANY other
// string — and a parse failure on a shape that DID match, an invalid
// calendar date included — returns the raw string unchanged, Ruby's rescue
// branch ported verbatim.
//
// One narrow, deliberate divergence, verified against the live component
// rather than assumed: Ruby's Time.new / Time.strptime silently normalize
// an out-of-range day via calendar overflow (e.g. "2026-02-30" becomes
// 2 Mar 2026 rather than raising), while Go's time.Parse rejects it
// outright. Porting Ruby's forgiving arithmetic would buy an edge case no
// real typed-date payload produces at the cost of a second parse path with
// different error semantics per shape — this port keeps all four shapes
// uniformly strict instead: an invalid calendar date shows raw, same as any
// other unparseable value.
func formatAiDate(raw string, now time.Time) string {
	str := strings.TrimSpace(raw)
	switch {
	case aiISODateRe.MatchString(str):
		if t, err := time.Parse("2006-01-02", str); err == nil {
			return houseDateOnly(t, now)
		}
	case aiDMYDateRe.MatchString(str):
		if t, err := time.Parse("02-01-2006", str); err == nil {
			return houseDateOnly(t, now)
		}
	case aiISODateTimeRe.MatchString(str):
		if t, ok := parseAiISODateTime(str); ok {
			return houseDateTime(t, now)
		}
	case aiDMYDateTimeRe.MatchString(str):
		if t, err := time.Parse("02-01-2006 15:04", str); err == nil {
			return houseDateTime(t, now)
		}
	}
	return str
}

// houseDateOnly renders a date-only value per the house date rule: the
// day+abbreviated-month with no leading zero, no year, when the year
// matches now's; the two-digit apostrophe year appended for any OTHER
// year, past or future. A date-only value never collapses on "today" —
// unlike houseDateTime, there is no trailing clock time left to carry the
// row once the date drops.
func houseDateOnly(t, now time.Time) string {
	if sameYear(t.Year(), now) {
		return t.Format("2 Jan")
	}
	return t.Format("2 Jan '06")
}

// houseDateTime renders a datetime value per the house date rule
// (TimestampPrefixComponent, timestamp_prefix_component.rb:38-47): today
// collapses to a bare "15:04" (the date drops entirely), the current year
// keeps day+month without a year, any other year appends the two-digit
// apostrophe year. t must already be converted to the caller's local zone
// (parseAiISODateTime does this for zoned instants) — today/current-year is
// decided on that already-converted value, same as sameYear/YearDay pair
// stamp() itself keys off.
func houseDateTime(t, now time.Time) string {
	switch {
	case sameYear(t.Year(), now) && t.YearDay() == now.YearDay():
		return t.Format("15:04")
	case sameYear(t.Year(), now):
		return t.Format("2 Jan 15:04")
	default:
		return t.Format("2 Jan '06 15:04")
	}
}

// parseAiISODateTime finishes the ISO_DATETIME branch: seconds default to
// ":00" when absent (Rails' Time.zone.iso8601 — verified against the live
// component — does NOT require seconds, unlike plain Ruby Time.iso8601),
// the fractional part is dropped (none of the house date shapes carry
// sub-minute precision), and a bare "+HHMM" offset is normalized to
// "+HH:MM" so Go's "Z07:00" layout token — which accepts either a literal
// "Z" or a colon-bearing offset, never a colonless one — can parse
// whichever wire form the regex admitted.
func parseAiISODateTime(str string) (time.Time, bool) {
	m := aiISODateTimeRe.FindStringSubmatch(str)
	if m == nil {
		return time.Time{}, false
	}
	base, secs, zone := m[1], m[2], m[4]
	if secs == "" {
		secs = ":00"
	}
	layout, clean := "2006-01-02T15:04:05", base+secs
	if zone != "" {
		layout += "Z07:00"
		if zone != "Z" && !strings.Contains(zone, ":") {
			zone = zone[:3] + ":" + zone[3:]
		}
		clean += zone
	}
	t, err := time.Parse(layout, clean)
	if err != nil {
		return time.Time{}, false
	}
	if zone != "" {
		// TimestampPrefixComponent's `in_time_zone` (pito,
		// timestamp_prefix_component.rb:38): a zoned instant renders in
		// the app-local zone, never the offset it arrived with.
		t = t.In(time.Local)
	}
	// Zoneless keeps Parse's own wall clock as-is (Parse tags it UTC with
	// no source offset to convert from — exactly Rails treating a
	// zoneless string as already expressed in the local zone).
	return t, true
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

// aiTableNumericCellRe ports table_block_component.rb's NUMERIC_CELL 1:1 —
// digits with optional grouping/decimals, then an optional K/M/B/% suffix,
// case-insensitive (the shapes the model actually sends: "7,709", "2.2K",
// "93%"). Verbatim quirk kept: a cell of bare commas/dots ("...") matches
// in Ruby (`[\d,.]+` needs no actual digit) and must match here too.
var aiTableNumericCellRe = regexp.MustCompile(`(?i)^\s*[\d,.]+\s*[kmb%]?\s*$`)

// aiTableIDCellRe ports table_block_component.rb's (extended) ID_CELL 1:1 —
// a bare "#" + digits and nothing else, e.g. "#38": the shape the system
// vid/game/channel list tables already right-align server-side. The "#"
// must own the WHOLE cell — "TEKKEN #38" does NOT match, a stray id token
// riding along inside prose is not the id column.
var aiTableIDCellRe = regexp.MustCompile(`^\s*#\d+\s*$`)

// aiTableDateCellRe ports table_block_component.rb's (extended) DATE_CELL
// 1:1 — any of four date/time shapes an @ai cell may carry: the house
// date/time stamp ("%-d %b" optionally + " 'YY" optionally + " HH:MM" —
// "2 Jan", "19 Jul 12:00", "5 Jun '25 12:00"), a bare clock time ("HH:MM"),
// an ISO date ("YYYY-MM-DD") optionally followed by "T" or " " + time, and
// a DMY date ("DD-MM-YYYY") optionally followed by " " + time. The ISO/DMY
// shapes are what frozen old payloads and model-sent cells still carry even
// though the house formatters (formatAiDate below, HouseDate on the Ruby
// side) now emit the first shape — all four classify a column as date/time,
// and cells may mix shapes within one column.
var aiTableDateCellRe = regexp.MustCompile(
	`^\s*(?:` +
		`\d{1,2} (?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)(?: '\d{2})?(?: \d{2}:\d{2})?` +
		`|\d{2}:\d{2}` +
		`|\d{4}-\d{2}-\d{2}(?:[T ]\d{2}:\d{2})?` +
		`|\d{2}-\d{2}-\d{4}(?: \d{2}:\d{2})?` +
		`)\s*$`,
)

// aiTableCellAligns is the alignment test a single cell must pass —
// numeric, #id, or date/time (a plain OR across the three families, not one
// combined regex, since a column's cells may mix families — a date column
// carrying both the house shape and a frozen DMY payload, say). Shared by
// aiTableAlignedCols' column census below AND formatAiKvValue's plain-value
// path (kv_table plain string values right-align the same way a typed
// value does, on the same three families).
func aiTableCellAligns(cell string) bool {
	return aiTableNumericCellRe.MatchString(cell) || aiTableIDCellRe.MatchString(cell) || aiTableDateCellRe.MatchString(cell)
}

// aiTableAlignedCols ports table_block_component.rb#align's column census
// (renamed from aiTableNumericCols now that alignment covers three shape
// families, not just numbers) — a column counts as ALIGNED when it has at
// least one non-empty BODY cell and every non-empty body cell matches
// aiTableCellAligns (numeric, #id, or date/time — cells may mix families
// within one column). An all-empty column (never any body content) left-
// aligns, same as the Ruby `cells.any?` guard.
func aiTableAlignedCols(header []string, rows [][]string) []bool {
	aligned := make([]bool, len(header))
	for col := range header {
		any := false
		all := true
		for _, row := range rows {
			if col >= len(row) {
				continue
			}
			cell := strings.TrimSpace(row[col])
			if cell == "" {
				continue
			}
			any = true
			if !aiTableCellAligns(cell) {
				all = false
				break
			}
		}
		aligned[col] = any && all
	}
	return aligned
}

// aiTableBlock renders a header+rows grid — the same rounded, horizontal-
// rules-only frame the system list tables use (render.go's r.table), but
// sized off the explicit width param rather than r.width: AI blocks can
// render narrower than the full message column, so the width this
// function honors is whatever the caller hands it, not the renderer's own
// fixed content width. Oversize content is handled by pito's own explicit
// column-width allocator (aiTableColumnWidths, owner: "consider number of
// columns and viewport") BEFORE lipgloss ever sees the table — never
// lipgloss/table's own total-Width() squeeze, which shrinks every column
// (aligned ones included) toward the median with no notion of which
// columns can actually afford to give something up.
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

// aiTableRigidFloor: a column whose natural width is already at or under
// this many cells classifies RIGID in aiTableColumnWidths regardless of
// content — squeezing an already-short column (a lone digit, a "—") buys
// the table nothing and just mangles it.
const aiTableRigidFloor = 8

// aiTableFlexFloor is the hard floor a FLEXIBLE column is squeezed to —
// below this a text column stops being legible at all. Aligned columns
// (numeric, #id, date/time — and any column under aiTableRigidFloor) never
// reach this floor because they're RIGID: aiTableColumnWidths never touches
// their width.
const aiTableFlexFloor = 6

// aiTablePadding is a single column's horizontal padding cost, matching
// renderAiTable's own StyleFunc (Padding(0, 1): 1 cell left + 1 right).
// aiTableFrame is the table's own border cost: BorderLeft/Right/Column
// are all false below (only the top/bottom rounded corners and the
// header rule survive, neither of which costs horizontal width), so it's
// zero — kept as its own named term so the fit check below reads the same
// as the owner's spec ("naturals + per-column padding + frame").
const (
	aiTablePadding = 2
	aiTableFrame   = 0
)

// aiTableColumnWidths is the @ai table block's pure, deterministic
// column-width allocator (owner: "consider number of columns and
// viewport") — same inputs always produce the same widths, so it's
// directly unit-testable apart from any lipgloss rendering.
//
// natural[i] is column i's widest display width across its header and
// every body cell (renderAiTable's own per-column census). aligned[i]
// flags an ALIGNED column (aiTableAlignedCols: numeric, #id, or date/time)
// — RIGID, because a truncated "7,709", "#38", or "19 Jul 12:00" would
// misreport the very value it shows.
// headerWidth[i] is column i's header cell width alone (its soft floor
// once squeezed). avail is the total width budget for cell CONTENT ONLY
// — the caller has already subtracted per-column padding and the frame
// (renderAiTable).
//
// When the naturals already fit avail, every column keeps its natural
// width untouched (renderAiTable skips calling this in that case, but the
// identity holds here too, so the function is correct standalone).
// Otherwise: RIGID columns (aligned, or natural <= aiTableRigidFloor)
// never move. Every other column is FLEXIBLE and absorbs the entire
// deficit between them, proportional to how far each sits above the fair
// per-column share of the space left after paying the rigid columns in
// full (fairShare) — the columns that most exceed fairShare give up the
// most, and a column already at or under fairShare gives up nothing. Each
// flexible column is clamped at its own soft floor (its header width, or
// aiTableFlexFloor if that's higher) during this pass; if the clamps
// themselves push the total back over avail — the header floors can't all
// be honored, "mathematically impossible" per the spec — a final
// deterministic settle loop shrinks the currently-widest flexible column
// one cell at a time (ties break on column order) until the width fits or
// every flexible column has been driven down to the hard floor, whichever
// comes first.
func aiTableColumnWidths(natural []int, aligned []bool, headerWidth []int, avail int) []int {
	allocated := make([]int, len(natural))
	copy(allocated, natural)

	naturalTotal := 0
	for _, w := range natural {
		naturalTotal += w
	}
	if naturalTotal <= avail {
		return allocated
	}

	rigid := make([]bool, len(natural))
	rigidTotal := 0
	var flex []int
	for i, w := range natural {
		if (i < len(aligned) && aligned[i]) || w <= aiTableRigidFloor {
			rigid[i] = true
			rigidTotal += w
			continue
		}
		flex = append(flex, i)
	}
	if len(flex) == 0 {
		return allocated // nothing left to squeeze; rigid columns never truncate
	}

	remaining := max(0, avail-rigidTotal)
	fairShare := remaining / len(flex)
	deficit := naturalTotal - avail

	excess := make([]int, len(natural))
	totalExcess := 0
	for _, i := range flex {
		if natural[i] > fairShare {
			excess[i] = natural[i] - fairShare
			totalExcess += excess[i]
		}
	}

	for _, i := range flex {
		cut := deficit / len(flex) // fallback: no column exceeds fairShare
		if totalExcess > 0 {
			cut = deficit * excess[i] / totalExcess
		}
		floor := max(aiTableFlexFloor, headerWidth[i])
		allocated[i] = max(natural[i]-cut, floor)
	}

	for {
		total := rigidTotal
		for _, i := range flex {
			total += allocated[i]
		}
		if total <= avail {
			break
		}
		idx, widest := -1, aiTableFlexFloor
		for _, i := range flex {
			if allocated[i] > widest {
				idx, widest = i, allocated[i]
			}
		}
		if idx < 0 {
			break // every flexible column is already at the hard floor
		}
		allocated[idx]--
	}

	return allocated
}

// renderAiTable builds an AI table block through lipgloss/table — the same
// rounded, horizontal-rules-only frame the system list tables use
// (render.go's r.table), Charm's own canonical bold-purple-header /
// alternating-gray-rows look (owner 2026-07-12 "align to Charm"), no
// background zebra. Aligned columns — numeric, #id, or date/time
// (table_block_component.rb#align) — right-align on both the header and
// the body — web parity, "pito's own table law", and NEVER truncate
// (aiTableColumnWidths' own RIGID rule). Column widths are decided by the
// explicit allocator BEFORE lipgloss ever builds a table: cells that need
// trimming are pre-truncated with ansi.Truncate, then the table renders
// through its natural (auto-detected) width — never lipgloss/table's own
// .Width(w) squeeze.
func renderAiTable(header []string, rows [][]string, width int, truecolor bool) string {
	aligned := aiTableAlignedCols(header, rows)

	natural := make([]int, len(header))
	headerWidth := make([]int, len(header))
	for i, h := range header {
		headerWidth[i] = lipgloss.Width(h)
		natural[i] = headerWidth[i]
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(natural) {
				continue
			}
			if w := lipgloss.Width(cell); w > natural[i] {
				natural[i] = w
			}
		}
	}
	naturalTotal := 0
	for _, w := range natural {
		naturalTotal += w
	}

	avail := max(width, 6)
	paddingTotal := len(header) * aiTablePadding

	displayHeader, displayRows := header, rows
	if naturalTotal+paddingTotal+aiTableFrame > avail {
		contentAvail := max(0, avail-paddingTotal-aiTableFrame)
		allocated := aiTableColumnWidths(natural, aligned, headerWidth, contentAvail)

		displayHeader = make([]string, len(header))
		for i, h := range header {
			displayHeader[i] = ansi.Truncate(h, allocated[i], "…")
		}
		displayRows = make([][]string, len(rows))
		for r, row := range rows {
			out := make([]string, len(row))
			for i, cell := range row {
				if i >= len(allocated) {
					out[i] = cell
					continue
				}
				out[i] = ansi.Truncate(cell, allocated[i], "…")
			}
			displayRows[r] = out
		}
	}

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(tablePurple(truecolor))).
		BorderColumn(false).
		BorderRow(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(true).
		// Never wrap: cells were already pre-truncated with … by the
		// allocator above, so there's nothing left for wrapping to do —
		// the same no-wrap choice r.table makes for the system list
		// tables.
		Wrap(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			st := lipgloss.NewStyle().Padding(0, 1)
			if col < len(aligned) && aligned[col] {
				st = st.Align(lipgloss.Right)
			}
			if row == table.HeaderRow {
				return st.Foreground(tablePurple(truecolor)).Bold(true)
			}
			if row%2 == 1 {
				return st.Foreground(ColorDim)
			}
			return st.Foreground(ColorFaint)
		}).
		Headers(displayHeader...)
	for _, row := range displayRows {
		t = t.Row(row...)
	}
	return t.Render()
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
