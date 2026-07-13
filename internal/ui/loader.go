package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// loaderDots is the five-dot glyph row painted while a list fetches its
// next page — notifications panel first, the /resume picker next.
const loaderDots = "∙ ∙ ∙ ∙ ∙"

var loaderDotsStyle = lipgloss.NewStyle().Foreground(render.ColorDim)

// loadingDots renders the house "fetching more" row: five middle dots
// centered in width. Truecolor terminals ride the dots' colors along
// PitoShimmer's traveling band via the SAME Colorize path as badge.go's
// aiBadge and model.go's "thinking…" line — phase is the model's global
// shimmer phase, so a loader dropped into any panel stays in lockstep
// with the rest of the house animation (no private ticker). Lesser
// terminals get a static dim row instead. Returns "" below width 10 —
// too narrow for the row to mean anything.
func loadingDots(phase float64, truecolor bool, width int) string {
	if width < 10 {
		return ""
	}
	dots := loaderDots
	visible := lipgloss.Width(dots)
	if truecolor {
		dots = render.PitoShimmer.Colorize(dots, phase)
	} else {
		dots = loaderDotsStyle.Render(dots)
	}
	if pad := (width - visible) / 2; pad > 0 {
		dots = strings.Repeat(" ", pad) + dots
	}
	return dots
}

// loadingDotsLeft is the same in-flight marker without the centering pad —
// panels that read strictly from the left edge (the footage probe) use it
// so their progress never floats mid-screen (owner 2026-07-13).
func loadingDotsLeft(phase float64, truecolor bool) string {
	dots := loaderDots
	if truecolor {
		return render.PitoShimmer.Colorize(dots, phase)
	}
	return loaderDotsStyle.Render(dots)
}
