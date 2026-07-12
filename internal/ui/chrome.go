package ui

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	bkey "charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// The ambassador wave's chrome pass: four terminal-native flourishes that
// lean on things a terminal emulator itself can do — OSC title/hyperlink
// escapes, a splash screen, a typed keymap — rather than more shimmer.
// Same house law as ambient.go/ripple.go/micro.go: each is independently
// kill-switchable below.
//
//   - splashEnabled: a startup wordmark, pure chrome over the loading
//     beat — see splash.go (its own file: a hand-crafted braille pixel
//     font plus the rise-away physics earned a home of their own).
//   - keymapFooterEnabled: '?' in chat mode toggles a compact, GROUPED
//     keymap strip above the input line — replacing helpLine's plain
//     single-tier list with one built FROM a charm.land/bubbles/v2/key
//     keymap struct (footerKeymap below) so the strip's text and the
//     bindings it documents can't quietly drift apart. Off-truecolor (or
//     the flag off) keeps the exact old helpLine() behavior byte-for-byte
//     — see keymapFooterView's own early return. Slides open/closed via
//     footerAnim, a plain spring.go overlayAnim value this file drives
//     directly (step/settled/target are already generic over any "active"
//     bool; no changes to spring.go itself were needed) — see
//     Model.stepFooterAnim/footerAnimating, wired from onAnimTick/
//     animGateOpen, and Model.onChatKey's own '?' case.
//   - oscTitleEnabled: the terminal tab title tracks "pito · <conversation
//     label>", plus " · ✉ N" once unread > 0 — see windowTitle, read by
//     Model.View() into tea.View.WindowTitle (a plain field in Bubble Tea
//     v2, not a Cmd). Recomputed every View() call, but bubbletea's own
//     renderer only ever EMITS the OSC sequence when the string actually
//     changed frame to frame (cursed_renderer.go's own View diff) — so
//     "update on conversation switch/unread change only" is a guarantee
//     the renderer already gives for free; nothing here needs to track
//     staleness itself. Truecolor-independent (window titles are a
//     terminal feature, not a color one) — gated on the flag alone.
//   - osc8LinksEnabled: a share-reply's URL (the web's `#handle share` —
//     Pito::Share::UniversalActions#handle_share, always a `kind: :system`
//     event whose html body is a witty sentence with the plain URL text
//     inside an <a>) renders as a REAL clickable terminal hyperlink — see
//     wrapEventLinks, the Model.onResize SetRenderer seam. Truecolor-
//     independent, same as the title. Never touches render.go: this
//     post-processes renderer.Event(ev)'s own STRING output rather than
//     reaching inside it, gated on the event's kind (messageBlock's own
//     domain — api.KindSystem/KindSystemFollowUp, the only two kinds this
//     wave touches) and a payload-shape peek (osc8SafeFromStructure) that
//     keeps it out of anything carrying table_rows/sections/an embedded
//     analyze chart — "never inside charts/tables," enforced by
//     construction rather than by pattern-matching the rendered bytes.
const (
	splashEnabled       = true
	keymapFooterEnabled = true
	oscTitleEnabled     = true
	osc8LinksEnabled    = true
)

// ── Effect 2: keymap footer ──────────────────────────────────────────────

// footerChip is the quiet kbd chip's own local reimplementation: dim
// foreground on a barely-elevated background, no gradient/shimmer — the
// exact shape render.go's replyAffordance already uses for its "shift+r"
// hint, but that builder is unexported (private to the render package), so
// per the brief this file grows its own rather than reaching into render's
// internals.
func footerChip(label string) string {
	return lipgloss.NewStyle().Foreground(render.ColorDim).Background(render.ColorZebra).Render(" " + label + " ")
}

// footerKeymap is the '?' footer's own typed keymap — bubbles/v2/key
// bindings, the Charm-native way to describe a keymap, even though
// dispatch itself stays onChatKey/onPickerKey/onNotificationsKey's
// existing plain switch (this struct documents those bindings, it never
// drives them). Every field's Help().Key is the DISPLAY label the footer
// renders (matching the house's existing "shift+r" convention for the
// real dispatch key "R" — replyAffordance's own shown text), and Keys()
// carries the real msg.String() values that binding answers to.
type footerKeymap struct {
	scrollArrows bkey.Binding
	scrollCtrl   bkey.Binding
	scrollPage   bkey.Binding
	reply        bkey.Binding
	scopeChannel bkey.Binding
	scopePeriod  bkey.Binding
	palette      bkey.Binding
	panelOpen    bkey.Binding
	panelClose   bkey.Binding
	quit         bkey.Binding
}

// newFooterKeymap builds the footer's fixed keymap — called once into the
// package-level footerKeymapDefault below; every binding here is a static
// fact about the app's own key handling, never per-instance state.
func newFooterKeymap() footerKeymap {
	return footerKeymap{
		scrollArrows: bkey.NewBinding(bkey.WithKeys("shift+up", "shift+down"), bkey.WithHelp("shift+↑/↓", "Scroll")),
		scrollCtrl:   bkey.NewBinding(bkey.WithKeys("ctrl+u", "ctrl+d"), bkey.WithHelp("ctrl+u/d", "Scroll")),
		scrollPage:   bkey.NewBinding(bkey.WithKeys("pgup", "pgdown"), bkey.WithHelp("pgup/pgdn", "Scroll")),
		reply:        bkey.NewBinding(bkey.WithKeys("R"), bkey.WithHelp("shift+r", "Reply")),
		scopeChannel: bkey.NewBinding(bkey.WithKeys("shift+tab"), bkey.WithHelp("shift+tab", "Scope")),
		scopePeriod:  bkey.NewBinding(bkey.WithKeys("ctrl+space"), bkey.WithHelp("ctrl+space", "Scope")),
		palette:      bkey.NewBinding(bkey.WithKeys("ctrl+k"), bkey.WithHelp("ctrl+k", "Panels")),
		panelOpen:    bkey.NewBinding(bkey.WithKeys("/notifications"), bkey.WithHelp("/notifications", "Panels")),
		panelClose:   bkey.NewBinding(bkey.WithKeys("esc"), bkey.WithHelp("esc", "Panels")),
		quit:         bkey.NewBinding(bkey.WithKeys("ctrl+c"), bkey.WithHelp("ctrl+c", "Quit")),
	}
}

// footerKeymapDefault is the one instance the footer ever renders from —
// bindings are static, so there is exactly one of these, built once at
// package init like any other package-level table in this codebase.
var footerKeymapDefault = newFooterKeymap()

// footerGroup pairs one of the brief's five labels with the bindings that
// belong under it, in display order.
type footerGroup struct {
	label    string
	bindings []bkey.Binding
}

// groups is footerKeymap's own grouping table — the SINGLE place that
// decides which bindings sit under which label, read by both bindings()
// (the flat "every binding" list a test walks) and the footer's own
// segments() renderer, so the two can never disagree about what's in the
// keymap.
func (k footerKeymap) groups() []footerGroup {
	return []footerGroup{
		{"Scroll", []bkey.Binding{k.scrollArrows, k.scrollCtrl, k.scrollPage}},
		{"Reply", []bkey.Binding{k.reply}},
		{"Scope", []bkey.Binding{k.scopeChannel, k.scopePeriod}},
		{"Panels", []bkey.Binding{k.palette, k.panelOpen, k.panelClose}},
		{"Quit", []bkey.Binding{k.quit}},
	}
}

// bindings flattens groups() into every binding this keymap documents, in
// display order — TestKeymapFooterListsEveryBinding walks this to assert
// the rendered footer and the struct can never silently diverge.
func (k footerKeymap) bindings() []bkey.Binding {
	var out []bkey.Binding
	for _, g := range k.groups() {
		out = append(out, g.bindings...)
	}
	return out
}

// segments renders each group as one "Label chip chip chip" string — the
// footer's own reveal (keymapFooterView) shows a growing PREFIX of this
// slice as its spring opens, one group popping in at a time.
func (k footerKeymap) segments() []string {
	groups := k.groups()
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		chips := make([]string, 0, len(g.bindings))
		for _, b := range g.bindings {
			chips = append(chips, footerChip(b.Help().Key))
		}
		out = append(out, statusStyle.Render(g.label)+" "+strings.Join(chips, " "))
	}
	return out
}

// footerAnimating reports whether the footer's own open/close spring still
// needs ticks — folded into Model.animGateOpen alongside every other
// windowed effect.
func (m Model) footerAnimating() bool {
	return keymapFooterEnabled && m.truecolor && !m.footerAnim.settled(m.showHelp)
}

// stepFooterAnim advances the footer's spring one tick toward m.showHelp's
// own target (1 open, 0 closed) — called from onAnimTick every tick,
// exactly like spring.go's stepOverlays steps pickerAnim/notifAnim. A no-op
// off-truecolor or with the flag off: the plain instant helpLine() swap
// (keymapFooterView's own early branch) needs no spring at all.
func (m *Model) stepFooterAnim() {
	if !keymapFooterEnabled || !m.truecolor {
		return
	}
	m.footerAnim = m.footerAnim.step(m.showHelp)
}

// keymapFooterView is viewContent's (and chatViewportHeight's) one seam
// for the '?' strip: off-truecolor or the flag off keeps returning exactly
// what helpLine() always did — "" when showHelp is false, the single
// plain-text line otherwise — so non-truecolor terminals and every golden
// test (none of which sets WithTruecolor, and none of which ever presses
// '?') see byte-identical output to before this file existed. Truecolor
// with the flag on renders the grouped panel instead, its own group
// segments revealing one at a time as footerAnim's spring travels from 0
// to 1 (opening) or back (closing) — "" once the spring is fully closed AND
// showHelp is false, so a mid-close frame still shows its own fading tail.
func (m Model) keymapFooterView() string {
	if !keymapFooterEnabled || !m.truecolor {
		if m.showHelp {
			return m.helpLine()
		}
		return ""
	}
	segs := footerKeymapDefault.segments()
	pos := m.footerAnim.pos
	if pos <= 0 {
		return ""
	}
	if pos > 1 {
		pos = 1
	}
	n := int(pos*float64(len(segs)) + 0.5)
	if n <= 0 {
		return ""
	}
	if n > len(segs) {
		n = len(segs)
	}
	// Wrap the groups into lines that respect the content column — a
	// footer running past the terminal edge gets its last groups cut
	// (owner-visible in the W7.e capture), so pack greedily instead.
	sep := statusStyle.Render(" · ")
	limit := m.contentWidth()
	var lines []string
	line := ""
	for _, seg := range segs[:n] {
		switch {
		case line == "":
			line = seg
		case lipgloss.Width(line)+lipgloss.Width(sep)+lipgloss.Width(seg) <= limit:
			line += sep + seg
		default:
			lines = append(lines, line)
			line = seg
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// ── Effect 3: OSC window title ───────────────────────────────────────────

// windowTitleText is the title string's own pure builder: "pito · <label>"
// plus " · ✉ N" once unread is positive — split out from windowTitle so
// the label/unread forms are directly testable without a whole Model.
func windowTitleText(label string, unread int) string {
	title := "pito · " + label
	if unread > 0 {
		title += " · ✉ " + strconv.Itoa(unread)
	}
	return title
}

// windowTitle is Model.View()'s own seam into tea.View.WindowTitle. label
// mirrors statusLine's own fallback chain (Conversation.Label, then "new
// conversation"/"(unnamed)") so the tab title and the in-app status line
// never disagree about what this conversation is called. Reads m.unread
// (the settled target), never displayUnread's live-rolling odometer value
// — a window title should track the real count, not visibly count through
// every intermediate integer while ripple.go's odometer animation plays.
func (m Model) windowTitle() string {
	name := m.conv.Label()
	switch {
	case name != "":
	case m.conv.UUID == "":
		name = "new conversation"
	default:
		name = "(unnamed)"
	}
	return windowTitleText(name, m.unread)
}

// ── Effect 4: OSC 8 hyperlinks ───────────────────────────────────────────

// shareURLPattern finds a bare https:// run inside an already-rendered
// block — literal bytes, any surrounding ANSI escapes included: bodyText
// applies at most one style span per paragraph (never per word), so a URL
// sitting in prose is never interrupted mid-scheme by an SGR code, and
// scanning the FINAL rendered string is exactly as reliable as scanning
// the source text would have been. Stops at whitespace or the next ANSI
// escape/BEL — the run's own upper bound.
var shareURLPattern = regexp.MustCompile(`https://[^\s\x1b\x07]+`)

// urlTrailingPunct is punctuation a URL is never actually part of in the
// server's own prose (the witty share sentences end IN a sentence, not a
// link) — trimmed off the greedy match before wrapping so
// "…/share/ab12c3. Enjoy!" doesn't pull the sentence's own full stop
// inside the hyperlink.
const urlTrailingPunct = ".,;:!?)]}'\""

// wrapPlainURLs OSC-8-wraps every bare https:// run in text.
// lipgloss.Style.Hyperlink (charm.land/lipgloss/v2's own v2-native
// builder — no hand-emitted escapes needed) renders
// ansi.SetHyperlink(url)+text+ansi.ResetHyperlink() around the matched
// text; charm.land/x/ansi's width/wrap/truncate already treat that pair as
// zero-width (verified against the module's own OSC-8 wrap/truncate test
// fixtures), so this is safe to run AFTER messageBlock has already sized
// its bar's fixed padding from the plain, unwrapped text — the wrap adds
// only invisible bytes around characters that were already there.
func wrapPlainURLs(text string) string {
	if !strings.Contains(text, "https://") {
		return text
	}
	return shareURLPattern.ReplaceAllStringFunc(text, func(match string) string {
		url := strings.TrimRight(match, urlTrailingPunct)
		trailing := match[len(url):]
		return lipgloss.NewStyle().Hyperlink(url).Render(url) + trailing
	})
}

// osc8EligibleKind reports whether ev's kind is messageBlock's own domain
// (render.go's Event switch: KindSystem/KindSystemFollowUp reach
// messageBlock via the systemBar branch — KindEnhanced/KindEnhancedFollowUp
// reach the SAME function but also carry detail cards/similar strips/game
// channels/glance panels, richer structured renders this wave stays out
// of). Share-reply confirmations (Pito::Share::UniversalActions#handle_share,
// the pito source of every URL this effect will ever see) are always
// kind: :system, so restricting to these two kinds already covers the
// entire real-world target without touching a single chart/table kind.
func osc8EligibleKind(kind string) bool {
	switch kind {
	case api.KindSystem, api.KindSystemFollowUp:
		return true
	default:
		return false
	}
}

// osc8PayloadShape peeks at the RAW payload — never the rendered block —
// for the structured shapes messageBlock also renders through its OWN
// textPayload: table_rows, sections, an embedded analyze chart. This is
// the second half of "never inside charts/tables": an eligible-kind event
// that ALSO carries any of these skips the wrap entirely, even though its
// own headline prose might still contain a literal URL.
type osc8PayloadShape struct {
	Analyze   json.RawMessage   `json:"analyze"`
	TableRows []json.RawMessage `json:"table_rows"`
	Sections  []json.RawMessage `json:"sections"`
	Games     []json.RawMessage `json:"games"`
}

// osc8SafeFromStructure reports whether payload is free of every
// structured shape osc8PayloadShape watches for. A decode failure degrades
// to "safe" — bodyText itself falls back to raw text on the same failure
// (render.go's own `if err := json.Unmarshal(...); err != nil` branch), so
// there is no table/chart to protect against in that case either.
func osc8SafeFromStructure(payload []byte) bool {
	var shape osc8PayloadShape
	if json.Unmarshal(payload, &shape) != nil {
		return true
	}
	return len(shape.Analyze) == 0 && len(shape.TableRows) == 0 &&
		len(shape.Sections) == 0 && len(shape.Games) == 0
}

// wrapEventLinks is the actual Model.onResize SetRenderer seam: a thin
// post-process over renderer.Event(ev)'s own return value, gated on the
// event's kind/payload shape rather than on the rendered bytes themselves.
// Truecolor-independent (osc8LinksEnabled's own doc comment) — the ONE
// condition checked is the flag.
func wrapEventLinks(ev api.Event, block string) string {
	if !osc8LinksEnabled || !osc8EligibleKind(ev.Kind) || !osc8SafeFromStructure(ev.Payload) {
		return block
	}
	return wrapPlainURLs(block)
}
