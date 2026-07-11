package render

import "github.com/charmbracelet/lipgloss"

// The pito palette — fixed 256-color values chosen for dark terminals
// (the product's home turf) that stay legible on light ones. Deliberately
// NOT lipgloss.AdaptiveColor: adaptive resolution queries the terminal
// background at render time, which deadlocks under Bubble Tea's input
// reader — the same trap as glamour's auto style.
var (
	// ColorPrimary is pito purple — brand accents, enhanced blocks.
	ColorPrimary = lipgloss.Color("99")
	// ColorAccent is the pink used for the user's own presence: echo
	// blocks, the prompt, reply handles.
	ColorAccent = lipgloss.Color("205")
	// ColorOK / ColorWarn / ColorErr drive connection state and banners.
	ColorOK   = lipgloss.Color("78")
	ColorWarn = lipgloss.Color("221")
	ColorErr  = lipgloss.Color("203")
	// ColorDim and ColorFaint are the two grays: metadata and hints.
	ColorDim   = lipgloss.Color("245")
	ColorFaint = lipgloss.Color("241")
	// ColorCyan mirrors the web's text-cyan (#22d3ee): detail-block keys
	// on confirmation cards.
	ColorCyan = lipgloss.Color("44")
	// ColorInk is the dark foreground used on colored badge backgrounds.
	ColorInk = lipgloss.Color("232")
	// ColorZebra tints alternate table rows — a whisper of plum (owner
	// 2026-07-11: the old #332052 read too contrasting; this stays
	// pito family, not battleship gray.
	ColorZebra = lipgloss.Color("#1B142B")
)
