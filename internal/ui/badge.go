package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// aiSparkle is the picker's AI badge glyph — one visible cell.
const aiSparkle = "✦"

var aiBadgeStyle = lipgloss.NewStyle().Foreground(render.ColorPrimary)

// aiBadge renders the picker's AI sparkle for a conversation with an
// ai-kind event (tui-needs ask 9b). Truecolor terminals ride the AIAccent
// gradient through the same Colorize path the house shimmer uses, so the
// badge rides the model's GLOBAL phase and shares its cadence/angle;
// lesser terminals get a static primary-colored glyph instead.
func aiBadge(phase float64, truecolor bool) string {
	if truecolor {
		return render.AIAccent.Colorize(aiSparkle, phase)
	}
	return aiBadgeStyle.Render(aiSparkle)
}
