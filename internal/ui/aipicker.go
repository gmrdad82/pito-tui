// The /config ai model picker — the TUI face of the web's
// Pito::Ai::PickerComponent overlay (owner 2026-07-12: "I should see the
// same things / similar things to OpenCode or Crush or my web… skip the
// name of the model at the top of the modal near the Esc").
//
// A bare `/config ai` intercepts client-side (the server's chat JSON
// branch answers web_only by design — tui-needs ask #3) and opens this
// overlay instead. State comes from GET /settings/ai — the SAME
// Ai::PickerState assembly the web renders, so the two faces cannot
// drift — and every action persists through PATCH /settings/ai exactly
// like the web's Stimulus controller: enter selects a model (or reveals
// a key entry on a connect row, or cycles effort), ctrl+f favorites,
// ctrl+x clears the selected row's provider key, esc closes (esc inside
// a key entry backs out to the list first), typing filters model rows
// on the "provider/model" substring.
//
// Copy: every visible word is pito-authored — the chrome from
// render.PitoCopy.AiPicker (palette.ai_picker + copy.ai.picker.key_gate,
// mirrored at the pinned ref) and the status flashes mirrored verbatim
// from ai_picker_controller.js, the web's own author for those lines.
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

// aiEffortCycle is the web's EFFORT_CYCLE, verbatim — enter on the
// effort row advances one step, wrapping.
var aiEffortCycle = [4]string{"off", "low", "medium", "high"}

// aiPickerPanel is the overlay's state. rows are rebuilt from state +
// query on demand (the catalog is small); cursor indexes into that
// built slice and only ever rests on a selectable row.
type aiPickerPanel struct {
	state       *api.AiPickerState
	cursor      int
	query       string
	fetching    bool
	err         string
	flash       string
	keyProvider string // non-"" while a key entry is open (its provider)
	keyLabel    string
	keyBuf      []rune
}

// AiPickerFetchedMsg carries the picker state (or the fetch error).
type AiPickerFetchedMsg struct {
	State *api.AiPickerState
	Err   error
}

// AiSettingsPatchedMsg is one write's outcome. Ok/Fail are the flash
// words (pito's JS-authored strings, chosen at dispatch time); Refetch
// asks for a fresh state read (key writes change which models exist).
type AiSettingsPatchedMsg struct {
	Result  *api.AiSettingsResult
	Err     error
	Ok      string
	Fail    string
	Refetch bool
}

// aiPickerTrigger recognizes the picker-opening form: exactly
// "/config ai" (case-insensitive, any interior spacing) — every other
// /config shape keeps round-tripping to the server untouched.
func aiPickerTrigger(text string) bool {
	tokens := strings.Fields(strings.ToLower(text))
	return len(tokens) == 2 && tokens[0] == "/config" && tokens[1] == "ai"
}

// openAiPicker opens the overlay and fires the state fetch.
func (m Model) openAiPicker() (tea.Model, tea.Cmd) {
	m.mode = modeAiPicker
	m.aiPicker = aiPickerPanel{fetching: true}
	return m, tea.Batch(m.aiPickerFetchCmd(), m.animate())
}

func (m Model) aiPickerFetchCmd() tea.Cmd {
	client, uuid := m.client, m.conv.UUID
	return func() tea.Msg {
		state, err := client.FetchAiPicker(context.Background(), uuid)
		return AiPickerFetchedMsg{State: state, Err: err}
	}
}

func (m Model) onAiPickerFetched(msg AiPickerFetchedMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeAiPicker {
		return m, nil // closed before the state landed
	}
	m.aiPicker.fetching = false
	if msg.Err != nil {
		if isUnauthorized(msg.Err) {
			m.needsLogin = true
		}
		m.aiPicker.err = msg.Err.Error()
		return m, nil
	}
	m.aiPicker.err = ""
	m.aiPicker.state = msg.State
	m.aiPicker.cursor = m.aiPicker.clampCursor(m.aiPicker.cursor)
	return m, nil
}

// aiPatchCmd runs one PATCH with its outcome flashes bound at dispatch.
func (m Model) aiPatchCmd(patch api.AiSettingsPatch, ok, fail string, refetch bool) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		result, err := client.PatchAiSettings(context.Background(), patch)
		return AiSettingsPatchedMsg{Result: result, Err: err, Ok: ok, Fail: fail, Refetch: refetch}
	}
}

func (m Model) onAiSettingsPatched(msg AiSettingsPatchedMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeAiPicker {
		return m, nil
	}
	if msg.Err != nil {
		if isUnauthorized(msg.Err) {
			m.needsLogin = true
		}
		m.aiPicker.flash = msg.Fail
		return m, nil
	}
	m.aiPicker.flash = msg.Ok
	if st := m.aiPicker.state; st != nil && msg.Result != nil {
		// The write's echo updates what it names; grouping (favorites/
		// recents sections) refreshes on reopen, like the web's own
		// "(reopen to regroup)" wording.
		if msg.Result.Model != "" {
			st.ActiveModel = msg.Result.Model
			st.ActiveProvider = msg.Result.Provider
		}
		st.Effort = msg.Result.Effort
		st.Favorites = msg.Result.Favorites
		st.Recents = msg.Result.Recents
	}
	if msg.Refetch {
		m.aiPicker.fetching = true
		return m, tea.Batch(m.aiPickerFetchCmd(), m.animate())
	}
	return m, nil
}

// ── rows ─────────────────────────────────────────────────────────────────

type aiRowKind int

const (
	aiRowHeader aiRowKind = iota
	aiRowEffort
	aiRowModel
	aiRowConnect
	aiRowGate
)

type aiRow struct {
	kind       aiRowKind
	text       string // header title / gate copy
	keyPresent bool   // header chip state (provider sections only)
	chip       bool   // header carries a key chip at all
	provider   string
	label      string // provider label (connect rows, section-entry trailing)
	model      string
	active     bool
	favorite   bool
	trailing   string // model row's right slot ("pinned" / provider label)
	effort     string // effort row's current value ("" = off)
}

func (r aiRow) selectable() bool {
	return r.kind == aiRowEffort || r.kind == aiRowModel || r.kind == aiRowConnect
}

// buildRows mirrors the web template's order: the effort cycler (only
// when the ACTIVE provider declares reasoning), the conversation/
// favorites/recents groups (only when non-empty; entries resolved
// against the registry, unknown providers silently skipped), then one
// section per provider — connect row when keyless, models, and the
// key-gate line when a keyless section lists nothing. A live query
// filters MODEL rows on the "provider/model" substring (the web's
// #filter touches only data-row-type="model"); everything else stays.
func (p aiPickerPanel) buildRows() []aiRow {
	st := p.state
	if st == nil {
		return nil
	}
	q := strings.ToLower(p.query)
	match := func(provider, model string) bool {
		return q == "" || strings.Contains(strings.ToLower(provider+"/"+model), q)
	}
	labels := make(map[string]string, len(st.Providers))
	reasoning := ""
	for _, prov := range st.Providers {
		labels[prov.Provider] = prov.Label
		if prov.Provider == st.ActiveProvider {
			reasoning = prov.Reasoning
		}
	}

	var rows []aiRow
	if reasoning != "" && reasoning != "none" {
		rows = append(rows, aiRow{kind: aiRowEffort, provider: st.ActiveProvider, effort: st.Effort})
	}

	sections := render.PitoCopy.AiPicker.Sections
	for _, group := range []struct {
		key     string
		entries []string
	}{
		{"conversation", st.ConversationModels},
		{"favorites", st.Favorites},
		{"recents", st.Recents},
	} {
		var entryRows []aiRow
		for _, entry := range group.entries {
			provider, model, ok := strings.Cut(entry, "/")
			if !ok || model == "" {
				continue
			}
			label, known := labels[provider]
			if !known || !match(provider, model) {
				continue
			}
			entryRows = append(entryRows, aiRow{
				kind: aiRowModel, provider: provider, model: model,
				active:   provider == st.ActiveProvider && model == st.ActiveModel,
				favorite: contains(st.Favorites, entry),
				trailing: label,
			})
		}
		if len(entryRows) > 0 {
			rows = append(rows, aiRow{kind: aiRowHeader, text: sections[group.key]})
			rows = append(rows, entryRows...)
		}
	}

	for _, prov := range st.Providers {
		rows = append(rows, aiRow{
			kind: aiRowHeader, text: prov.Label,
			chip: true, keyPresent: prov.KeyPresent, provider: prov.Provider,
		})
		if !prov.KeyPresent {
			rows = append(rows, aiRow{kind: aiRowConnect, provider: prov.Provider, label: prov.Label})
		}
		for _, model := range prov.Models {
			if !match(prov.Provider, model.ID) {
				continue
			}
			trailing := ""
			if model.Pinned {
				trailing = "pinned"
			}
			rows = append(rows, aiRow{
				kind: aiRowModel, provider: prov.Provider, model: model.ID,
				active:   prov.Provider == st.ActiveProvider && model.ID == st.ActiveModel,
				favorite: contains(st.Favorites, prov.Provider+"/"+model.ID),
				trailing: trailing,
			})
		}
		if len(prov.Models) == 0 && !prov.KeyPresent {
			rows = append(rows, aiRow{kind: aiRowGate, text: render.PitoCopy.AiPicker.KeyGate})
		}
	}
	return rows
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// clampCursor rests the cursor on the nearest selectable row (searching
// forward, then backward), or 0 when nothing is selectable.
func (p aiPickerPanel) clampCursor(want int) int {
	rows := p.buildRows()
	if len(rows) == 0 {
		return 0
	}
	if want >= len(rows) {
		want = len(rows) - 1
	}
	if want < 0 {
		want = 0
	}
	for i := want; i < len(rows); i++ {
		if rows[i].selectable() {
			return i
		}
	}
	for i := want - 1; i >= 0; i-- {
		if rows[i].selectable() {
			return i
		}
	}
	return 0
}

// step moves the cursor to the next selectable row in dir (±1), staying
// put at the edges — the web's #move.
func (p aiPickerPanel) step(dir int) int {
	rows := p.buildRows()
	for i := p.cursor + dir; i >= 0 && i < len(rows); i += dir {
		if rows[i].selectable() {
			return i
		}
	}
	return p.cursor
}

// ── keys ─────────────────────────────────────────────────────────────────

func (m Model) onAiPickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := &m.aiPicker

	// An open key entry owns the keyboard: enter submits, esc backs out
	// to the list (never closes the modal — the web's staged dismiss),
	// backspace edits, anything printable appends. ctrl+f/ctrl+x and
	// navigation stay out, exactly like the web while an input is open.
	if p.keyProvider != "" {
		switch msg.String() {
		case "esc":
			p.keyProvider, p.keyLabel, p.keyBuf = "", "", nil
			return m, nil
		case "enter":
			key := strings.TrimSpace(string(p.keyBuf))
			if key == "" {
				return m, nil
			}
			provider := p.keyProvider
			p.keyProvider, p.keyLabel, p.keyBuf = "", "", nil
			return m, m.aiPatchCmd(
				api.AiSettingsPatch{Provider: provider, APIKey: key},
				provider+" key saved", "could not save key", true)
		case "backspace":
			if len(p.keyBuf) > 0 {
				p.keyBuf = p.keyBuf[:len(p.keyBuf)-1]
			}
			return m, nil
		}
		if text := msg.Key().Text; text != "" {
			p.keyBuf = append(p.keyBuf, []rune(text)...)
		}
		return m, nil
	}

	rows := p.buildRows()
	selected := aiRow{}
	if p.cursor < len(rows) {
		selected = rows[p.cursor]
	}

	switch msg.String() {
	case "esc":
		m.mode = modeChat
		m.aiPicker = aiPickerPanel{}
		return m, nil
	case "up", "ctrl+p":
		p.cursor = p.step(-1)
		return m, nil
	case "down", "ctrl+n":
		p.cursor = p.step(1)
		return m, nil
	case "enter":
		switch selected.kind {
		case aiRowModel:
			return m, m.aiPatchCmd(
				api.AiSettingsPatch{Provider: selected.provider, Model: selected.model},
				"model saved: "+selected.provider+"/"+selected.model, "could not save model", false)
		case aiRowConnect:
			p.keyProvider, p.keyLabel, p.keyBuf = selected.provider, selected.label, nil
			return m, nil
		case aiRowEffort:
			current := selected.effort
			if current == "" {
				current = "off"
			}
			next := aiEffortCycle[0]
			for i, e := range aiEffortCycle {
				if e == current {
					next = aiEffortCycle[(i+1)%len(aiEffortCycle)]
					break
				}
			}
			return m, m.aiPatchCmd(
				api.AiSettingsPatch{Effort: next},
				"effort: "+next, "could not set effort", false)
		}
		return m, nil
	case "ctrl+f":
		if selected.kind != aiRowModel {
			return m, nil
		}
		entry := selected.provider + "/" + selected.model
		return m, m.aiPatchCmd(
			api.AiSettingsPatch{Favorite: entry},
			"favorite toggled: "+entry+" (reopen to regroup)", "could not toggle favorite", false)
	case "ctrl+x":
		if selected.provider == "" {
			return m, nil
		}
		return m, m.aiPatchCmd(
			api.AiSettingsPatch{Provider: selected.provider, ClearKey: true},
			selected.provider+" key cleared", "could not clear key", true)
	case "backspace":
		if p.query != "" {
			runes := []rune(p.query)
			p.query = string(runes[:len(runes)-1])
			p.cursor = p.clampCursor(0)
		}
		return m, nil
	}
	if text := msg.Key().Text; text != "" {
		p.query += text
		p.cursor = p.clampCursor(0)
	}
	return m, nil
}

// ── view ─────────────────────────────────────────────────────────────────

// The web's text-pito (brand blue) paints the key chip's ●●●● and the
// active-model ● — the same literal aiPromptStyle already wears.
var (
	aiBrandBlue    = lipgloss.Color("#5170ff")
	aiChipOnStyle  = lipgloss.NewStyle().Foreground(aiBrandBlue)
	aiHeaderStyle  = lipgloss.NewStyle().Foreground(render.ColorPrimary).Bold(true)
	aiMarkerStyle  = lipgloss.NewStyle().Foreground(aiBrandBlue)
	aiPickerActive = lipgloss.NewStyle().Foreground(render.ColorAccent)
)

// aiPickerView renders the overlay in the house modal language: brand
// header + right-edge Esc, the search line, the windowed rows (zebra +
// ▌ stripe cursor), the web's own footer keymap, and the status flash.
func (m Model) aiPickerView() string {
	p := m.aiPicker
	width := m.contentWidth()
	copyStrings := render.PitoCopy.AiPicker

	var b strings.Builder
	head := pickerBadgeStyle.Render("pito") + " " + render.Brand(copyStrings.Title, m.truecolor)
	esc := render.Kbd(copyStrings.EscHint, m.truecolor)
	if pad := width - lipgloss.Width(head) - lipgloss.Width(esc) - 1; pad > 0 {
		head += strings.Repeat(" ", pad) + esc
	}
	b.WriteString(head + "\n")
	rule := min(width-2, 44)
	if rule < 4 {
		rule = 4
	}
	b.WriteString(pickerDimStyle.Render(strings.Repeat("─", rule)) + "\n")
	switch {
	case p.keyProvider != "":
		// The revealed key entry (the web's password input): masked
		// echo, enter submits, esc backs out. It replaces the search
		// line while open — the list below stays for context.
		entry := "› paste " + p.keyLabel + " API key: " + strings.Repeat("•", len(p.keyBuf))
		b.WriteString(entry + pickerCursorStyle.Render("▌") + "\n")
	case p.query == "":
		b.WriteString("› " + pickerDimStyle.Render(copyStrings.SearchPlaceholder) + "\n")
	default:
		b.WriteString("› " + p.query + "\n")
	}

	rows := p.buildRows()
	var lines []string
	zebraIndex := 0
	for i, row := range rows {
		selected := i == p.cursor && p.keyProvider == ""
		var line string
		switch row.kind {
		case aiRowHeader:
			title := aiHeaderStyle.Render(row.text)
			if row.chip {
				chip := pickerDimStyle.Render("no key")
				if row.keyPresent {
					chip = pickerDimStyle.Render("key ") + aiChipOnStyle.Render("●●●●")
				}
				if pad := width - lipgloss.Width(title) - lipgloss.Width(chip) - 2; pad > 0 {
					title += strings.Repeat(" ", pad) + chip
				}
			}
			line = title
			zebraIndex = 0
		case aiRowGate:
			line = "   " + pickerDimStyle.Render(row.text)
		case aiRowEffort:
			zebra := zebraRowStyle(zebraIndex)
			// The web's effort_label: the stored value, or "model
			// default" (the template's own wording) when unset.
			label := row.effort
			if label == "" {
				label = "model default"
			}
			body := zebra.Render("effort " + label)
			if selected {
				body = pickerCursorStyle.Render("effort " + label)
			}
			right := "enter cycles"
			line = " " + body
			if pad := width - 1 - lipgloss.Width(line) - lipgloss.Width(right) - 2; pad > 0 {
				line += strings.Repeat(" ", pad) + pickerDimStyle.Render(right) + "  "
			}
			zebraIndex++
		case aiRowConnect:
			zebra := zebraRowStyle(zebraIndex)
			body := zebra.Render("+ paste " + row.label + " API key")
			if selected {
				body = pickerCursorStyle.Render("+ paste " + row.label + " API key")
			}
			right := "enter"
			line = " " + body
			if pad := width - 1 - lipgloss.Width(line) - lipgloss.Width(right) - 2; pad > 0 {
				line += strings.Repeat(" ", pad) + pickerDimStyle.Render(right) + "  "
			}
			zebraIndex++
		case aiRowModel:
			zebra := zebraRowStyle(zebraIndex)
			marker := "  "
			if row.active {
				marker = aiMarkerStyle.Render("●") + " "
			}
			name := zebra.Render(row.model)
			if selected {
				name = pickerCursorStyle.Render(row.model)
			}
			body := marker + name
			if row.favorite {
				body += " " + aiPickerActive.Render("★")
			}
			line = " " + body
			if row.trailing != "" {
				if pad := width - 1 - lipgloss.Width(line) - lipgloss.Width(row.trailing) - 2; pad > 0 {
					line += strings.Repeat(" ", pad) + zebra.Render(row.trailing) + "  "
				}
			}
			zebraIndex++
		}
		if selected {
			line = cursorStripe(line, width)
		} else {
			line = " " + line
		}
		lines = append(lines, line)
	}
	if p.fetching {
		lines = append(lines, loadingDots(m.phase, m.truecolor, width))
	}
	if p.err != "" {
		lines = append(lines, notifErrStyle.Render("  "+p.err))
	}

	// Window like the other overlays: chrome above (3) + footer (2).
	visible := m.height - 7
	if visible < 3 {
		visible = 3
	}
	cursorLine := min(p.cursor, max(len(lines)-1, 0))
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

	// The web footer, word for word: ↑/↓ choose · enter select/connect ·
	// ctrl+f favorite · ctrl+x clear key.
	footer := render.KbdBare("↑/↓", m.truecolor) + pickerDimStyle.Render(" choose · ") +
		render.KbdBare("enter", m.truecolor) + pickerDimStyle.Render(" select/connect · ") +
		render.KbdBare("ctrl+f", m.truecolor) + pickerDimStyle.Render(" favorite · ") +
		render.KbdBare("ctrl+x", m.truecolor) + pickerDimStyle.Render(" clear key")
	b.WriteString(footer + "\n")
	if p.flash != "" {
		b.WriteString(pickerDimStyle.Render(p.flash) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
