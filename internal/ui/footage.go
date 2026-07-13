// The ctrl+f "update footage" flow (owner 2026-07-13): probe a folder of
// video files with ffprobe and send `update game footage <gameID> <hours>`
// through the ordinary chat send path — never a hand-typed command.
//
// The flow is a small state machine:
//
//  1. ctrl+f (chat mode, authenticated) first gates on ffprobe being on
//     PATH — missing binary opens the warning overlay (modeWarn) and goes
//     no further.
//  2. Gate passed → the existing show-game picker (entitypicker.go) opens,
//     retitled "footage": picking a row does NOT send `show game`, it
//     calls onFootageGamePicked below, which remembers the game and opens
//     the folder step.
//  3. FolderPicker (folderpicker.go), started at the persisted last-used
//     folder (config.LoadFootageFolder, threaded in via WithFootageFolder).
//     Confirm persists FolderPicker.CurrentPath() as the new last-used and
//     moves to the probing step.
//  4. Each selected file is probed one at a time via footageProbeCmd/
//     FootageProbedMsg — a progress line reads "N/M probed"; an unreadable
//     file counts 0 hours and is tallied as skipped. Once every file has
//     answered, the per-file hours (footageFileHours) are summed and
//     ceiled to a whole hour (footageTotalHours) — pito's footage_hours is
//     integer.
//  5. finalizeFootage sends `update game footage <gameID> <total>` through
//     the real send path (sendCmd — the reply lands in the conversation
//     like any command) and closes back to modeChat; a nonzero skip tally
//     surfaces as a notice line, mirroring pushNotice's other call site.
//     The typed `footage update <id> <hours>` form is RETIRED in pito
//     (Chat::Handlers::Footage#moved) — `update game footage` is the ONLY
//     surviving grammar (owner bug report 2026-07-13: the flow was still
//     sending the retired form and pito answered "that form moved").
//
// Esc cancels cleanly from the folder or probing step (onFootageKey);
// canceling out of the game-picker step is entitypicker's own esc, already
// unconditional. A canceled/reopened flow can never see a stale
// FootageProbedMsg land (onFootageProbed checks mode+step+index).
//
// Every step past the game pick keeps the picked game on screen: the
// FolderPicker (folder step) and the probing view both render the same
// footageBreadcrumb "footage → <title> #<id>" line (owner order 2026-07-13:
// "keep the picked game visible … so the user never loses sight of what
// they're probing for") — threaded in at pick time via WithBreadcrumb since
// the game title only exists on the picker row, not on disk anywhere.
package ui

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// footageStep names the flow's state past the game-picker (which rides
// modeEntityPicker/entityPicker.footage — see entitypicker.go).
type footageStep int

const (
	footageStepFolder footageStep = iota
	footageStepProbing
)

// footageFlow is the overlay's state for the folder + probing steps.
type footageFlow struct {
	gameID    int64
	gameTitle string
	step      footageStep
	folder    FolderPicker

	// probing progress — files is the confirmed selection; probed counts
	// answers received so far (successful or not); hoursSum accumulates
	// footageFileHours per successful probe; skipped counts unreadable
	// files (contract: "unreadable files count 0 … tallied as skipped").
	files    []string
	probed   int
	hoursSum float64
	skipped  int
}

// warnPanel is the flow's generic full-screen warning/error overlay (today
// only the ffprobe-missing gate uses it) — any key dismisses back to chat.
type warnPanel struct {
	title string
	lines []string
}

// footageExec seams os/exec for the flow's two syscalls: the ffprobe
// LookPath gate (step 1) and one ffprobe invocation per selected file
// (step 4) — sound.execer's own seam (internal/sound/player.go), mirrored
// here since the two packages share no exec abstraction and neither is
// big enough to warrant one.
type footageExec interface {
	LookPath(name string) (string, error)
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type realFootageExec struct{}

func (realFootageExec) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (realFootageExec) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	// ARRAY args, never a shell string — the stack's own security rule.
	return exec.CommandContext(ctx, name, args...).Output()
}

// withFootageExec swaps the exec seam — test-only (unexported: footage_test.go
// lives in this package and calls it directly, the same way sound_test.go
// reaches into its own package for withExec).
func withFootageExec(e footageExec) Option {
	return func(m *Model) { m.footageExec = e }
}

// WithFootageFolder wires the persisted last-used footage folder: the ui
// package never touches disk itself (cookies.json's own layering) — the
// app layer resolves config.FootagePath()/LoadFootageFolder before the
// program starts and injects both the initial value and a save callback
// bound to config.SaveFootageFolder.
func WithFootageFolder(initial string, save func(string) error) Option {
	return func(m *Model) {
		m.footageFolder = initial
		m.saveFootageFolder = save
	}
}

// ── entry point ─────────────────────────────────────────────────────────

// openFootageGate is ctrl+f's handler (onChatKey): the ffprobe requirement
// gate runs FIRST and synchronously (a LookPath stat, not network I/O —
// sound.Player's own pickPlayer does the same off the event loop).
func (m Model) openFootageGate() (tea.Model, tea.Cmd) {
	if _, err := m.footageExec.LookPath("ffprobe"); err != nil {
		return m.openWarn("footage",
			"ffprobe is required to probe footage duration.",
			"",
			"Install it from your distro's ffmpeg package — e.g. apt install",
			"ffmpeg, dnf install ffmpeg, pacman -S ffmpeg, or brew install",
			"ffmpeg — then press ctrl+f again.",
		)
	}
	return m.openFootageGamePicker()
}

// openFootageGamePicker opens the show-game picker retitled for footage —
// entitypicker.go's own machinery (fetch/filter/render) drives it
// unchanged; only entity.footage=true marks the session so its enter key
// advances the flow instead of sending `show game <id>`.
func (m Model) openFootageGamePicker() (tea.Model, tea.Cmd) {
	m.mode = modeEntityPicker
	m.entity = entityPicker{noun: "games", command: "game", footage: true}
	return m.maybeFetchMoreEntities()
}

// onFootageGamePicked is entitypicker.go's enter-key branch for a footage
// session: remember the game, open the FolderPicker at the persisted
// last-used folder (or $HOME — NewFolderPicker's own fallback), sized to
// the current terminal exactly like Model.onResize feeds its own
// width/height (folderpicker.go's View doc comment).
func (m Model) onFootageGamePicked(gameID int64, gameTitle string) (tea.Model, tea.Cmd) {
	start := m.footageFolder
	fp := NewFolderPicker(start).WithTruecolor(m.truecolor).WithBreadcrumb(footageBreadcrumb(gameTitle, gameID))
	fp, _ = fp.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m.mode = modeFootage
	m.entity = entityPicker{}
	m.footage = footageFlow{gameID: gameID, gameTitle: gameTitle, step: footageStepFolder, folder: fp}
	return m, nil
}

// ── key handling ────────────────────────────────────────────────────────

func (m Model) onFootageKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.footage.step == footageStepProbing {
		if msg.String() == "esc" {
			m.mode = modeChat
			m.footage = footageFlow{}
		}
		return m, nil
	}
	fp, _ := m.footage.folder.Update(msg)
	m.footage.folder = fp
	switch {
	case fp.Canceled():
		m.mode = modeChat
		m.footage = footageFlow{}
		return m, nil
	case fp.Confirmed():
		return m.onFootageFolderConfirmed()
	}
	return m, nil
}

func (m Model) onWarnKey(tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Contract: ANY key dismisses.
	m.mode = modeChat
	m.warn = warnPanel{}
	return m, nil
}

func (m Model) openWarn(title string, lines ...string) (tea.Model, tea.Cmd) {
	m.mode = modeWarn
	m.warn = warnPanel{title: title, lines: lines}
	return m, nil
}

// ── folder confirm → probing ───────────────────────────────────────────

// onFootageFolderConfirmed persists the confirmed folder (contract #3)
// and starts probing, or finalizes immediately when nothing was selected
// (enter can confirm on an unselected file row — folderpicker.go contract
// #5 — leaving an empty selection a legal, if unusual, path).
func (m Model) onFootageFolderConfirmed() (tea.Model, tea.Cmd) {
	files := m.footage.folder.SelectedFiles()
	folder := m.footage.folder.CurrentPath()
	saveCmd := m.footageSaveFolderCmd(folder)

	m.footage.step = footageStepProbing
	m.footage.files = files
	m.footage.probed = 0
	m.footage.hoursSum = 0
	m.footage.skipped = 0

	if len(files) == 0 {
		next, sendCmd := m.finalizeFootage()
		return next, tea.Batch(saveCmd, sendCmd)
	}
	return m, tea.Batch(saveCmd, m.footageProbeCmd(0))
}

// footageSaveFolderCmd fires the persisted-folder write in the background
// (fire-and-forget, markNotificationReadOnArrival's own shape) — a nil
// callback (no WithFootageFolder configured, e.g. a test that never wired
// one) is a no-op, not a panic.
func (m Model) footageSaveFolderCmd(folder string) tea.Cmd {
	save := m.saveFootageFolder
	if save == nil {
		return nil
	}
	return func() tea.Msg {
		_ = save(folder)
		return nil
	}
}

// ── probing ─────────────────────────────────────────────────────────────

// FootageProbedMsg carries one ffprobe answer (or its failure).
type FootageProbedMsg struct {
	Index    int
	Duration float64 // seconds; meaningless when Err != nil
	Err      error
}

// footageProbeCmd runs ffprobe on files[index] — exec.Command with ARRAY
// args, never a shell string.
func (m Model) footageProbeCmd(index int) tea.Cmd {
	ex := m.footageExec
	file := m.footage.files[index]
	return func() tea.Msg {
		out, err := ex.Output(context.Background(), "ffprobe",
			"-v", "error",
			"-show_entries", "format=duration",
			"-of", "default=noprint_wrappers=1:nokey=1",
			file,
		)
		if err != nil {
			return FootageProbedMsg{Index: index, Err: err}
		}
		duration, perr := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		if perr != nil {
			return FootageProbedMsg{Index: index, Err: perr}
		}
		return FootageProbedMsg{Index: index, Duration: duration}
	}
}

// onFootageProbed folds one ffprobe answer into the running tally and
// either fires the next probe or finalizes. The mode/step/index guard
// drops a message that lands after the flow was canceled or restarted —
// onEntityPickerFetched's own stale-reply guard, mirrored.
func (m Model) onFootageProbed(msg FootageProbedMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeFootage || m.footage.step != footageStepProbing || msg.Index != m.footage.probed {
		return m, nil
	}
	m.footage.probed++
	if msg.Err != nil {
		m.footage.skipped++
	} else {
		m.footage.hoursSum += footageFileHours(msg.Duration)
	}
	if m.footage.probed < len(m.footage.files) {
		return m, m.footageProbeCmd(m.footage.probed)
	}
	return m.finalizeFootage()
}

// footageFileHours is the retired pito snippet's exact math: each file
// rounds UP to the next half hour (never down — a 61-minute file reads as
// 1.5h, not 1h).
func footageFileHours(durationSeconds float64) float64 {
	if durationSeconds <= 0 {
		return 0
	}
	return math.Ceil(durationSeconds/3600*2) / 2
}

// footageTotalHours ceils the summed per-file hours to a whole hour — pito's
// games.footage_hours is hourly-integer.
func footageTotalHours(sum float64) int {
	return int(math.Ceil(sum))
}

// finalizeFootage sends `update game footage <gameID> <total>` through the
// real send path (entitypicker.go's own enter-branch shape: recordHistory +
// sounds.Send + sendCmd) and closes the modal. A nonzero skip tally
// surfaces as a notice line — pushNotice's other call site
// (notifications.go's "notifications need a newer pito"). The typed
// `footage update <id> <hours>` form pito used to accept is RETIRED
// (Chat::Handlers::Footage#moved) — `update game footage` is the ONLY
// surviving grammar (lib/pito/chat/handlers/update.rb, GAME_FIELDS).
func (m Model) finalizeFootage() (Model, tea.Cmd) {
	total := footageTotalHours(m.footage.hoursSum)
	text := fmt.Sprintf("update game footage %d %d", m.footage.gameID, total)
	skipped := m.footage.skipped

	m.mode = modeChat
	m.footage = footageFlow{}
	if skipped > 0 {
		word := "file"
		if skipped != 1 {
			word = "files"
		}
		m.pushNotice(fmt.Sprintf("footage: %d unreadable %s skipped", skipped, word))
		m.refreshViewport()
	}
	m.recordHistory(text)
	m.sounds.Send()
	return m, m.sendCmd(m.conv.UUID, text, m.contentWidth()*8)
}

// ── view ─────────────────────────────────────────────────────────────────

// warnLineStyle paints the warning overlay's body — ColorWarn, the same
// hue warnBanner uses for the "not logged in" banner.
var warnLineStyle = lipgloss.NewStyle().Foreground(render.ColorWarn)

// footageGameStyle picks out the picked game's title inside the breadcrumb —
// ColorAccent, the house "current value" pink already worn by the picker
// cursor stripe and the scope hint's live channel/period value, so the one
// thing that changes step to step (the game) is the one thing that pops.
var footageGameStyle = lipgloss.NewStyle().Foreground(render.ColorAccent).Bold(true)

// footageProgressStyle is the probing line's non-truecolor fallback — flat
// ColorPrimary, same tier-down as render.Brand's own "lesser terminals get
// the primary" rule.
var footageProgressStyle = lipgloss.NewStyle().Foreground(render.ColorPrimary).Bold(true)

// footageBreadcrumb renders the persistent "footage → <title> #<id>" line
// every step past the game pick shows (owner order 2026-07-13: "keep the
// picked game visible … so the user never loses sight of what they're
// probing for") — dim wayfinding bookends around the one thing that's
// actually live right now, the picked game, the same shape a breadcrumb
// trail always takes. WithBreadcrumb (folderpicker.go) and
// footageProbingView below are its only two renderers, so the folder and
// probing steps can never drift apart on how the game is shown.
func footageBreadcrumb(title string, id int64) string {
	return pickerDimStyle.Render("footage → ") + footageGameStyle.Render(title) +
		pickerDimStyle.Render(fmt.Sprintf(" #%d", id))
}

func (m Model) footageView() string {
	if m.footage.step == footageStepProbing {
		return m.footageProbingView()
	}
	return m.footage.folder.View()
}

// footageProbingView renders the probing step: the usual pito-badge chrome,
// the persistent game breadcrumb (contract above), a "probing N/M…" line
// riding the brand shimmer on truecolor terminals (render.PitoShimmer, the
// same ramp Kbd/Brand sample — no new color introduced) so an otherwise
// static line between ffprobe answers still reads as alive, and the house
// loadingDots row underneath — the SAME "something is in flight" glyph the
// game/folder pickers use for their own async fetches, reused rather than
// inventing a second spinner language for the one async step that's local
// (an ffprobe subprocess) instead of a network fetch.
func (m Model) footageProbingView() string {
	width := m.contentWidth()
	var b strings.Builder
	head := pickerBadgeStyle.Render("pito") + " " + render.Brand("footage", m.truecolor)
	b.WriteString(head + "\n")
	rule := min(width-2, 44)
	if rule < 4 {
		rule = 4
	}
	b.WriteString(pickerDimStyle.Render(strings.Repeat("─", rule)) + "\n")
	b.WriteString(footageBreadcrumb(m.footage.gameTitle, m.footage.gameID) + "\n\n")

	progress := fmt.Sprintf("probing %d/%d…", m.footage.probed, len(m.footage.files))
	if m.truecolor {
		b.WriteString(render.PitoShimmer.Colorize(progress, m.phase))
	} else {
		b.WriteString(footageProgressStyle.Render(progress))
	}
	if m.footage.skipped > 0 {
		b.WriteString(pickerDimStyle.Render(fmt.Sprintf("  (%d skipped)", m.footage.skipped)))
	}
	if dots := loadingDots(m.phase, m.truecolor, width); dots != "" {
		b.WriteString("\n" + dots)
	}
	return b.String()
}

// warnView renders the generic warning/error overlay: the same badge +
// brand header + dim rule every other modal opens with, an "any key" hint
// standing in for the usual Esc chip (contract: any key dismisses, not
// just Esc), the message lines in ColorWarn. The ⚠ glyph riding the brand
// header mirrors warnBanner/errBanner's own " ⚠ …" prefix (model.go) — the
// house's one way of marking "this line is a warning", reused rather than
// a bespoke glyph for this overlay.
func (m Model) warnView() string {
	width := m.contentWidth()
	var b strings.Builder
	head := pickerBadgeStyle.Render("pito") + " " + render.Brand("⚠ "+m.warn.title, m.truecolor)
	hint := render.Kbd("any key", m.truecolor)
	if pad := width - lipgloss.Width(head) - lipgloss.Width(hint) - 1; pad > 0 {
		head += strings.Repeat(" ", pad) + hint
	}
	b.WriteString(head + "\n")
	rule := min(width-2, 44)
	if rule < 4 {
		rule = 4
	}
	b.WriteString(pickerDimStyle.Render(strings.Repeat("─", rule)) + "\n\n")
	for _, line := range m.warn.lines {
		if line == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(warnLineStyle.Render(line) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
