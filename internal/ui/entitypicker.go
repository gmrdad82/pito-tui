// The show game / show vid picker — the TUI face of pito's
// PickerControllers (GET /games/picker.json, /videos/picker.json; the
// endpoints' own doc comments name this client). Owner 2026-07-12:
// "the picker for show game and show vid that I have in web pito with
// search, 50 paginated if more than 50 available".
//
// A bare `show game` / `show vid` (the forms that open the web's picker
// sidebar — the server answers non-browser clients with its browser-only
// notice) intercepts client-side and opens this overlay instead: rows
// page in 50 at a time through the house after=/next_cursor cursor,
// scrolling near the end fetches more, typing filters (the JSON feed has
// no q= yet, so the filter is client-side and EAGERLY pages the rest of
// the list in while active — search must cover everything, not just the
// pages that happened to load), ↑↓ move, Enter selects: it SENDS
// `show game <id>` through the real send path, exactly like the web's
// games_nav "show" mode fills the chatbox and submits. Esc slides the
// panel away and returns to the conversation.
package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// entityPicker is the overlay's state. noun is the API path segment
// ("games"/"videos"); command is the tool noun the selection sends
// ("game"/"vid").
type entityPicker struct {
	noun     string
	command  string
	rows     []api.PickerRow
	cursor   int
	next     string // cursor for the NEXT fetch; "" + fetched == exhausted
	fetched  bool
	fetching bool
	query    string
	err      string
}

// EntityPickerFetchedMsg carries one page of the picker feed.
type EntityPickerFetchedMsg struct {
	Noun string
	Page *api.PickerPage
	Err  error
}

// entityPickerTrigger recognizes the bare picker-opening forms of a
// typed command: "show game" / "show vid" (and their plural/synonym
// spellings, mirroring the grammar's noun aliases). Returns the API noun
// and command noun, or ok=false.
func entityPickerTrigger(text string) (noun, command string, ok bool) {
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(tokens) != 2 || tokens[0] != "show" {
		return "", "", false
	}
	switch tokens[1] {
	case "game", "games", "gamez":
		return "games", "game", true
	case "vid", "vids", "video", "videos":
		return "videos", "vid", true
	}
	return "", "", false
}

// openEntityPicker opens the overlay and fires page 1.
func (m Model) openEntityPicker(noun, command string) (tea.Model, tea.Cmd) {
	m.mode = modeEntityPicker
	m.entity = entityPicker{noun: noun, command: command}
	return m.maybeFetchMoreEntities()
}

// needsFetch mirrors notificationsPanel.needsFetch: zero-value opens the
// first page, short pages keep pulling, an error retries on the next
// nudge. While a filter query is active the list pages EAGERLY until
// exhausted — the search must see every row.
func (p entityPicker) needsFetch() bool {
	if p.fetching {
		return false
	}
	if p.fetched && p.next == "" {
		return false
	}
	if p.query != "" {
		return true // eager: filtering wants the whole list in
	}
	return p.cursor >= len(p.visibleRows())-1
}

func (m Model) maybeFetchMoreEntities() (tea.Model, tea.Cmd) {
	if !m.entity.needsFetch() {
		return m, nil
	}
	m.entity.fetching = true
	return m, tea.Batch(m.entityFetchCmd(), m.animate())
}

// entityFetchCmd GETs the panel's next page (bound to the CURRENT noun
// and cursor — notificationsFetchCmd's shape, and directly callable from
// tests the same way).
func (m Model) entityFetchCmd() tea.Cmd {
	client, noun, after := m.client, m.entity.noun, m.entity.next
	return func() tea.Msg {
		page, err := client.FetchPickerPage(context.Background(), noun, after)
		return EntityPickerFetchedMsg{Noun: noun, Page: page, Err: err}
	}
}

func (m Model) onEntityPickerFetched(msg EntityPickerFetchedMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeEntityPicker || msg.Noun != m.entity.noun {
		return m, nil // panel closed or switched before this landed
	}
	m.entity.fetching = false
	if msg.Err != nil {
		if isUnauthorized(msg.Err) {
			m.needsLogin = true
		}
		m.entity.err = msg.Err.Error()
		return m, nil
	}
	m.entity.err = ""
	m.entity.rows = append(m.entity.rows, msg.Page.Rows...)
	m.entity.next = msg.Page.NextCursor
	m.entity.fetched = true
	return m.maybeFetchMoreEntities()
}

// visibleRows applies the client-side filter: case-insensitive substring
// on the title (the web's search-local is ILIKE %q% — same semantics).
func (p entityPicker) visibleRows() []api.PickerRow {
	if p.query == "" {
		return p.rows
	}
	q := strings.ToLower(p.query)
	var out []api.PickerRow
	for _, row := range p.rows {
		if strings.Contains(strings.ToLower(row.Title), q) {
			out = append(out, row)
		}
	}
	return out
}

func (m Model) onEntityPickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeChat
		m.entity = entityPicker{}
		return m, nil
	case "up", "ctrl+p":
		if m.entity.cursor > 0 {
			m.entity.cursor--
		}
		return m, nil
	case "down", "ctrl+n":
		if m.entity.cursor < len(m.entity.visibleRows())-1 {
			m.entity.cursor++
		}
		return m.maybeFetchMoreEntities()
	case "enter":
		rows := m.entity.visibleRows()
		if len(rows) == 0 {
			return m, nil
		}
		row := rows[min(m.entity.cursor, len(rows)-1)]
		text := fmt.Sprintf("show %s %d", m.entity.command, row.ID)
		m.mode = modeChat
		m.entity = entityPicker{}
		// The web's games_nav fills the chatbox AND submits — the
		// selection goes straight through the real send path.
		m.recordHistory(text)
		m.sounds.Send()
		return m, m.sendCmd(m.conv.UUID, text, m.contentWidth()*8)
	case "backspace":
		if m.entity.query != "" {
			runes := []rune(m.entity.query)
			m.entity.query = string(runes[:len(runes)-1])
			m.entity.cursor = 0
		}
		return m, nil
	}
	if text := msg.Key().Text; text != "" {
		m.entity.query += text
		m.entity.cursor = 0
		return m.maybeFetchMoreEntities() // eager paging while filtering
	}
	return m, nil
}

// entityPickerView renders the overlay: brand header with the right-edge
// Esc chip, the query line, the windowed rows (title left, id — and the
// channel handle for vids — dim right), the shimmer-dots loader while a
// page is in flight.
func (m Model) entityPickerView() string {
	p := m.entity
	width := m.contentWidth()

	var b strings.Builder
	head := pickerBadgeStyle.Render("pito") + " " + render.Brand(p.noun, m.truecolor)
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
	if p.query == "" {
		b.WriteString("› " + pickerDimStyle.Render(render.PitoCopy.Palette.SearchPlaceholder) + "\n")
	} else {
		b.WriteString("› " + p.query + "\n")
	}

	rows := p.visibleRows()
	var lines []string
	for i, row := range rows {
		right := fmt.Sprintf("%d", row.ID)
		if row.Handle != "" {
			right = row.Handle + " · " + right
		}
		line := "  " + row.Title
		if pad := width - lipgloss.Width(line) - lipgloss.Width(right) - 3; pad > 0 {
			line += strings.Repeat(" ", pad) + pickerDimStyle.Render(right) + "  "
		}
		if i == p.cursor {
			if pad := width - 1 - lipgloss.Width(line); pad > 0 {
				line += strings.Repeat(" ", pad)
			}
			line = lipgloss.NewStyle().Background(render.ColorElevated).Render(line)
		}
		lines = append(lines, line)
	}
	if p.fetching {
		lines = append(lines, loadingDots(m.phase, m.truecolor, width))
	}
	if p.err != "" {
		lines = append(lines, notifErrStyle.Render("  "+p.err))
	}

	// Window like the notifications panel: title(3) + help(2) margins.
	visible := m.height - 5
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
	if start > 0 {
		b.WriteString(pickerDimStyle.Render(fmt.Sprintf("  ↑ %d more", start)) + "\n")
	}
	for _, line := range lines[start:end] {
		b.WriteString(line + "\n")
	}
	if end < len(lines) {
		b.WriteString(pickerDimStyle.Render(fmt.Sprintf("  ↓ %d more", len(lines)-end)) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
