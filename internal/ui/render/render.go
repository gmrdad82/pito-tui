// Package render turns canonical events into terminal blocks. One renderer
// per known kind, *_follow_up variants reuse their base, and everything
// else — including kinds this client has never heard of — falls back to a
// generic payload dump. Novelty must never crash: renderers decode the raw
// payload themselves and degrade on any mismatch.
package render

import (
	"bytes"
	"encoding/json"
	"image/color"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/glamour"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// ContentCap is the owner-locked containment law: EVERYTHING — message
// blocks, cards, tables, chart canvases, banners, the palette, overlay
// bodies, the context meter, the prompt line, and the status bar —
// renders LEFT-ANCHORED inside min(terminalWidth−2, ContentCap)
// columns as ONE coherent column (owner 2.0.0 smoke, 2026-07-12: a
// capped conversation over full-width chrome read as two different
// apps; nothing is exempt anymore). The margin beyond the column
// belongs to the ambient star-field. The caller (internal/ui
// Model.contentWidth) computes the margined-and-capped width and feeds
// it into New — every renderer in this package inherits the cap for
// free by never reading past r.width.
const ContentCap = 100

// R renders events at a fixed width. Rebuild it on terminal resize — the
// glamour renderer word-wraps at construction time.
type R struct {
	width     int
	plain     bool
	truecolor bool
	phase     float64
	conductor float64 // 0 = every painter keeps its own stagger; 1 = all collapse onto r.phase (see SetConductor)
	// shake holds the ambassador wave's per-event error-shake offsets
	// (micro.go effect 1) — eventID → cells, absent ⇒ render at rest. See
	// SetShake/shakeFor and Event's own applyShake call.
	shake map[int64]int
	// glint is the ambassador wave's confirm-glint sweep progress
	// (micro.go effect 2): -1 (New's own default) means no sweep is live
	// right now, 0..1 otherwise. See SetGlint and confirmChrome.
	glint float64
	// ticks mirrors model.go's raw aliveTicks (real, gate-open animation
	// ticks only) — see SetTicks. r.phase alone can't drive a cadence
	// LONGER than its own ~2.667s wrap (scaling an already-wrapped value
	// can only shorten its period, never lengthen it), so effects that
	// need a slower beat — the shiny badge's iridescent twinkle,
	// tokens.go's sparkleActive — key off this instead, the same
	// "aliveTicks % cycleTicks" shape ui/micro.go's confirmGlintProgress
	// already uses for the confirm dialog's own glint.
	ticks int64
	now   func() time.Time
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

// WithTruecolor enables gradient/shimmer painting (COLORTERM-detected by
// the app before Bubble Tea starts; 256-color terminals get static
// accents instead).
func WithTruecolor(on bool) Option {
	return func(r *R) { r.truecolor = on }
}

// WithNow injects the clock stamp() ages timestamps against (tests pin
// it; the app leaves the default).
func WithNow(now func() time.Time) Option {
	return func(r *R) { r.now = now }
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
	bar := func(c color.Color) lipgloss.Style {
		return lipgloss.NewStyle().
			Border(lipgloss.ThickBorder(), false, false, false, true).
			BorderForeground(c).
			PaddingLeft(1).Width(width - 1)
	}
	r := &R{
		width: width,
		now:   time.Now,
		glint: -1, // no confirm-glint sweep in progress until SetGlint says otherwise
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

// SetConductor blends every painter's per-element stagger toward zero:
// weight 0 (the default, and the shimmer conductor's resting state)
// changes nothing — each element keeps sampling r.phase at its own
// stable offset, same as before the conductor existed. Weight 1
// collapses every offset to zero, so every shimmer painter on screen
// samples the exact same r.phase and the whole surface reads as one
// traveling wave instead of scattered neighbors. The model ramps this
// 0→1→0 over its ~2s sweep window (ambient.go's conductorWeight) —
// SetPhase alone can't produce that synchronized moment because the
// per-element offset is additive inside staggered/staggered20 (bars.go),
// not a shift of the shared phase itself.
func (r *R) SetConductor(weight float64) { r.conductor = weight }

// SetTicks forwards the model's raw aliveTicks counter every animation
// tick — see the R.ticks field doc for why this exists alongside SetPhase
// rather than being derived from it.
func (r *R) SetTicks(ticks int64) { r.ticks = ticks }

// paintShimmer replaces marker-wrapped words with paint (color spec v2,
// owner 2026-07-12): subject words shimmer pink→derived-band, reference
// tokens shimmer cyan→derived-band; off-truecolor both settle on their
// static base (pink 175 / cyan).
func (r *R) paintShimmer(text string) string {
	if !strings.ContainsRune(text, ShimmerStart) && !strings.ContainsRune(text, TokenStart) &&
		!strings.ContainsRune(text, HeaderStart) {
		return text
	}
	var b strings.Builder
	var word *strings.Builder
	var token *strings.Builder
	var header *strings.Builder
	for _, ru := range text {
		if header != nil {
			if ru == HeaderEnd {
				b.WriteString(lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(header.String()))
				header = nil
			} else {
				header.WriteRune(ru)
			}
			continue
		}
		switch ru {
		case HeaderStart:
			header = &strings.Builder{}
		case ShimmerStart:
			word = &strings.Builder{}
		case ShimmerEnd:
			if word != nil {
				if r.truecolor {
					b.WriteString(SubjectShimmer.Colorize(word.String(), r.phase))
				} else {
					b.WriteString(lipgloss.NewStyle().Foreground(ColorSubject).Bold(true).Render(word.String()))
				}
				word = nil
			}
		case TokenStart:
			token = &strings.Builder{}
		case TokenEnd:
			if token != nil {
				if r.truecolor {
					b.WriteString(ReferenceShimmer.Colorize(token.String(), r.phase))
				} else {
					b.WriteString(lipgloss.NewStyle().Foreground(ColorCyan).Render(token.String()))
				}
				token = nil
			}
		default:
			switch {
			case token != nil:
				token.WriteRune(ru)
			case word != nil:
				word.WriteRune(ru)
			default:
				b.WriteRune(ru)
			}
		}
	}
	if word != nil { // unterminated marker: degrade to plain
		b.WriteString(word.String())
	}
	if token != nil {
		b.WriteString(token.String())
	}
	if header != nil {
		b.WriteString(header.String())
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
	var block string
	switch ev.Kind {
	case api.KindEcho:
		block = r.echo(ev)
	case api.KindSystem, api.KindSystemFollowUp:
		block = r.messageBlock(r.systemBar, ev)
	case api.KindEnhanced, api.KindEnhancedFollowUp:
		block = r.messageBlock(r.enhancedBar, ev)
	case api.KindError:
		block = r.errorEvent(ev)
	case api.KindThinking:
		block = r.thinking(ev)
	case api.KindConfirmation, api.KindConfirmationFollowUp:
		block = r.confirmation(ev)
	case api.KindAi:
		block = r.aiEvent(ev)
	default:
		block = r.fallback(ev)
	}
	// The ambassador wave's error-shake (micro.go effect 1) hooks in
	// last, at this same per-event render seam — applyShake is a no-op
	// (byte-identical return) for every event with no entry in r.shake,
	// which is every event, always, unless something is actively
	// shaking right now. See applyShake's own doc comment for why this
	// never shifts any OTHER event's block.
	return applyShake(block, r.shakeFor(ev.ID))
}

// stamp is the dim timestamp prefix the web shows inside each block.
// Today's messages show bare "HH:MM"; older ones carry their day so a
// days-old conversation never reads like it happened today. Format is
// the owner's (2026-07-11): same year "6 Jul 11:04", other years
// "2 Jan '25 11:04" — day without leading zero, abbreviated month,
// apostrophe two-digit year.
func (r *R) stamp(ev api.Event) string {
	if ev.CreatedAt.IsZero() {
		return ""
	}
	local := ev.CreatedAt.Local()
	now := r.now()
	layout := "15:04"
	switch {
	case sameYear(local.Year(), now) && local.YearDay() == now.YearDay():
		// today — time only
	case sameYear(local.Year(), now):
		layout = "2 Jan 15:04"
	default:
		layout = "2 Jan '06 15:04"
	}
	return r.dim(local.Format(layout)) + " "
}

// sameYear is the day-aware rule stamp() keys its year-elision off — true
// once year no longer needs spelling out because it matches now's. Shared
// with tokens.go's shinyDateSuffix (the shiny badge's unlock date), which
// mirrors this exact rule at month granularity (the web sends no day for
// badge dates, only "Mon 'YY").
func sameYear(year int, now time.Time) bool {
	return year == now.Year()
}

// dim styles inline fragments without the full-width dimStyle block.
func (r *R) dim(text string) string {
	return lipgloss.NewStyle().Foreground(ColorDim).Render(text)
}

// accent styles inline fragments in the user-accent color.
func (r *R) accent(text string) string {
	return lipgloss.NewStyle().Foreground(ColorAccent).Render(text)
}

// replyAffordance renders the reply-affordance meta line — the web's
// `#handle shift+r` shape (owner screenshots: literally "#iota-5965
// shift+r"): the handle in the accent it already uses, then "shift+r" as
// a QUIET kbd chip — dim foreground on a barely-elevated background, no
// gradient/shimmer (tokens.go's ShinyBadge/platformChip build louder
// chips; this one is a keyboard hint, not a badge). Non-truecolor drops
// the background and stays dim-on-default. The one shared builder so the
// three call sites (render.go's replyHintFor/confirmation, ai.go's
// aiDoneEvent) can't drift apart again.
func (r *R) replyAffordance(handle string) string {
	// The chip's own leading padding space doubles as the handle/chip
	// separator — matching the exact web shape "#handle shift+r" with a
	// single space, not two. Bare (no bed): the plum background read as
	// harsh sitting on the @ai tile's lavender (owner 2026-07-13).
	return r.accent(handle) + KbdBare("shift+r", r.truecolor)
}

// shimmerRunes writes keys to b one rune at a time, each rune styled by
// styleAt(t) where t walks the same 0.15..0.5 mid-band sample of
// PitoShimmer — bright enough to gleam on a plum bed or bare ground, never
// wrapping back to the dim base stop. The single gradient-math source Kbd,
// KbdBare, and KbdPlain all sample from, so the "frozen gleam, not an
// animated one" look can't drift between the three chip shapes.
func shimmerRunes(b *strings.Builder, keys string, styleAt func(t float64) lipgloss.Style) {
	runes := []rune(keys)
	for i, ru := range runes {
		t := 0.15 + 0.35*float64(i)/float64(max(len(runes)-1, 1))
		b.WriteString(styleAt(t).Render(string(ru)))
	}
}

// Kbd renders one keyboard chip — the shared shape for every keybinding
// hint (reply affordance, the chatbox cycler hints, the modal Esc
// chips, the status row's quit hint). Exported for the ui package's
// chrome. Truecolor chips got the charm treatment (owner 2026-07-12:
// "Esc key doesn't look charmy and glossy… use fancyness more on it"):
// an elevated plum bed with the glyphs riding a static sample of the
// brand ramp — each rune a step further along PitoShimmer, a frozen
// gleam rather than an animated one (a keycap, not a badge). Non-
// truecolor stays the quiet dim chip.
// KbdBare is Kbd without the bed — the same brand-ramp glyphs on
// transparent ground, for chips that sit on tinted surfaces (the reply
// affordance on the @ai tile; a background-on-background read as harsh).
func KbdBare(keys string, truecolor bool) string {
	if !truecolor {
		return lipgloss.NewStyle().Foreground(ColorDim).Render(" " + keys + " ")
	}
	var b strings.Builder
	b.WriteString(" ")
	shimmerRunes(&b, keys, func(t float64) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(hex(PitoShimmer.At(t)))
	})
	b.WriteString(" ")
	return b.String()
}

func Kbd(keys string, truecolor bool) string {
	if !truecolor {
		return lipgloss.NewStyle().Foreground(ColorDim).Render(" " + keys + " ")
	}
	bed := lipgloss.NewStyle().Background(ColorZebra)
	var b strings.Builder
	b.WriteString(bed.Render(" "))
	shimmerRunes(&b, keys, func(t float64) lipgloss.Style {
		return bed.Foreground(hex(PitoShimmer.At(t)))
	})
	b.WriteString(bed.Render(" "))
	return b.String()
}

// KbdPlain is KbdBare with the outer padding stripped: no leading/trailing
// literal space, no padded bed cell on either side — just the bare
// brand-ramp glyphs. Kbd and KbdBare both self-pad one space per side
// (" "+keys+" " in the non-truecolor branch; padded bed cells in the
// truecolor ones), which is the right shape for a chip dropped mid-
// sentence but doubles up wherever the caller ALSO supplies its own
// separator space — the status bar's doubled spacing this fixes
// (pito-tui 3.0.0 task U1.1, owner 2026-07-15). Kbd/KbdBare keep their
// padded shape as-is (goldens pin them); callers that own their own
// spacing want KbdPlain instead.
func KbdPlain(keys string, truecolor bool) string {
	if !truecolor {
		return lipgloss.NewStyle().Foreground(ColorDim).Render(keys)
	}
	var b strings.Builder
	shimmerRunes(&b, keys, func(t float64) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(hex(PitoShimmer.At(t)))
	})
	return b.String()
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
	// AI marks @ai turns' echoes (2.0.0) — they wear the AI accent.
	AI bool `json:"ai"`
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
	// kv-hash shape (jobs status, config status tables): rows carry
	// key/value + web class hints instead of cells. table() routes
	// them to the kv renderer.
	Key        string `json:"key"`
	Value      string `json:"value"`
	KeyClass   string `json:"key_class"`
	ValueClass string `json:"value_class"`
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
		parts = append(parts, r.replyAffordance(p.ReplyHandle))
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
		// pending (no series yet), hold the space with the web's own
		// pending dot-grid canvas (COPY LAW: never a client-made note);
		// the event.replace stream fills the charts in as jobs land.
		headline = r.paintShimmer(htmlToText(analyzeIntro(ev.Payload)))
		if charts == "" {
			charts = r.pendingCanvas()
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
			headline = r.paintShimmer(channelsIntro(ev.Payload)) + "\n\n" + r.pendingCanvas()
		case hasPendingGlance(ev.Payload):
			// Same rhythm for the glance panel's AnalyticsFillJob.
			headline = r.paintShimmer(glanceIntroText(ev.Payload)) + "\n\n" + r.pendingCanvas()
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
				headline = r.paintShimmer(RenderMessageHTML(p.Body))
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

// tablePurple resolves the header/rule purple lipgloss/table StyleFuncs
// share: ColorPrimary ("99") is literally the color Charm's own canonical
// table example (the Pokémon table) uses for both header text and border
// rules, on any terminal; truecolor gets the exact web-Charm hex
// (CharmPurple) instead of that 256-color approximation. See CharmPurple's
// doc comment (theme.go) for the owner order this ports.
func tablePurple(truecolor bool) color.Color {
	if truecolor {
		return CharmPurple
	}
	return ColorPrimary
}

// table renders structured rows through lipgloss/table — the shared list
// viewer for ls channels/vids/games (and every reply that re-emits a
// list). Rounded frame, header rule, alternating gray-foreground rows —
// Charm's own canonical lipgloss/table look (owner 2026-07-12 "align to
// Charm"), no background zebra; alignment follows the server's own class
// hints (text-right = numbers/dates); columns whose body cells all render
// empty (images are ignored wholesale) drop. classStyle maps the web's
// text-* class hints onto theme colors — the kv-hash rows (jobs/config
// status) carry explicit classes.
func classStyle(class string) lipgloss.Style {
	st := lipgloss.NewStyle()
	switch {
	case strings.Contains(class, "text-purple"):
		// Man-page section headers (pito-help-block's "Usage:"/"Options:"
		// spans) ride this class — same purple as sections()'s titles.
		return st.Foreground(ColorPrimary)
	case strings.Contains(class, "text-red"):
		return st.Foreground(ColorErr)
	case strings.Contains(class, "text-green"):
		return st.Foreground(ColorOK)
	case strings.Contains(class, "text-yellow"):
		return st.Foreground(ColorWarn)
	case strings.Contains(class, "text-cyan"):
		return st.Foreground(ColorCyan)
	case strings.Contains(class, "text-orange"):
		return st.Foreground(ColorOrange)
	case strings.Contains(class, "text-pito"):
		return st.Foreground(ColorPito)
	case strings.Contains(class, "text-fg-dim"):
		// Checked ahead of any bare "text-fg" match (none exists today,
		// but strings.Contains would otherwise let a future one mis-match
		// this more specific class first) — same specific-before-general
		// rule text-fg-faded below follows.
		return st.Foreground(ColorDim)
	case strings.Contains(class, "text-fg-faded"):
		return st.Foreground(ColorFaint)
	default:
		return st
	}
}

// kvHashTable renders key/value table_rows (jobs status, config status):
// keys padded to a shared column, both sides honoring their web class
// hints. The shape has no cells — see tableRow.
func (r *R) kvHashTable(rows []tableRow) string {
	keyWidth := 0
	for _, row := range rows {
		if w := lipgloss.Width(row.Key); w > keyWidth {
			keyWidth = w
		}
	}
	var b strings.Builder
	for i, row := range rows {
		if i > 0 {
			b.WriteString("\n")
		}
		pad := strings.Repeat(" ", keyWidth-lipgloss.Width(row.Key))
		b.WriteString("  " + classStyle(row.KeyClass).Render(row.Key) + pad + "  " +
			classStyle(row.ValueClass).Render(row.Value))
	}
	return b.String()
}

func (r *R) table(heading []tableCell, rows []tableRow) string {
	// kv-hash rows (no cells anywhere, keys present) take the kv path.
	if len(rows) > 0 {
		hashRows := true
		for _, row := range rows {
			if len(row.Cells) > 0 || row.Key == "" {
				hashRows = false
				break
			}
		}
		if hashRows {
			return r.kvHashTable(rows)
		}
	}
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
			BorderStyle(lipgloss.NewStyle().Foreground(tablePurple(r.truecolor))).
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
					return st.Foreground(tablePurple(r.truecolor)).Bold(true)
				}
				if row >= 0 && row < len(accents) && accents[row][src] {
					// Class hints that carry MEANING (the pink
					// pito-action-shimmer ids) keep their own color — the
					// decorative gray alternation below never overrides it.
					return st.Foreground(ColorAccent)
				}
				// Charm's own canonical look: no background zebra at all —
				// alternating gray FOREGROUNDS instead, the same two grays
				// (ColorDim/ColorFaint) Charm's Pokémon table example ships
				// as gray/lightGray.
				if row%2 == 1 {
					return st.Foreground(ColorDim)
				}
				return st.Foreground(ColorFaint)
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
	if p.AI {
		// @ai turns wear the AI accent — a vertical purple→pito-blue
		// gradient bar (the web's data-accent="ai"). W2 adds the shimmer.
		return r.aiAccentBar(r.stamp(ev)+p.Text) + "\n"
	}
	return r.echoBar.Render(r.stamp(ev)+p.Text) + "\n"
}

// aiAccentBar wraps content like the other message bars but paints the
// left bar rune-by-rune down the AI gradient. Off-truecolor it settles
// on the brand purple.
func (r *R) aiAccentBar(content string) string {
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
			style = style.Foreground(hex(AIAccent.At(t)))
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(style.Render("┃") + line)
	}
	return b.String()
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
		p.Detail = p.MessageKey + " — try `--help` on the tool"
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
		// Confirmation bodies carry pito-token spans (e.g. the channel
		// handle in a disconnect card) — paint them, don't leak markers.
		body = r.paintShimmer(htmlToText(body))
	}
	if p.Resolved && p.OutcomeText != "" {
		body = p.OutcomeText
		if strings.Contains(body, "<") {
			// The web sometimes renders outcome_text with inline pito-token
			// spans (e.g. the channel handle in a disconnect card) — same
			// leak the pending body just above guards against. A plain-text
			// outcome ("Called off.") carries no "<" and skips this
			// entirely, so it renders byte-for-byte unchanged.
			body = r.paintShimmer(RenderMessageHTML(body))
		}
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
		content += "\n" + r.replyAffordance(p.ReplyHandle)
	}
	if p.Resolved || !r.truecolor {
		return r.confirmStyle.Render(content) + "\n"
	}
	// A live, truecolor confirmation breathes: the warn border pulses with
	// the shimmer phase until a reply resolves it, plus the ambassador
	// wave's own confirm-glint (micro.go effect 2) — a single brighter
	// cell sliding down the border on top of that pulse. Both need a
	// PER-LINE border color, which lipgloss's Border()+BorderForeground()
	// can't give (one color for every line); hand-painted bar cells, like
	// ai.go's aiChrome, are the seam that can.
	return r.confirmChrome(content) + "\n"
}

// confirmChrome hand-paints a pending confirmation's left border one line
// at a time — ai.go's aiChrome shape, applied here so the confirm-glint
// (micro.go effect 2) can recolor ONE row independently of every other
// row's shared pulseWarn breathing. Reproduces confirmStyle's own
// PaddingLeft(1)+Width(width-1) content box byte-for-byte; only the
// border character's per-line color differs from the plain
// BorderForeground path confirmation() takes when the glint is off
// (r.glint < 0 for every row here), so a settled/inactive sweep renders
// identically either way.
func (r *R) confirmChrome(content string) string {
	body := lipgloss.NewStyle().PaddingLeft(1).Width(r.width - 1).Render(content)
	lines := strings.Split(body, "\n")
	base := pulseWarn(r.phase)
	glintRow := -1
	if r.glint >= 0 {
		glintRow = confirmGlintRow(r.glint, len(lines))
	}
	var b strings.Builder
	for i, line := range lines {
		c := base
		if i == glintRow {
			c = brighten(base, 0.4)
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(lipgloss.NewStyle().Foreground(hex(c)).Render("┃") + line)
	}
	return b.String()
}

// confirmGlintRow resolves the glint's 0..1 sweep progress against THIS
// block's own row count: round(progress*(rows-1)), so row 0 (top) lights
// at progress 0 and the last row lights at progress 1 regardless of how
// tall any particular confirmation card is. rows <= 0 has nothing to
// light (returns -1, never equal to any real line index).
func confirmGlintRow(progress float64, rows int) int {
	if rows <= 0 {
		return -1
	}
	if rows == 1 {
		return 0
	}
	row := int(progress*float64(rows-1) + 0.5)
	if row < 0 {
		row = 0
	}
	if row > rows-1 {
		row = rows - 1
	}
	return row
}

// brighten lifts c toward white by amount ∈ [0,1] — the glint's own
// "+40% brightness" on top of whatever the pulse's base color is at this
// instant, per channel, clamped at 255.
func brighten(c RGB, amount float64) RGB {
	lift := func(x uint8) uint8 {
		v := float64(x) + (255-float64(x))*amount
		if v > 255 {
			v = 255
		}
		return uint8(v)
	}
	return RGB{R: lift(c.R), G: lift(c.G), B: lift(c.B)}
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

// SetShake installs the ambassador wave's error-shake offsets (micro.go
// effect 1; eventID → cells). Events absent from the map render at their
// resting position (no pad) — "absence ⇒ settled".
func (r *R) SetShake(offsets map[int64]int) { r.shake = offsets }

// shakeFor is the current event's jitter offset — 0 (no entry in the
// map, or a nil map entirely) means render at rest.
func (r *R) shakeFor(id int64) int {
	if r.shake == nil {
		return 0
	}
	return r.shake[id]
}

// SetGlint installs the ambassador wave's confirm-glint sweep progress
// (micro.go effect 2): -1 means no sweep is live right now (New's own
// default, and confirmGlintProgress's off/outside-window result), 0..1
// otherwise. confirmChrome resolves a non-negative progress against its
// own row count.
func (r *R) SetGlint(progress float64) { r.glint = progress }

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
