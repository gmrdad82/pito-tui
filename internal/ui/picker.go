package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// pickerRow is one selectable line in the conversation picker: the "new
// conversation" entry or one resume row.
type pickerRow struct {
	uuid    string
	title   string
	last    time.Time
	isNew   bool
	ai      bool   // conversation carries an ai-kind event — sparkle badge
	section string // "recent" / "older" — group header painted above the first row of each
}

func pickerRows(list *api.ResumeList) []pickerRow {
	rows := []pickerRow{{isNew: true, title: "start a new conversation"}}
	return append(rows, resumeListRows(list)...)
}

// resumeListRows converts one /resume.json page's recent/older rows to
// pickerRows, WITHOUT the "new conversation" sentinel — pickerRows (the
// picker's first page) prepends that; onResume's pagination follow-on
// (tui-needs ask 9a) appends this straight onto the rows already on
// screen instead. Pages past the first fold their flat `rows` key into
// Older on the wire (ResumeList.UnmarshalJSON, events.go) and rarely
// carry Recent at all, but both slices are read identically here either
// way, so a page that DID carry genuine recent rows still slots in
// correctly, continuing (or starting) the RECENT/OLDER section run
// pickerView paints from.
func resumeListRows(list *api.ResumeList) []pickerRow {
	var rows []pickerRow
	for _, r := range list.Recent {
		rows = append(rows, pickerRow{uuid: r.UUID, title: r.Label(), last: r.LastActivityAt, ai: r.AI, section: "recent"})
	}
	for _, r := range list.Older {
		rows = append(rows, pickerRow{uuid: r.UUID, title: r.Label(), last: r.LastActivityAt, ai: r.AI, section: "older"})
	}
	return rows
}

var (
	pickerBadgeStyle   = lipgloss.NewStyle().Background(render.ColorPrimary).Foreground(lipgloss.Color("231")).Bold(true).Padding(0, 1)
	pickerSectionStyle = lipgloss.NewStyle().Foreground(render.ColorPrimary).Bold(true)
	pickerCursorStyle  = lipgloss.NewStyle().Foreground(render.ColorAccent).Bold(true)

	pickerNewStyle = lipgloss.NewStyle().Foreground(render.ColorOK)
	pickerDimStyle = lipgloss.NewStyle().Foreground(render.ColorDim)
	pickerKeyStyle = lipgloss.NewStyle().Foreground(render.ColorDim).Bold(true)

	// pickerDeleteConfirmStyle paints the dd-armed row's confirm prompt —
	// the same "needs a second press to confirm" language as the armed-quit
	// chip (onKey's "ctrl+c again to quit"), not the web's orange (pito
	// blue's warn accent is ColorWarn here, no direct copy of the web's
	// Pito::Copy palette).
	pickerDeleteConfirmStyle = lipgloss.NewStyle().Foreground(render.ColorWarn).Bold(true)
)

// pickerView renders the conversation picker, windowed so long lists keep
// the cursor on screen (title and help stay fixed). now anchors the
// relative timestamps so golden frames stay deterministic. phase is the
// model's global shimmer phase, threaded through so ai-flagged rows'
// sparkle badge rides the same sweep as the rest of the house animation.
// fetching appends the loadingDots row below the list — the picker's
// pagination follow-on (tui-needs ask 9a) fetching its next page,
// mirroring notificationsPanelView's loader row. A server that never
// sends next_cursor never sets this, so its frames render byte-identical
// to before pagination existed. renamingUUID/renameInputView swap the
// highlighted row for the live rename textinput.Model's own View() (the
// picker's `n` key, mirroring the web's pito--rename inline <input>
// swap); deleteArmed swaps it for the dd confirm prompt instead (the
// picker's `d`/`dd` key, mirroring resume_controller.js's #arm). Both are
// mutually exclusive with each other and only ever apply to the row under
// cursor — a renamingUUID that doesn't match the cursor row (stale after
// a fetch reshuffled rows) is simply never matched below.
func pickerView(rows []pickerRow, cursor, width, height int, now time.Time, truecolor bool, phase float64, fetching, canClose bool, renamingUUID, renameInputView string, deleteArmed bool) string {
	// Build every list line first, remembering which line the cursor is on
	// (section headers interleave, so row index ≠ line index).
	var lines []string
	cursorLine := 0
	section := ""
	for i, row := range rows {
		if row.section != "" && row.section != section {
			section = row.section
			lines = append(lines, pickerSectionStyle.Render(strings.ToUpper(section)))
		}
		selected := i == cursor

		// Inline rename replaces the row entirely with the input widget —
		// the web's own row.innerHTML swap (pito--rename #startRename).
		if selected && renamingUUID != "" && row.uuid == renamingUUID {
			marker := pickerCursorStyle.Render("▌ ")
			cursorLine = len(lines)
			lines = append(lines, lipgloss.NewStyle().MaxWidth(width).Render(marker+renameInputView))
			continue
		}

		// A dd-armed row replaces its content with the confirm prompt —
		// the web's own row.innerHTML swap (pito--resume #arm).
		armed := selected && deleteArmed && !row.isNew

		marker := "  "
		label := row.title
		switch {
		case armed:
			label = pickerDeleteConfirmStyle.Render("d again to delete")
		case row.isNew:
			label = pickerNewStyle.Render("+ " + label)
		case label == "":
			label = "(unnamed)"
		}
		if selected {
			marker = pickerCursorStyle.Render("▌ ")
			cursorLine = len(lines)
			if !row.isNew && !armed {
				label = pickerCursorStyle.Render(row.title)
			}
		}
		if row.ai && !armed {
			label += " " + aiBadge(phase, truecolor)
		}
		line := marker + label
		if !row.last.IsZero() && !armed {
			line += pickerDimStyle.Render("  · " + relativeTime(row.last, now))
		}
		line = lipgloss.NewStyle().MaxWidth(width).Render(line)
		if selected {
			// The selection wears a full-width elevated-gray highlight —
			// retinted off the table zebra's old plum (owner 2026-07-12
			// "align to Charm"): this is a SELECTION affordance, not
			// decoration, so it keeps a background, just a neutral one.
			if pad := width - 1 - lipgloss.Width(line); pad > 0 {
				line += strings.Repeat(" ", pad)
			}
			line = lipgloss.NewStyle().Background(render.ColorElevated).Render(line)
		}
		lines = append(lines, line)
	}
	if fetching {
		lines = append(lines, loadingDots(phase, truecolor, width))
	}

	// Window the list to the space between the fixed title (2 lines) and
	// help (2 lines), keeping the cursor visible.
	visible := height - 4
	if visible < 3 {
		visible = 3
	}
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
	end := start + visible
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	rule := width - 2
	if rule > 44 {
		rule = 44
	}
	if rule < 4 {
		rule = 4
	}
	head := pickerBadgeStyle.Render("pito") + " " + render.Brand("conversations", truecolor)
	if canClose {
		// Opened over a live conversation (/resume) — show the way back,
		// right-edge chip like every other modal (owner 2026-07-12).
		esc := render.Kbd("Esc", truecolor)
		if pad := width - lipgloss.Width(head) - lipgloss.Width(esc) - 1; pad > 0 {
			head += strings.Repeat(" ", pad) + esc
		}
	}
	b.WriteString(head + "\n")
	b.WriteString(pickerDimStyle.Render(strings.Repeat("─", rule)) + "\n")
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
		pickerKeyStyle.Render("enter") + pickerDimStyle.Render(" open · ") +
		pickerKeyStyle.Render("n") + pickerDimStyle.Render(" rename · ") +
		pickerKeyStyle.Render("dd") + pickerDimStyle.Render(" delete · ") +
		pickerKeyStyle.Render("ctrl-c") + pickerDimStyle.Render(" quit")
	b.WriteString("\n" + help)
	return b.String()
}

// relativeTime is the 1:1 port of pito's Pito::Formatter::CompactTimeAgo
// (lib/pito/formatter/compact_time_ago.rb) — same tiers, same "~" prefix,
// same rounding (always DOWN: a just-finished event reads "~0s ago", never
// "~1m ago"). The Ruby takes a nilable Time and returns "never" for nil;
// its Go counterpart takes the zero time.Time as that same "no timestamp"
// case. Ruby's `(Time.current - time).to_i` floors a non-negative float
// delta the same way integer-dividing whole seconds does here, so the
// straight Duration-in-seconds arithmetic below matches it exactly —
// there's no separate floor step to port.
func relativeTime(t, now time.Time) string {
	if t.IsZero() {
		return "never"
	}
	seconds := int64(now.Sub(t) / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	switch {
	case seconds < 60:
		return fmt.Sprintf("~%ds ago", seconds)
	case seconds < 3_600:
		return fmt.Sprintf("~%dm ago", seconds/60)
	case seconds < 86_400:
		return fmt.Sprintf("~%dh ago", seconds/3_600)
	case seconds < 2_592_000:
		return fmt.Sprintf("~%dd ago", seconds/86_400)
	case seconds < 31_536_000:
		return fmt.Sprintf("~%dmo ago", seconds/2_592_000)
	default:
		return fmt.Sprintf("~%dyr ago", seconds/31_536_000)
	}
}

// zebraRowStyle is the ls-vids table language for modal lists —
// alternating neutral-gray FOREGROUNDS for resting rows (owner
// 2026-07-13: "use what you used for ls vids. It's way cooler").
func zebraRowStyle(i int) lipgloss.Style {
	if i%2 == 0 {
		return lipgloss.NewStyle().Foreground(render.ColorDim)
	}
	return lipgloss.NewStyle().Foreground(render.ColorFaint)
}

// cursorStripe paints the SELECTED modal row: the ▌ accent bar plus the
// plum zebra stripe across the full row (owner 2026-07-13: "use the
// zebra effect from ls vids to show the cursor") — one cursor language
// for /resume, /notifications, ctrl+k, and the show game/vid pickers.
func cursorStripe(line string, width int) string {
	if pad := width - 1 - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return pickerCursorStyle.Render("▌") + lipgloss.NewStyle().Background(render.ColorZebra).Render(line)
}
