package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// NotificationsFetchedMsg carries one page of GET /notifications.json (or
// its failure).
type NotificationsFetchedMsg struct {
	Page *api.NotificationPage
	Err  error
}

// notificationsPanel is the /notifications overlay's state: the loaded
// rows (newest-first, exactly as served — no client sort), the cursor,
// and the pagination cursor for the next fetch.
type notificationsPanel struct {
	rows     []api.NotificationRow
	cursor   int
	next     string // cursor for the NEXT fetch; "" + fetched == exhausted
	fetched  bool   // at least one page has landed successfully
	fetching bool   // a fetch is in flight — guards duplicate requests
	err      string // last fetch's error text ("unavailable" is handled separately, panel closes instead)
}

// needsFetch reports whether the panel should kick off a fetch: nothing
// already in flight, the list isn't exhausted, and the cursor rests on
// (or past) the last loaded row — the infinite-scroll trigger.
//
// This single predicate does triple duty, deliberately: a zero-value
// panel satisfies it (cursor 0 >= len(rows)-1 == -1), so it doubles as
// the very-first-open trigger; a page that just landed with the cursor
// still resting at the bottom satisfies it too, so short pages keep
// pulling until the viewport fills or the list is exhausted; and an
// errored fetch leaves rows/cursor/next untouched, so it stays true and
// the next scroll nudge simply retries the same request — no separate
// retry path to maintain.
func (p notificationsPanel) needsFetch() bool {
	if p.fetching {
		return false
	}
	if p.fetched && p.next == "" {
		return false
	}
	return p.cursor >= len(p.rows)-1
}

// notificationsFetchCmd GETs one page starting at before ("" for page 1).
func (m Model) notificationsFetchCmd(before string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		page, err := client.FetchNotifications(context.Background(), before, 50)
		return NotificationsFetchedMsg{Page: page, Err: err}
	}
}

// openNotifications intercepts `/notifications` client-side (see
// onChatKey) and opens the overlay with a fresh panel — maybeFetchMore
// on the zero-value panel is what fires the first page. notifAnim resets
// to a standing start too, so the slide-up plays from the bottom edge on
// every open, not just the first.
func (m Model) openNotifications() (tea.Model, tea.Cmd) {
	m.mode = modeNotifications
	m.notif = notificationsPanel{}
	return m.maybeFetchMore()
}

// maybeFetchMore fires notificationsFetchCmd when needsFetch says to,
// and keeps the shared animation tick alive so the loader's phase moves.
func (m Model) maybeFetchMore() (tea.Model, tea.Cmd) {
	if !m.notif.needsFetch() {
		return m, nil
	}
	m.notif.fetching = true
	return m, tea.Batch(m.notificationsFetchCmd(m.notif.next), m.animate())
}

func (m Model) onNotificationsFetched(msg NotificationsFetchedMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeNotifications {
		return m, nil // panel closed before this landed — drop it
	}
	m.notif.fetching = false
	if msg.Err != nil {
		if errors.Is(msg.Err, api.ErrNotificationsUnavailable) {
			m.mode = modeChat
			m.notif = notificationsPanel{}
			m.pushNotice("notifications need a newer pito — update the server")
			m.refreshViewport()
			return m, nil
		}
		m.notif.err = msg.Err.Error()
		return m, nil
	}
	m.notif.err = ""
	m.notif.rows = append(m.notif.rows, msg.Page.Rows...)
	m.notif.next = msg.Page.NextCursor
	m.notif.fetched = true
	return m.maybeFetchMore()
}

// markNotificationReadOnArrival implements the web's arrow rule: an
// UNREAD row the cursor lands on flips read — optimistic local restyle,
// unread badge decrement, PATCH in the background. Read rows are left
// alone (movement never un-reads). Pointer-free: the caller re-batches
// the returned cmd; the local mutation happens here on the value model
// the caller keeps.
func (m *Model) markNotificationReadOnArrival(i int) tea.Cmd {
	if i < 0 || i >= len(m.notif.rows) || m.notif.rows[i].Read {
		return nil
	}
	m.notif.rows[i].Read = true
	if m.unread > 0 {
		m.beginUnreadRoll(m.unread - 1)
	}
	client, id := m.client, m.notif.rows[i].ID
	return func() tea.Msg {
		_ = client.PatchNotification(context.Background(), id, true)
		return nil
	}
}

// toggleNotificationRead is the web's click: flip either way.
func (m *Model) toggleNotificationRead(i int) tea.Cmd {
	if i < 0 || i >= len(m.notif.rows) {
		return nil
	}
	target := !m.notif.rows[i].Read
	m.notif.rows[i].Read = target
	if target && m.unread > 0 {
		m.beginUnreadRoll(m.unread - 1)
	} else if !target {
		m.beginUnreadRoll(m.unread + 1)
	}
	client, id := m.client, m.notif.rows[i].ID
	return func() tea.Msg {
		_ = client.PatchNotification(context.Background(), id, target)
		return nil
	}
}

// notifPageStep is the pgup/pgdn jump size — the picker's window formula
// (see notificationsPanelView), reused so a page-key press moves roughly
// one screenful.
func notifPageStep(height int) int {
	step := height - 4
	if step < 3 {
		step = 3
	}
	return step
}

func (m Model) onNotificationsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		// Slide back down first — the mode only actually switches once
		// the closing spring settles (spring.go's stepOverlays).
		m.mode = modeChat
		m.notif = notificationsPanel{}
		return m, nil
	case "enter":
		// The web's click: toggle the row's read state (the ONLY way a
		// row flips back to unread) — optimistic restyle + PATCH.
		if m.notif.cursor < len(m.notif.rows) {
			return m, m.toggleNotificationRead(m.notif.cursor)
		}
		return m, nil
	case "j", "down":
		if m.notif.cursor < len(m.notif.rows)-1 {
			m.notif.cursor++
		}
		next, cmd := m.maybeFetchMore()
		mm := next.(Model)
		// The web's arrow contract (notifications_nav): ARRIVING on an
		// unread row marks it read; read rows are never flipped by
		// movement.
		return mm, tea.Batch(cmd, mm.markNotificationReadOnArrival(mm.notif.cursor))
	case "k", "up":
		if m.notif.cursor > 0 {
			m.notif.cursor--
		}
		next, cmd := m.maybeFetchMore()
		mm := next.(Model)
		return mm, tea.Batch(cmd, mm.markNotificationReadOnArrival(mm.notif.cursor))
	case "pgdown":
		m.notif.cursor = min(m.notif.cursor+notifPageStep(m.height), max(len(m.notif.rows)-1, 0))
		return m.maybeFetchMore()
	case "pgup":
		m.notif.cursor -= notifPageStep(m.height)
		if m.notif.cursor < 0 {
			m.notif.cursor = 0
		}
		return m.maybeFetchMore()
	}
	return m, nil
}

// ── View ────────────────────────────────────────────────────────────────

var (
	notifUnreadStyle = lipgloss.NewStyle().Foreground(render.ColorCyan)
	notifReadStyle   = lipgloss.NewStyle().Foreground(render.ColorFaint)
	notifErrStyle    = lipgloss.NewStyle().Foreground(render.ColorErr)
)

func (m Model) notificationsView() string {
	// The notifications panel is an overlay body — content, capped like
	// every other surface (height/window math stays on the raw terminal).
	return notificationsPanelView(m.notif, m.contentWidth(), m.height, m.now(), m.truecolor, m.phase)
}

// notificationsPanelView renders the overlay, windowed exactly like
// pickerView so a long list keeps the cursor on screen. now anchors the
// row stamps so golden/unit frames stay deterministic; phase/truecolor
// thread the shared shimmer through to loadingDots.
func notificationsPanelView(p notificationsPanel, width, height int, now time.Time, truecolor bool, phase float64) string {
	var lines []string
	switch {
	case p.fetching && len(p.rows) == 0:
		lines = append(lines, loadingDots(phase, truecolor, width))
	case len(p.rows) == 0 && p.err != "":
		lines = append(lines, notifErrStyle.Render("  "+p.err))
	case len(p.rows) == 0:
		lines = append(lines, pickerDimStyle.Render("  no notifications"))
	default:
		for i, row := range p.rows {
			lines = append(lines, notificationRowLine(row, width, now, i == p.cursor))
		}
		switch {
		case p.fetching:
			lines = append(lines, loadingDots(phase, truecolor, width))
		case p.err != "":
			lines = append(lines, notifErrStyle.Render("  "+p.err))
		}
	}

	// Window the list to the space between the fixed title (2 lines) and
	// help (2 lines), keeping the cursor visible — pickerView's formula.
	visible := height - 4
	if visible < 3 {
		visible = 3
	}
	cursorLine := min(p.cursor, len(lines)-1)
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

	var b strings.Builder
	rule := width - 2
	if rule > 44 {
		rule = 44
	}
	if rule < 4 {
		rule = 4
	}
	// Title row with the Esc chip at the modal's RIGHT edge (owner
	// 2026-07-12 — same rule as the ctrl+k palette).
	head := pickerBadgeStyle.Render("pito") + " " + render.Brand("notifications", truecolor)
	esc := render.Kbd("Esc", truecolor)
	if pad := width - lipgloss.Width(head) - lipgloss.Width(esc) - 1; pad > 0 {
		head += strings.Repeat(" ", pad) + esc
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
	help := pickerKeyStyle.Render("j/k") + pickerDimStyle.Render(" move")
	b.WriteString("\n" + help)
	return b.String()
}

// notificationRowLine renders one row: unread/read glyph, the message
// truncated to fit, and a right-aligned dim day-aware stamp. The selected
// row wears the picker's full-width elevated-gray highlight, glyph left in
// place (the panel is read-only — there is no separate cursor marker to
// swap in, the unread/read state must stay visible either way).
func notificationRowLine(row api.NotificationRow, width int, now time.Time, selected bool) string {
	marker := notifReadStyle.Render("○ ")
	if !row.Read {
		marker = notifUnreadStyle.Render("● ")
	}
	stamp := pickerDimStyle.Render(notificationStamp(row.CreatedAt, now))
	stampW := lipgloss.Width(stamp)

	avail := width - lipgloss.Width(marker) - stampW - 1
	if avail < 1 {
		avail = 1
	}
	// Server messages may carry simple inline HTML (<strong> around the
	// entity name — W7.f audit finding); flatten to plain text before
	// truncating so tags never render literally.
	body := marker + truncateEllipsis(render.FlattenHTML(row.Message), avail)

	pad := width - lipgloss.Width(body) - stampW
	if pad < 1 {
		pad = 1
	}
	line := body + strings.Repeat(" ", pad) + stamp
	line = lipgloss.NewStyle().MaxWidth(width).Render(line)

	if selected {
		if pad := width - 1 - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		line = lipgloss.NewStyle().Background(render.ColorElevated).Render(line)
	}
	return line
}

// notificationStamp mirrors render.(*R).stamp's day-aware layout rules
// exactly (today "15:04", same year "2 Jan 15:04", else "2 Jan '06
// 15:04") so a notification's timestamp reads identically to a
// transcript event's, whichever panel it's in.
func notificationStamp(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	local := t.Local()
	switch {
	case local.Year() == now.Year() && local.YearDay() == now.YearDay():
		return local.Format("15:04")
	case local.Year() == now.Year():
		return local.Format("2 Jan 15:04")
	default:
		return local.Format("2 Jan '06 15:04")
	}
}

// truncateEllipsis trims s to width runes, trailing with "…" when it had
// to cut (matching the tokens.go shiny-face convention).
func truncateEllipsis(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
