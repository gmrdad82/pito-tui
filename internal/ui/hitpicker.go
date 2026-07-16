// The conversation-search jump affordance (pito-tui 3.0.0 U2.3): when the
// NEWEST rendered event carries hits (a `search conversations for/like …`
// reply — pito's lib/pito/chat/handlers/search_conversations.rb, rendered
// via lib/pito/message_builder/conversation/hits.rb's table_heading/
// table_rows card), "J" at an empty prompt opens a small overlay listing
// the hits so the owner can jump straight to a hit's anchor instead of
// re-reading the rendered table and hand-typing a reply.
//
// ## Idiom choice
//
// This reuses entitypicker.go's own shape — a transient numbered overlay:
// mode switch, ↑/↓ (+ digit shortcuts) move/select, Enter jumps, Esc closes
// — rather than the other option on the table (bare alt+1..alt+9 over the
// scrollback with no overlay at all). The overlay wins because:
//
//   - It is the SAME shape as entityPicker/aiPicker/importPicker (every
//     existing "pick one of N structured rows" surface in this codebase),
//     so it costs nothing new to learn and nothing new to maintain — no
//     fourth navigation idiom.
//   - It needs NONE of entityPicker's paging/filtering machinery: a hits
//     list is small (search's own page_size caps it at 20) and arrives
//     fully loaded in the triggering event's payload — there is nothing to
//     fetch.
//   - It keeps "jump" off the typed-command grammar the reply-handle
//     hashtag idiom (`#a7 apply 2`) owns server-side: the hits table's own
//     numbered rows already exist for an owner who'd rather type a reply;
//     this overlay is a pure CLIENT shortcut over the same data, never a
//     new server round-trip.
//
// ## Same-vs-cross-conversation
//
// A hit carries `conversation_uuid` (api.ConversationHit — every route that
// opens a conversation, GET /chat/:uuid.json and the `/resume` slash
// command (config/pito/tools.yml `resume:`), is uuid-keyed, so this alone
// is enough to resolve ANY hit for real) plus `anchor_event_id`. The picker
// still prefers the cheap, local answer when it can: asking
// Transcript.EventLineRange(hit.AnchorEventID) — if the anchor event is
// already in the loaded transcript, the hit is (trivially) same-conversation
// and jumps in place via SetYOffset, no round trip needed. When it isn't
// loaded, the row renders dimmed up front ("(elsewhere)") so the owner
// knows before pressing anything, and picking it submits
// `/resume <conversation_uuid>` (or, for a lexical `for`/bare hit,
// `/resume <conversation_uuid> <anchor_event_id>` so resume lands on the
// matched occurrence, not just the conversation) through the real send
// path (#resumeHit) — the exact command a click on the web's
// conversation-name cell types+submits (see hits.rb's name_cell doc
// comment), rather than a dead end that only explains itself.
package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// hitPickerRow pairs a hit with whether its anchor is resolvable in the
// CURRENTLY loaded transcript, computed once at open time so the view can
// gray out unreachable rows before the owner ever presses a key.
type hitPickerRow struct {
	hit      api.ConversationHit
	jumpable bool
}

// hitPicker is the overlay's state.
type hitPicker struct {
	rows   []hitPickerRow
	cursor int
}

// hitsFromLatestEvent reports the newest rendered event's hits, ok=false
// when the transcript is empty, the newest event isn't kind "system", or
// its payload carries none — the "J" binding's gate (onChatKey's
// empty-prompt block).
func (m Model) hitsFromLatestEvent() ([]api.ConversationHit, bool) {
	ev, ok := m.transcript.LatestEvent()
	if !ok || ev.Kind != api.KindSystem {
		return nil, false
	}
	payload, err := api.DecodeSystemPayload(ev.Payload)
	if err != nil || len(payload.Hits) == 0 {
		return nil, false
	}
	return payload.Hits, true
}

// openHitPicker opens the overlay over hits (the "J" binding's target),
// precomputing each row's jumpability against the loaded transcript.
func (m Model) openHitPicker(hits []api.ConversationHit) (tea.Model, tea.Cmd) {
	rows := make([]hitPickerRow, len(hits))
	for i, hit := range hits {
		_, _, ok := m.transcript.EventLineRange(hit.AnchorEventID)
		rows[i] = hitPickerRow{hit: hit, jumpable: ok}
	}
	m.mode = modeHitPicker
	m.hitPick = hitPicker{rows: rows}
	return m, nil
}

// onHitPickerKey drives the overlay: ↑/↓ (and j/k, ctrl+p/n) move, a digit
// 1-9 jumps straight to that row (1-indexed, matching the rendered table's
// own visual order — rows past 9 stay reachable via the arrows, search's
// own page_size already caps the list at 20), Enter jumps the highlighted
// row, Esc closes without acting.
func (m Model) onHitPickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeChat
		m.hitPick = hitPicker{}
		return m, nil
	case "up", "k", "ctrl+p":
		if m.hitPick.cursor > 0 {
			m.hitPick.cursor--
		}
		return m, nil
	case "down", "j", "ctrl+n":
		if m.hitPick.cursor < len(m.hitPick.rows)-1 {
			m.hitPick.cursor++
		}
		return m, nil
	case "enter":
		if len(m.hitPick.rows) == 0 {
			return m, nil
		}
		return m.jumpToHit(m.hitPick.rows[m.hitPick.cursor].hit)
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		i := int(msg.String()[0] - '1')
		if i < len(m.hitPick.rows) {
			return m.jumpToHit(m.hitPick.rows[i].hit)
		}
		return m, nil
	}
	return m, nil
}

// jumpToHit closes the overlay. When the anchor event's turn is already in
// the loaded transcript it scrolls straight to its start (EventLineRange,
// transcript.go) and releases follow mode — a deliberate jump is not a
// "stay pinned to the bottom" moment, mirroring ctrl+home's own
// setFollow(m.sc.AtBottom()) rule. Re-resolves the anchor here rather than
// trusting the row's precomputed jumpable flag, so display and action
// share one source of truth even if the transcript changed while the
// overlay sat open. A hit whose anchor isn't loaded (cross-conversation —
// see this file's doc comment) hands off to #resumeHit instead of a dead
// end.
func (m Model) jumpToHit(hit api.ConversationHit) (tea.Model, tea.Cmd) {
	m.mode = modeChat
	m.hitPick = hitPicker{}
	start, _, ok := m.transcript.EventLineRange(hit.AnchorEventID)
	if !ok {
		return m.resumeHit(hit)
	}
	m.sc.SetYOffset(start)
	m.setFollow(m.sc.AtBottom())
	m.refreshViewport()
	return m, m.animate()
}

// resumeHit sends the same `/resume` command a click on the web's
// conversation-name cell types+submits (hits.rb's name_cell doc comment):
// `/resume <uuid>` for a semantic `like` hit, `/resume <uuid>
// <anchor_event_id>` for a lexical `for`/bare hit so resume lands on the
// matched occurrence rather than just the conversation. Goes through the
// real send path (sendCmd) — the same "pick a row, send the equivalent
// command" shape entitypicker.go's own enter-key handler uses — so
// whatever the server answers with renders exactly like any other typed
// command; this function does no conversation-switching of its own.
func (m Model) resumeHit(hit api.ConversationHit) (tea.Model, tea.Cmd) {
	text := "/resume " + hit.ConversationUUID
	if hit.OccurrenceCount != nil {
		text += " " + strconv.FormatInt(hit.AnchorEventID, 10)
	}
	m.recordHistory(text)
	m.sounds.Send()
	return m, m.sendCmd(m.conv.UUID, text, m.contentWidth()*8)
}

// ── View ────────────────────────────────────────────────────────────────

// hitPickerView renders the overlay: brand header with the right-edge Esc
// chip, the windowed hit rows (numbered, title + score/occurrence value,
// unreachable rows dimmed), the move/jump/close help line —
// pickerView/notificationsView's own shape.
func (m Model) hitPickerView() string {
	width := m.contentWidth()
	var b strings.Builder
	head := pickerBadgeStyle.Render("pito") + " " + render.Brand("conversation hits", m.truecolor)
	esc := render.Kbd("Esc", m.truecolor)
	if pad := width - lipgloss.Width(head) - lipgloss.Width(esc) - 1; pad > 0 {
		head += strings.Repeat(" ", pad) + esc
	}
	b.WriteString(head + "\n")
	rule := min(width-2, 44)
	if rule < 4 {
		rule = 4
	}
	b.WriteString(pickerDimStyle.Render(strings.Repeat("─", rule)) + "\n")

	var lines []string
	for i, row := range m.hitPick.rows {
		lines = append(lines, hitPickerRowLine(i, row, width, i == m.hitPick.cursor))
	}
	if len(lines) == 0 {
		lines = append(lines, pickerDimStyle.Render("  no hits"))
	}

	// Window like the notifications panel: title(2) + rule(1) + help(2).
	visible := m.height - 5
	if visible < 3 {
		visible = 3
	}
	cursorLine := min(m.hitPick.cursor, len(lines)-1)
	start := 0
	if len(lines) > visible {
		start = cursorLine - visible/2
		if start < 0 {
			start = 0
		}
		if start > len(lines)-visible {
			start = len(lines) - visible
		}
	}
	end := min(start+visible, len(lines))
	if start > 0 {
		b.WriteString(pickerDimStyle.Render(fmt.Sprintf("  ↑ %d more", start)) + "\n")
	}
	for _, line := range lines[start:end] {
		b.WriteString(line + "\n")
	}
	if end < len(lines) {
		b.WriteString(pickerDimStyle.Render(fmt.Sprintf("  ↓ %d more", len(lines)-end)) + "\n")
	}
	help := pickerKeyStyle.Render("j/k") + pickerDimStyle.Render(" move · ") +
		pickerKeyStyle.Render("1-9") + pickerDimStyle.Render(" / ") +
		pickerKeyStyle.Render("enter") + pickerDimStyle.Render(" jump · ") +
		pickerKeyStyle.Render("esc") + pickerDimStyle.Render(" close")
	b.WriteString("\n" + help)
	return strings.TrimRight(b.String(), "\n")
}

// hitPickerRowLine renders one row: a 1-indexed number (blank past 9, the
// digit shortcuts' own limit), the title (cursor accent when selected,
// dimmed + "(elsewhere)" when its anchor isn't in the loaded transcript),
// and the value cell trailing in dim text — no snippet column in the real
// contract (hits.rb dropped it in 3.0.0), so this shows what the second
// table cell actually held: a semantic hit's 0-100 score, or a lexical
// hit's occurrence count (#hitValueText). The selected row wears the house
// ▌ cursor stripe (picker.go's cursorStripe).
func hitPickerRowLine(i int, row hitPickerRow, width int, selected bool) string {
	num := ""
	if i < 9 {
		num = strconv.Itoa(i + 1)
	}
	zebra := zebraRowStyle(i)

	title := row.hit.Title
	if title == "" {
		title = "(untitled)"
	}
	titleText := zebra.Render(title)
	switch {
	case selected:
		titleText = pickerCursorStyle.Render(title)
	case !row.jumpable:
		titleText = pickerDimStyle.Render(title)
	}

	line := " " + zebra.Render(fmt.Sprintf("%-1s ", num)) + titleText
	if !row.jumpable {
		line += pickerDimStyle.Render(" (elsewhere)")
	}
	if value := hitValueText(row.hit); value != "" {
		if avail := width - 1 - lipgloss.Width(line) - 3; avail > 4 {
			line += pickerDimStyle.Render("  " + value)
		}
	}
	line = lipgloss.NewStyle().MaxWidth(width).Render(line)
	if selected {
		return cursorStripe(line, width)
	}
	return " " + line
}

// hitValueText renders a hit's second table cell back to plain text — a
// semantic `like` hit's 0-100 score (hits.rb's #like_cells {score:} cell)
// or a lexical `for`/bare hit's match count (#for_cells {text:} cell).
// The overlay has no room for the web's full score bar (ScoreBarComponent);
// this mirrors what the bare cell value reads as. Empty when neither field
// decoded (a malformed row, defensive only — DecodeSystemPayload already
// filters those out before this ever renders).
func hitValueText(hit api.ConversationHit) string {
	switch {
	case hit.Score != nil:
		return strconv.Itoa(*hit.Score)
	case hit.OccurrenceCount != nil:
		return strconv.Itoa(*hit.OccurrenceCount)
	default:
		return ""
	}
}
