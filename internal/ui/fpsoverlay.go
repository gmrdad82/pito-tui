// The F9 frame-rate chip: a small top-left "NN fps" readout, same key and
// same shape in pito web, pitomd, and here (owner cross-repo order).
// Mirrors scrollnav.go's paintScrollNavOverlay — a chip stamped over the
// viewport's first visible line — except flush LEFT instead of right, and
// with no server copy involved: the chip's one token is a plain measured
// count, computed entirely client-side.
//
// Design note (Bubble Tea reality): renders happen on Update, not a fixed
// clock, so an idle TUI genuinely paints ~0 times a second — that number
// would be honest (it IS the frame rate) but would read as a frozen chip.
// startFPSTick/onFPSTick below run a self-rescheduling ~100ms tea.Tick
// LOOP while the chip is on: each tick is itself one repaint (so the chip
// keeps breathing at idle, a floor of ~10) and, because View() stamps on
// every real render regardless of what triggered it, the same loop also
// measures whatever true rate a busier moment (typing, scrolling, a
// streaming reply) actually achieves. One mechanism, two jobs — see
// fpsCounter.stamp for where the counting itself happens.
package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// fpsTickInterval: ~10 nudges a second — enough that the idle chip never
// looks stuck, cheap enough it costs nothing next to the shimmer loop's
// own 60fps (shimmerTick).
const fpsTickInterval = 100 * time.Millisecond

func fpsTick() tea.Cmd {
	return tea.Tick(fpsTickInterval, func(time.Time) tea.Msg { return FPSTickMsg{} })
}

// startFPSTick begins the chip's own idle-nudge loop — a no-op if the
// chip is off or a chain is already running (mirrors ambient.go's
// startHeartbeat/heartbeatTicking shape exactly: the boolean is set here,
// the ONE place that clears it again is onFPSTick itself, so there is
// never more than one live chain rescheduling regardless of how fast f9
// is mashed).
func (m *Model) startFPSTick() tea.Cmd {
	if !m.fpsOn || m.fpsTicking {
		return nil
	}
	m.fpsTicking = true
	return fpsTick()
}

// onFPSTick reschedules itself while the chip stays on; the instant f9
// turns it off, this is the single authoritative point that lets the
// chain die (one more harmless tick after toggling off, exactly like
// onHeartbeatTick's own lazy stop).
func (m Model) onFPSTick() (tea.Model, tea.Cmd) {
	if !m.fpsOn {
		m.fpsTicking = false
		return m, nil
	}
	return m, fpsTick()
}

// fpsCounter is F9's sliding-window frame counter: one timestamp per real
// View() paint, pruned to the trailing 1s on every stamp. A plain slice
// rather than a fixed ring — pruning already keeps it bounded to however
// many paints actually land inside one second, never more.
type fpsCounter struct {
	frames []time.Time
}

// stamp records one paint at now, drops every stamp older than 1s, and
// returns the resulting count — the number the chip shows this frame. A
// true sliding window: the count is always "paints in the last second,"
// never a fixed-period average.
func (c *fpsCounter) stamp(now time.Time) int {
	c.frames = append(c.frames, now)
	cutoff := now.Add(-time.Second)
	i := 0
	for i < len(c.frames) && c.frames[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		c.frames = c.frames[i:]
	}
	return len(c.frames)
}

// fpsChipText renders the chip's one token in the house dim style
// (pickerDimStyle, the same muted foreground scrollnav.go's glyph wears).
func fpsChipText(count int) string {
	return pickerDimStyle.Render(fmt.Sprintf("%d fps", count))
}

// paintFPSOverlay stamps the chip over the viewport's first visible line,
// flush LEFT with a one-cell margin — the mirror of scrollnav.go's
// paintScrollNavOverlay, which stamps its pills flush right on the same
// line. chip == "" is a no-op so callers never need their own guard
// beyond the fpsOn check that already skips computing it.
func paintFPSOverlay(body, chip string, width int) string {
	if chip == "" {
		return body
	}
	lines := strings.Split(body, "\n")
	if len(lines) == 0 {
		return body
	}
	cw := lipgloss.Width(chip)
	if cw+1 > width {
		return body // terminal too narrow to fit the chip at all
	}
	rest := ansi.TruncateLeft(lines[0], cw+1, "")
	lines[0] = chip + " " + rest
	return strings.Join(lines, "\n")
}
