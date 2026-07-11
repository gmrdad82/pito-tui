// Package render turns canonical events into terminal blocks. One renderer
// per known kind, *_follow_up variants reuse their base, and everything
// else — including kinds this client has never heard of — falls back to a
// generic payload dump. Novelty must never crash: renderers decode the raw
// payload themselves and degrade on any mismatch.
package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// R renders events at a fixed width. Rebuild it on terminal resize — the
// glamour renderer word-wraps at construction time.
type R struct {
	width      int
	plain      bool
	truecolor  bool
	phase      float64
	revealFrac float64 // current event's grow-in (1 = settled)
	reveal     map[int64]float64
	style      string
	glam       *glamour.TermRenderer

	echoBar      lipgloss.Style
	systemBar    lipgloss.Style
	enhancedBar  lipgloss.Style
	errorStyle   lipgloss.Style
	dimStyle     lipgloss.Style
	confirmStyle lipgloss.Style
}

// Option configures an R.
type Option func(*R)

// WithPlain disables glamour markdown rendering (deterministic output for
// golden tests; also the safe path if glamour ever misbehaves).
func WithPlain() Option {
	return func(r *R) { r.plain = true }
}

// WithTruecolor enables gradient/shimmer painting (COLORTERM-detected by
// the app before Bubble Tea starts; 256-color terminals get static
// accents instead).
func WithTruecolor(on bool) Option {
	return func(r *R) { r.truecolor = on }
}

// WithStyle picks the glamour style ("dark"/"light"). The caller resolves
// it ONCE before Bubble Tea takes the terminal — glamour's auto style
// queries the background over stdin, which deadlocks against tea's input
// reader (the "loading…" freeze).
func WithStyle(style string) Option {
	return func(r *R) { r.style = style }
}

// New builds a renderer for the given content width.
func New(width int, opts ...Option) *R {
	if width < 20 {
		width = 20
	}
	bar := func(color lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().
			Border(lipgloss.ThickBorder(), false, false, false, true).
			BorderForeground(color).
			PaddingLeft(1).Width(width - 2)
	}
	r := &R{
		width:      width,
		revealFrac: 1, // fully revealed unless an animation says otherwise
		// Mirrors the web's block language: every message is a left-bar
		// block — echo in the user accent, replies in their own colors —
		// with the timestamp inside.
		echoBar:      bar(ColorAccent),
		systemBar:    bar(ColorFaint),
		enhancedBar:  bar(ColorPrimary),
		errorStyle:   bar(ColorErr).Foreground(ColorErr),
		dimStyle:     lipgloss.NewStyle().Foreground(ColorDim).Width(width),
		confirmStyle: bar(ColorWarn),
	}
	for _, opt := range opts {
		opt(r)
	}
	if !r.plain {
		style := r.style
		if style == "" {
			style = "dark"
		}
		// Best-effort: glamour failure downgrades to plain text forever
		// rather than erroring per event. NEVER WithAutoStyle here — it
		// queries the terminal on stdin and deadlocks under Bubble Tea.
		if g, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(style),
			glamour.WithWordWrap(width),
		); err == nil {
			r.glam = g
		}
	}
	return r
}

// SetPhase advances the shimmer sweep (one animation tick). The model
// drives it only while fresh shimmer is on screen.
func (r *R) SetPhase(phase float64) { r.phase = phase }

// paintShimmer replaces marker-wrapped words with gradient paint (or a
// static accent when the terminal cannot truecolor).
func (r *R) paintShimmer(text string) string {
	if !strings.ContainsRune(text, ShimmerStart) {
		return text
	}
	var b strings.Builder
	var word *strings.Builder
	for _, ru := range text {
		switch ru {
		case ShimmerStart:
			word = &strings.Builder{}
		case ShimmerEnd:
			if word != nil {
				if r.truecolor {
					b.WriteString(PitoShimmer.Colorize(word.String(), r.phase))
				} else {
					b.WriteString(lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(word.String()))
				}
				word = nil
			}
		default:
			if word != nil {
				word.WriteRune(ru)
			} else {
				b.WriteRune(ru)
			}
		}
	}
	if word != nil { // unterminated marker: degrade to plain
		b.WriteString(word.String())
	}
	return b.String()
}

// HasShimmer reports whether a payload carries anything that rides the
// shimmer phase — marked words, bar fills, or sparkline charts — the
// model uses it to decide which turns re-render on animation ticks.
func HasShimmer(payload []byte) bool {
	s := string(payload)
	return strings.Contains(s, "pito-subject-shimmer") ||
		strings.Contains(s, "pito-bar-shimmer") ||
		strings.Contains(s, "pito-metric--sparkline")
}

// Event renders one event to a newline-terminated block.
func (r *R) Event(ev api.Event) string {
	r.revealFrac = r.revealFor(ev.ID)
	switch ev.Kind {
	case api.KindEcho:
		return r.echo(ev)
	case api.KindSystem, api.KindSystemFollowUp:
		return r.messageBlock(r.systemBar, ev)
	case api.KindEnhanced, api.KindEnhancedFollowUp:
		return r.messageBlock(r.enhancedBar, ev)
	case api.KindError:
		return r.errorEvent(ev)
	case api.KindThinking:
		return r.thinking(ev)
	case api.KindConfirmation, api.KindConfirmationFollowUp:
		return r.confirmation(ev)
	default:
		return r.fallback(ev)
	}
}

// stamp is the dim HH:MM prefix the web shows inside each block.
func (r *R) stamp(ev api.Event) string {
	if ev.CreatedAt.IsZero() {
		return ""
	}
	return r.dim(ev.CreatedAt.Local().Format("15:04")) + " "
}

// dim styles inline fragments without the full-width dimStyle block.
func (r *R) dim(text string) string {
	return lipgloss.NewStyle().Foreground(ColorDim).Render(text)
}

// accent styles inline fragments in the user-accent color.
func (r *R) accent(text string) string {
	return lipgloss.NewStyle().Foreground(ColorAccent).Render(text)
}

// dimCopy renders product copy dim, with `backtick` command spans in the
// accent — the terminal cousin of the web's inline-code styling. An
// unbalanced backtick degrades to plain dim text.
func (r *R) dimCopy(text string) string {
	if strings.Count(text, "`")%2 != 0 {
		return r.dim(text)
	}
	var b strings.Builder
	parts := strings.Split(text, "`")
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i%2 == 1 {
			b.WriteString(r.accent(part))
		} else {
			b.WriteString(r.dim(part))
		}
	}
	return b.String()
}

// messageBlock renders a system/enhanced-shaped event as one bar block:
// timestamp + body, with the reply affordance inside the block like the web.
func (r *R) messageBlock(bar lipgloss.Style, ev api.Event) string {
	content := r.stamp(ev) + r.bodyText(ev)
	if hint := r.replyHintFor(ev); hint != "" {
		content += "\n" + hint
	}
	return bar.Render(content) + "\n"
}

// Notice renders a transient dim line (web-only verb replies, local hints).
func (r *R) Notice(text string) string {
	return r.dimStyle.Render("· "+text) + "\n"
}

type textPayload struct {
	Text string `json:"text"`
	Body string `json:"body"`
	HTML bool   `json:"html"`
	// Reply affordance (api.md): reply-capable events are stamped with a
	// reply_handle (#xyz); once a reply consumes it, drop the hint.
	ReplyHandle   string `json:"reply_handle"`
	ReplyConsumed bool   `json:"reply_consumed"`
	Channel       string `json:"channel"`
	// Structured list data (ls vids / ls games …): rows of cells with
	// CSS-class hints the web styles with; the TUI aligns and colors.
	TableRows []tableRow `json:"table_rows"`
	// TableHeading entries are strings OR {text, class} objects.
	TableHeading []tableCell `json:"table_heading"`
	// ListFooter is the dim usage hint under a list.
	ListFooter string `json:"list_footer"`
	// Sections are /help-style titled key/value groups.
	Sections []section `json:"sections"`
	// Games is channel_games' structured rows (tui-needs.md item 5):
	// text clients render from THIS, never the html cover grid.
	Games []gameRow `json:"games"`
}

type gameRow struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Vids  int    `json:"vids"`
}

type section struct {
	Title string `json:"title"`
	Rows  []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"rows"`
}

type tableRow struct {
	Cells []tableCell `json:"cells"`
}

type tableCell struct {
	Text  string `json:"text"`
	Class string `json:"class"`
	HTML  bool   `json:"html"`
}

// UnmarshalJSON lets a cell be a bare string (heading shorthand) or the
// full {text, class} object.
func (c *tableCell) UnmarshalJSON(raw []byte) error {
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		c.Text = plain
		return nil
	}
	type alias tableCell
	var full alias
	if err := json.Unmarshal(raw, &full); err != nil {
		return err
	}
	*c = tableCell(full)
	return nil
}

// replyHintFor renders the meta line (event/meta_line's cousin):
// "#handle" affordance in accent, "@channel" scope in cyan. Consumed
// handles drop the reply part.
func (r *R) replyHintFor(ev api.Event) string {
	var p textPayload
	if json.Unmarshal(ev.Payload, &p) != nil {
		return ""
	}
	parts := []string{}
	if p.ReplyHandle != "" && !p.ReplyConsumed {
		parts = append(parts, r.dim("reply with ")+r.accent(p.ReplyHandle)+r.dim(" …"))
	}
	if p.Channel != "" {
		cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Render("@" + strings.TrimPrefix(p.Channel, "@"))
		parts = append(parts, cyan)
	}
	return strings.Join(parts, r.dim(" · "))
}

// bodyText extracts renderable text from system/enhanced-shaped payloads:
// {text} for plain copy, {body, html:true} for prerendered HTML (tags
// stripped), and markdown-ish text through glamour when available.
func (r *R) bodyText(ev api.Event) string {
	var p textPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return strings.TrimSpace(string(ev.Payload))
	}
	// channel_games: synthesize the standard list table from the
	// structured rows and keep only the body's intro sentence — the
	// cover grid behind it is web-only.
	if len(p.Games) > 0 && len(p.TableRows) == 0 {
		p.TableHeading, p.TableRows = gamesTable(p.Games)
		if p.HTML {
			// First line only — the cover grid behind it is web-only.
			// Stays on the html path so shimmer markers paint.
			flat := htmlToText(p.Body)
			if i := strings.IndexByte(flat, '\n'); i >= 0 {
				flat = flat[:i]
			}
			p.Body = strings.TrimSpace(flat)
		}
	}
	headline := ""
	charts := r.analyzeBlock(ev.Payload)
	switch {
	case hasAnalyze(ev.Payload):
		// Analyze payloads: the html body is the WEB's drawing (ascii
		// hearts, pending dot-grids) — the terminal draws its own from
		// `analyze`, keeping only the intro. While metrics are still
		// pending (no series yet), show a quiet note instead of the art;
		// the event.replace stream fills the charts in as jobs land.
		headline = r.paintShimmer(htmlToText(analyzeIntro(ev.Payload)))
		if charts == "" {
			charts = r.dim("crunching the numbers…")
		}
	case p.Body != "" && p.HTML:
		// Show cards and game segments get the structured treatment:
		// zebra kv + 1:1 score/TTB bars (detail), scored rows (similar),
		// coverage bars + recommendations (channels). Anything else
		// flattens to text.
		switch {
		case hasPendingChannels(ev.Payload):
			// The distribution fills asynchronously — the web shows a
			// dotted canvas; the terminal waits quietly. The ready body
			// arrives by event.replace/resync.
			headline = r.paintShimmer(channelsIntro(ev.Payload)) + "\n\n" + r.dim("mapping the territory…")
		case hasPendingGlance(ev.Payload):
			// Same rhythm for the glance panel's AnalyticsFillJob.
			headline = r.paintShimmer(glanceIntroText(ev.Payload)) + "\n\n" + r.dim("crunching the numbers…")
		default:
			if card, ok := parseDetailCard(p.Body); ok {
				headline = r.detailCard(card)
			} else if sh, ok := parseShinies(p.Body); ok {
				headline = r.shiniesMessage(sh)
			} else if strip, ok := parseSimilarStrip(p.Body); ok {
				headline = r.similarStrip(strip)
			} else if gc, ok := parseGameChannels(p.Body); ok {
				headline = r.gameChannels(gc)
			} else if glance, ok := parseGlance(p.Body); ok {
				headline = r.glancePanel(glance)
			} else {
				headline = r.paintShimmer(htmlToText(p.Body))
			}
		}
	case p.Body != "":
		headline = r.markdown(p.Body)
	default:
		headline = r.markdown(p.Text)
	}
	parts := []string{}
	if headline != "" {
		parts = append(parts, headline)
	}
	if len(p.Sections) > 0 {
		parts = append(parts, r.sections(p.Sections))
	}
	if charts != "" {
		parts = append(parts, charts)
	}
	if len(p.TableRows) > 0 {
		if rendered := r.table(p.TableHeading, p.TableRows); rendered != "" {
			parts = append(parts, rendered)
		}
	}
	if p.ListFooter != "" {
		parts = append(parts, r.dimCopy(p.ListFooter))
	}
	return strings.Join(parts, "\n\n")
}

// sections renders /help-style titled key/value groups: purple section
// titles, accent keys, aligned values.
func (r *R) sections(groups []section) string {
	titleStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	var b strings.Builder
	for gi, group := range groups {
		if gi > 0 {
			b.WriteString("\n\n")
		}
		if group.Title != "" {
			b.WriteString(titleStyle.Render(group.Title) + "\n")
		}
		keyWidth := 0
		for _, row := range group.Rows {
			if w := lipgloss.Width(row.Key); w > keyWidth {
				keyWidth = w
			}
		}
		for ri, row := range group.Rows {
			if ri > 0 {
				b.WriteString("\n")
			}
			pad := keyWidth - lipgloss.Width(row.Key)
			b.WriteString(r.accent(row.Key) + strings.Repeat(" ", pad) + "  " + row.Value)
		}
	}
	return b.String()
}

// table renders structured rows through lipgloss/table — the shared list
// viewer for ls channels/vids/games (and every reply that re-emits a
// list). Rounded frame, header rule, zebra rows; alignment follows the
// server's own class hints (text-right = numbers/dates); columns whose
// body cells all render empty (images are ignored wholesale) drop.
func (r *R) table(heading []tableCell, rows []tableRow) string {
	cellText := func(cell tableCell) string {
		if cell.HTML {
			// Chips/coins stay plain inside lipgloss/table cells — the
			// table owns cell styling (zebra, truncation).
			return plainTokens(htmlToText(cell.Text))
		}
		return cell.Text
	}

	// Column census: text, emptiness, alignment, accent.
	columns := len(heading)
	for _, row := range rows {
		if len(row.Cells) > columns {
			columns = len(row.Cells)
		}
	}
	if columns == 0 {
		return ""
	}
	keep := make([]bool, columns)
	rightAlign := make([]bool, columns)
	for i, cell := range heading {
		if strings.Contains(cell.Class, "text-right") {
			rightAlign[i] = true
		}
	}
	texts := make([][]string, len(rows))
	accents := make([][]bool, len(rows))
	for ri, row := range rows {
		texts[ri] = make([]string, columns)
		accents[ri] = make([]bool, columns)
		for ci, cell := range row.Cells {
			if ci >= columns {
				break
			}
			text := strings.TrimSpace(cellText(cell))
			texts[ri][ci] = text
			accents[ri][ci] = strings.Contains(cell.Class, "action")
			if text != "" {
				keep[ci] = true
			}
			if strings.Contains(cell.Class, "text-right") {
				rightAlign[ci] = true
			}
		}
	}

	// A column survives if it has a heading OR any body content. Only
	// heading-less all-empty columns drop (the ignored-image residue) —
	// a headed column with empty cells is DATA (platform nobody set yet)
	// and hiding it broke `with platform` (owner report 2026-07-05).
	var cols []int
	for i := range keep {
		hasHeading := i < len(heading) && strings.TrimSpace(cellText(heading[i])) != ""
		if keep[i] || hasHeading {
			cols = append(cols, i)
		}
	}
	if len(cols) == 0 {
		return ""
	}
	pick := func(src []string) []string {
		out := make([]string, len(cols))
		for i, c := range cols {
			if c < len(src) {
				out[i] = src[c]
			}
		}
		return out
	}

	headerTexts := make([]string, columns)
	hasHeading := false
	for i, cell := range heading {
		headerTexts[i] = strings.TrimSpace(cellText(cell))
		if headerTexts[i] != "" {
			hasHeading = true
		}
	}

	build := func(width int) string {
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(ColorFaint)).
			BorderColumn(false).
			BorderRow(false).
			// Horizontal rules only (owner call): no vertical borders on
			// either side, the rows breathe against the message bar.
			BorderLeft(false).
			BorderRight(false).
			BorderHeader(hasHeading).
			// Never wrap: constrained cells truncate with … (the chosen
			// design). Wrapping spilled continuation lines outside the
			// frame once `with genre` pushed a table past the viewport.
			Wrap(false).
			StyleFunc(func(row, col int) lipgloss.Style {
				src := cols[col]
				st := lipgloss.NewStyle().Padding(0, 1)
				if rightAlign[src] {
					st = st.Align(lipgloss.Right)
				}
				if row == table.HeaderRow {
					return st.Foreground(ColorDim).Bold(true)
				}
				if row >= 0 && row < len(accents) && accents[row][src] {
					st = st.Foreground(ColorAccent)
				}
				if row%2 == 1 {
					st = st.Background(ColorZebra)
				}
				return st
			})
		if hasHeading {
			t = t.Headers(pick(headerTexts)...)
		}
		for _, row := range texts {
			t = t.Row(pick(row)...)
		}
		if width > 0 {
			t = t.Width(width)
		}
		return t.Render()
	}

	// The table lives inside a message bar: Width(r.width-2) including
	// 1 col of left padding, plus the border col outside that width —
	// content space is r.width-3. One char over and the bar wraps the
	// table's last column onto stub lines (zebra paints them visibly).
	avail := r.width - 3
	out := build(0)
	if lipgloss.Width(out) > avail {
		out = build(avail) // lipgloss/table truncates cells with … (Wrap(false))
	}
	// lipgloss/table quirk: heading-less tables drop their bottom rule
	// (even with BorderBottom(true)) — close the frame by repeating the
	// top rule. Box-drawing ─ never appears in cell text (em-dashes are
	// a different rune), so dash counts identify rule lines reliably.
	if !hasHeading {
		top := out[:strings.IndexByte(out, '\n')]
		last := out[strings.LastIndexByte(out, '\n')+1:]
		if strings.Count(last, "─") != strings.Count(top, "─") {
			out += "\n" + top
		}
	}
	return out
}

func (r *R) markdown(text string) string {
	if r.glam == nil {
		return strings.TrimSpace(text)
	}
	out, err := r.glam.Render(text)
	if err != nil {
		return strings.TrimSpace(text)
	}
	// Glamour pads with blank lines and trailing spaces; the transcript
	// owns spacing between blocks.
	return strings.TrimRight(strings.Trim(out, "\n"), " \n")
}

func (r *R) echo(ev api.Event) string {
	var p textPayload
	_ = json.Unmarshal(ev.Payload, &p)
	return r.echoBar.Render(r.stamp(ev)+p.Text) + "\n"
}

func (r *R) errorEvent(ev api.Event) string {
	var p struct {
		Text       string `json:"text"`
		Detail     string `json:"detail"`
		MessageKey string `json:"message_key"`
	}
	_ = json.Unmarshal(ev.Payload, &p)
	if p.Text == "" && p.MessageKey != "" {
		// I18n-only errors (server gap, on the Rails list): the key's
		// last segment is at least a humane hint — "usage" beats a JSON
		// dump. `verb --help` is always the way out.
		parts := strings.Split(p.MessageKey, ".")
		p.Text = strings.ReplaceAll(parts[len(parts)-1], "_", " ")
		p.Detail = p.MessageKey + " — try `--help` on the verb"
	}
	if p.Text == "" {
		return r.fallback(ev)
	}
	content := r.stamp(ev) + "✗ " + p.Text
	if p.Detail != "" {
		content += "\n" + r.dim(p.Detail)
	}
	return r.errorStyle.Render(content) + "\n"
}

func (r *R) thinking(ev api.Event) string {
	var p struct {
		Resolved       bool     `json:"resolved"`
		ElapsedSeconds *float64 `json:"elapsed_seconds"`
	}
	_ = json.Unmarshal(ev.Payload, &p)
	if p.Resolved && p.ElapsedSeconds != nil {
		return r.dimStyle.Render(fmt.Sprintf(">_< thought for %.1fs", *p.ElapsedSeconds)) + "\n"
	}
	if p.Resolved {
		return r.dimStyle.Render(">_< thought about it") + "\n"
	}
	return r.dimStyle.Render("thinking…") + "\n"
}

func (r *R) confirmation(ev api.Event) string {
	var p struct {
		Body         string            `json:"body"`
		HTML         bool              `json:"html"`
		ReplyHandle  string            `json:"reply_handle"`
		Resolved     bool              `json:"resolved"`
		OutcomeText  string            `json:"outcome_text"`
		ExpandDetail []json.RawMessage `json:"expand_detail"`
	}
	_ = json.Unmarshal(ev.Payload, &p)
	body := p.Body
	if p.HTML {
		body = htmlToText(body)
	}
	if p.Resolved && p.OutcomeText != "" {
		body = p.OutcomeText
	}
	content := r.stamp(ev) + lipgloss.NewStyle().Foreground(ColorWarn).Bold(true).Render("? ") + body
	// Stats detail beneath the body, shown only while pending — the web's
	// BodyComponent detail block (hairline, kv rows, "" = blank spacer).
	if !p.Resolved && len(p.ExpandDetail) > 0 {
		if detail := r.expandDetail(p.ExpandDetail); detail != "" {
			content += "\n" + r.hairline(lipgloss.Width(body)) + "\n" + detail
		}
	}
	if !p.Resolved && p.ReplyHandle != "" {
		content += "\n" + r.dim("reply with ") + r.accent(p.ReplyHandle) + r.dim(" …")
	}
	bar := r.confirmStyle
	if !p.Resolved && r.truecolor {
		// A live confirmation breathes: the warn border pulses with the
		// shimmer phase until a reply resolves it.
		bar = bar.BorderForeground(hex(pulseWarn(r.phase)))
	}
	return bar.Render(content) + "\n"
}

// expandDetail renders a confirmation card's stats block — the web
// BodyComponent's detail rows: {key,value} hashes as aligned kv rows
// (keys cyan, values right-aligned), plain strings as fg lines, "" as a
// blank spacer. key_class/value_class beyond the defaults are web CSS
// concerns and are not mapped.
func (r *R) expandDetail(raw []json.RawMessage) string {
	type kv struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	type row struct {
		kv   *kv
		text string
	}
	var rows []row
	keyWidth, valWidth := 0, 0
	for _, m := range raw {
		var s string
		if err := json.Unmarshal(m, &s); err == nil {
			rows = append(rows, row{text: s})
			continue
		}
		var pair kv
		if err := json.Unmarshal(m, &pair); err != nil || pair.Key == "" {
			continue
		}
		if w := lipgloss.Width(pair.Key); w > keyWidth {
			keyWidth = w
		}
		if w := lipgloss.Width(pair.Value); w > valWidth {
			valWidth = w
		}
		rows = append(rows, row{kv: &pair})
	}
	keyStyle := lipgloss.NewStyle().Foreground(ColorCyan)
	var out []string
	for _, rw := range rows {
		switch {
		case rw.kv != nil:
			keyPad := strings.Repeat(" ", keyWidth-lipgloss.Width(rw.kv.Key))
			valPad := strings.Repeat(" ", valWidth-lipgloss.Width(rw.kv.Value))
			out = append(out, keyStyle.Render(rw.kv.Key)+keyPad+"  "+valPad+rw.kv.Value)
		case rw.text != "":
			out = append(out, rw.text)
		default:
			out = append(out, "")
		}
	}
	return strings.Join(out, "\n")
}

// pulseWarn oscillates the warn accent between bright and embered — the
// live-confirmation heartbeat.
func pulseWarn(phase float64) RGB {
	bright := RGB{0xff, 0xd7, 0x5f}
	ember := RGB{0x9a, 0x7a, 0x3a}
	f := (math.Sin(phase*2*math.Pi) + 1) / 2
	lerp := func(a, b uint8) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*f) }
	return RGB{lerp(bright.R, ember.R), lerp(bright.G, ember.G), lerp(bright.B, ember.B)}
}

// fallback renders any unknown kind: the kind label plus its payload,
// pretty-printed. Old clients degrade, they never crash.
func (r *R) fallback(ev api.Event) string {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, ev.Payload, "", "  "); err != nil {
		pretty.Reset()
		pretty.Write(ev.Payload)
	}
	label := r.dimStyle.Render("[" + ev.Kind + "]")
	return label + "\n" + r.dimStyle.Render(pretty.String()) + "\n"
}

// SetReveal installs the grow-in fractions for freshly-arrived events
// (eventID → 0..1); events absent from the map render fully revealed.
func (r *R) SetReveal(fracs map[int64]float64) { r.reveal = fracs }

// revealFor is the current event's grow-in fraction.
func (r *R) revealFor(id int64) float64 {
	if r.reveal == nil {
		return 1
	}
	if f, ok := r.reveal[id]; ok {
		return f
	}
	return 1
}

// gamesTable shapes channel_games rows into the shared table language:
// #id and vids right-aligned like every list, title left.
func gamesTable(games []gameRow) ([]tableCell, []tableRow) {
	heading := []tableCell{
		{Text: "#", Class: "text-right"},
		{Text: "Game"},
		{Text: "Vids", Class: "text-right"},
	}
	rows := make([]tableRow, 0, len(games))
	for _, g := range games {
		rows = append(rows, tableRow{Cells: []tableCell{
			{Text: "#" + itoa(g.ID), Class: "pito-action-shimmer text-right"},
			{Text: g.Title},
			{Text: itoa(g.Vids), Class: "text-right"},
		}})
	}
	return heading, rows
}
