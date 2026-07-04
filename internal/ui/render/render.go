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
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// R renders events at a fixed width. Rebuild it on terminal resize — the
// glamour renderer word-wraps at construction time.
type R struct {
	width int
	plain bool
	style string
	glam  *glamour.TermRenderer

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
		width: width,
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

// Event renders one event to a newline-terminated block.
func (r *R) Event(ev api.Event) string {
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
	headline := ""
	switch {
	case p.Body != "" && p.HTML:
		headline = htmlToText(p.Body)
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
	if len(p.TableRows) > 0 {
		parts = append(parts, r.table(p.TableHeading, p.TableRows))
	}
	if p.ListFooter != "" {
		parts = append(parts, r.dim(p.ListFooter))
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

// table renders structured rows as aligned columns: dim headings,
// right-aligned where the web says so (text-right), actionable references
// (#28 …) in the accent color — the closest thing to the web's clickable
// shimmer. HTML cells (avatars) flatten to their text content.
func (r *R) table(heading []tableCell, rows []tableRow) string {
	all := rows
	if len(heading) > 0 {
		all = append([]tableRow{{Cells: heading}}, rows...)
	}
	columns := 0
	for _, row := range all {
		if len(row.Cells) > columns {
			columns = len(row.Cells)
		}
	}
	cellText := func(cell tableCell) string {
		if cell.HTML {
			return htmlToText(cell.Text)
		}
		return cell.Text
	}
	widths := make([]int, columns)
	for _, row := range all {
		for i, cell := range row.Cells {
			if w := lipgloss.Width(cellText(cell)); w > widths[i] {
				widths[i] = w
			}
		}
	}
	var b strings.Builder
	for ri, row := range all {
		if ri > 0 {
			b.WriteString("\n")
		}
		isHeading := len(heading) > 0 && ri == 0
		for i, cell := range row.Cells {
			if i > 0 {
				b.WriteString("  ")
			}
			text := cellText(cell)
			pad := widths[i] - lipgloss.Width(text)
			switch {
			case isHeading:
				text = r.dim(text)
			case strings.Contains(cell.Class, "action"):
				text = r.accent(text)
			}
			if strings.Contains(cell.Class, "text-right") {
				b.WriteString(strings.Repeat(" ", pad) + text)
			} else if i < len(row.Cells)-1 {
				b.WriteString(text + strings.Repeat(" ", pad))
			} else {
				b.WriteString(text) // last column: no trailing padding
			}
		}
	}
	return b.String()
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
		Text   string `json:"text"`
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(ev.Payload, &p)
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
		Body        string `json:"body"`
		HTML        bool   `json:"html"`
		ReplyHandle string `json:"reply_handle"`
		Resolved    bool   `json:"resolved"`
		OutcomeText string `json:"outcome_text"`
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
	if !p.Resolved && p.ReplyHandle != "" {
		content += "\n" + r.dim("reply with ") + r.accent(p.ReplyHandle) + r.dim(" …")
	}
	return r.confirmStyle.Render(content) + "\n"
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
