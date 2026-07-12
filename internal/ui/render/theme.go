package render

import "charm.land/lipgloss/v2"

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
	// on confirmation cards and pito-token references (2.0.0).
	ColorCyan = lipgloss.Color("44")
	// ColorSubject is the 256-color stand-in for the 2.0.0 subject-shimmer
	// base (pink — mix(red, purple), owner 2026-07-12) on non-truecolor
	// terminals.
	ColorSubject = lipgloss.Color("175") // pink (owner 2026-07-12; was orange 215)
	// ColorInk is the dark foreground used on colored badge backgrounds.
	ColorInk = lipgloss.Color("232")
	// ColorZebra was the plum table-stripe background — tables no longer
	// paint a background at all (owner 2026-07-12: "align my tables to
	// Charm" — see table()'s alternating ColorDim/ColorFaint foregrounds
	// instead, Charm's own canonical lipgloss/table look). It survives as
	// the reply-affordance kbd chip's quiet background (replyAffordance) —
	// that one stays pito-family plum on purpose, don't repaint it.
	ColorZebra = lipgloss.Color("#1B142B")
	// ColorElevated is a neutral elevated-surface gray — the picker and
	// notification panels' cursor-row selection highlight. Retinted off
	// the plum family alongside the table restyle above (owner
	// 2026-07-12) so the whole app leaves plum-as-decoration together;
	// unlike ColorZebra this is a SELECTION affordance, not decoration,
	// so it keeps its background — just in a quieter, brand-neutral gray.
	ColorElevated = lipgloss.Color("#2A2E3A")
	// CharmPurple is the literal web-Charm brand purple (#7D56F4) for
	// truecolor table headers and rules. Charm's own canonical
	// lipgloss/table example (the Pokémon table in the project's README)
	// styles both header text and border rules with the 256-color "99" —
	// exactly ColorPrimary above — on any terminal; truecolor here trades
	// that ANSI approximation for the exact hex. Owner order 2026-07-12:
	// "my tables have custom colors / chroma that I don't like. can you
	// align them to Charm?"
	CharmPurple = lipgloss.Color("#7D56F4")
)
