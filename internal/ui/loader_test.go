package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestLoadingDotsTruecolorRidesPhase(t *testing.T) {
	withTrueColor(t)
	if loadingDots(0, true, 40) == loadingDots(0.5, true, 40) {
		t.Error("truecolor loader must vary with phase — it rides the global shimmer sweep")
	}
}

func TestLoadingDotsNonTruecolorIsStatic(t *testing.T) {
	withTrueColor(t) // profile shouldn't matter — the non-truecolor path never touches the gradient
	if loadingDots(0, false, 40) != loadingDots(0.5, false, 40) {
		t.Error("non-truecolor loader must stay static across phase")
	}
}

func TestLoadingDotsVisibleWidthNeverExceedsWidth(t *testing.T) {
	withTrueColor(t)
	for _, width := range []int{10, 11, 15, 20, 30, 44, 80} {
		for _, truecolor := range []bool{true, false} {
			line := loadingDots(0.25, truecolor, width)
			if w := lipgloss.Width(line); w > width {
				t.Errorf("width=%d truecolor=%v: visible width = %d, exceeds %d", width, truecolor, w, width)
			}
		}
	}
}

func TestLoadingDotsIsCentered(t *testing.T) {
	withTrueColor(t)
	for _, width := range []int{10, 11, 15, 20, 30, 44, 80} {
		for _, truecolor := range []bool{true, false} {
			line := loadingDots(0.4, truecolor, width)
			leading := len(line) - len(strings.TrimLeft(line, " "))
			visible := lipgloss.Width(line)
			want := (width - (visible - leading)) / 2
			if diff := leading - want; diff < -1 || diff > 1 {
				t.Errorf("width=%d truecolor=%v: leading spaces = %d, want ≈ %d (visible=%d)", width, truecolor, leading, want, visible)
			}
		}
	}
}

func TestLoadingDotsTooNarrowIsEmpty(t *testing.T) {
	withTrueColor(t)
	for _, width := range []int{-1, 0, 1, 9} {
		for _, truecolor := range []bool{true, false} {
			if got := loadingDots(0, truecolor, width); got != "" {
				t.Errorf("width=%d truecolor=%v: loadingDots = %q, want \"\"", width, truecolor, got)
			}
		}
	}
	if got := loadingDots(0, true, 10); got == "" {
		t.Error("width=10 must render — the floor is exclusive, not inclusive")
	}
}
