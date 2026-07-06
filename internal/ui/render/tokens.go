package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Token painting — platform chips and price coins. The html flattener
// marks them with private-use runes (html.go); this pass turns markers
// into styled glyphs: brand-colored chips with white text and a glassy
// sweep, gold Mario coins, the gold FREE star. Plain stretches keep the
// caller's base style so zebra stripes stay continuous.

var coinGold = RGB{0xf5, 0xc5, 0x42}

// platformBrand maps the web alt labels onto chip label + brand color.
func platformBrand(alt string) (string, RGB) {
	switch strings.ToLower(strings.TrimSpace(alt)) {
	case "playstation", "ps", "ps4", "ps5":
		return "PS", RGB{0x00, 0x70, 0xd1}
	case "switch", "nintendo switch":
		return "Switch", RGB{0xe6, 0x00, 0x12}
	case "xbox":
		return "Xbox", RGB{0x10, 0x7c, 0x10}
	case "steam":
		return "Steam", RGB{0x1b, 0x28, 0x38}
	default:
		if alt == "" {
			alt = "?"
		}
		return alt, RGB{0x55, 0x55, 0x66}
	}
}

// hasTokenMarkers reports marker runes worth a paint pass.
func hasTokenMarkers(s string) bool {
	return strings.ContainsRune(s, ChipStart) || strings.ContainsRune(s, CoinMark) ||
		strings.ContainsRune(s, StarMark) || strings.ContainsRune(s, ShinyStart)
}

// paintTokens renders a marker-carrying string: chips, coins, stars and
// base-styled plain segments. base carries the row's own look (dim keys,
// zebra background) so every plain stretch continues it seamlessly.
func (r *R) paintTokens(text string, base lipgloss.Style) string {
	var out strings.Builder
	var plain strings.Builder
	var chip *strings.Builder
	flush := func() {
		if plain.Len() > 0 {
			out.WriteString(base.Render(plain.String()))
			plain.Reset()
		}
	}
	goldStyle := lipgloss.NewStyle().Bold(true).Background(base.GetBackground())
	if r.truecolor {
		goldStyle = goldStyle.Foreground(hex(coinGold))
	} else {
		goldStyle = goldStyle.Foreground(ColorWarn)
	}
	// lastGlyph survives buffered spaces so a coin RUN stacks tight
	// (●●●) while exactly one space separates the run from the number.
	lastGlyph := false
	coin := func(glyph string) {
		if lastGlyph && strings.TrimSpace(plain.String()) == "" {
			plain.Reset() // swallow inter-glyph spaces
		}
		flush()
		out.WriteString(goldStyle.Render(glyph))
		lastGlyph = true
	}
	var shiny *strings.Builder
	for _, ru := range text {
		if shiny != nil {
			if ru == ShinyEnd {
				parts := strings.SplitN(shiny.String(), string(ShinySep), 2)
				if len(parts) == 2 {
					face := strings.Map(func(r rune) rune {
						if r == ShinySpace {
							return ' '
						}
						return r
					}, parts[1])
					out.WriteString(r.ShinyBadge(parts[0], face))
				}
				shiny = nil
			} else {
				shiny.WriteRune(ru)
			}
			continue
		}
		switch ru {
		case ShinyStart:
			flush()
			shiny = &strings.Builder{}
			lastGlyph = false
		case ChipStart:
			flush()
			chip = &strings.Builder{}
			lastGlyph = false
		case ChipEnd:
			if chip != nil {
				out.WriteString(r.platformChip(chip.String()))
				chip = nil
				lastGlyph = false
			}
		case CoinMark:
			coin("●")
		case StarMark:
			coin("★")
		default:
			if chip != nil {
				chip.WriteRune(ru)
				continue
			}
			if ru != ' ' && lastGlyph {
				// First non-space after a glyph run: the web's CSS gap
				// is exactly one space, whatever the flattener buffered.
				plain.Reset()
				plain.WriteRune(' ')
				lastGlyph = false
			}
			plain.WriteRune(ru)
		}
	}
	if chip != nil { // unterminated: degrade to plain
		plain.WriteString(chip.String())
	}
	flush()
	return out.String()
}

// platformChip renders one platform as a brand-colored chip — white bold
// text on the platform's color, with a glassy highlight band sweeping
// across the surface (staggered per platform, riding the shimmer phase).
func (r *R) platformChip(alt string) string {
	label, brand := platformBrand(alt)
	text := " " + label + " "
	if !r.truecolor {
		return lipgloss.NewStyle().Reverse(true).Bold(true).Render(text)
	}
	phase := staggered20(r.phase, "chip-"+label)
	runes := []rune(text)
	var b strings.Builder
	for i, ru := range runes {
		bg := glassBoost(brand, i, len(runes), phase)
		b.WriteString(lipgloss.NewStyle().
			Background(hex(bg)).
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true).
			Render(string(ru)))
	}
	return b.String()
}

// glassBoost lightens the chip surface where the glint passes — a
// narrow, soft highlight that reads as gloss.
func glassBoost(c RGB, i, cells int, phase float64) RGB {
	span := float64(cells) + 6
	center := phase*span - 3
	d := float64(i) - center
	if d < 0 {
		d = -d
	}
	if d > 2 {
		return c
	}
	f := 0.45 * (2 - d) / 2
	lerp := func(x uint8) uint8 { return uint8(float64(x) + (255-float64(x))*f) }
	return RGB{lerp(c.R), lerp(c.G), lerp(c.B)}
}

// plainTokens strips marker runes for surfaces that must stay unstyled
// text (table cells): chips become their short labels, coins and stars
// become nothing (the number beside them carries the value).
func plainTokens(text string) string {
	if !hasTokenMarkers(text) {
		return text
	}
	var out strings.Builder
	var chip *strings.Builder
	var shiny *strings.Builder
	for _, ru := range text {
		if shiny != nil {
			switch ru {
			case ShinyEnd:
				parts := strings.SplitN(shiny.String(), string(ShinySep), 2)
				if len(parts) == 2 {
					out.WriteString(strings.Map(func(r rune) rune {
						if r == ShinySpace {
							return ' '
						}
						return r
					}, parts[1]))
				}
				shiny = nil
			default:
				shiny.WriteRune(ru)
			}
			continue
		}
		switch ru {
		case ShinyStart:
			shiny = &strings.Builder{}
		case ChipStart:
			chip = &strings.Builder{}
		case ChipEnd:
			if chip != nil {
				label, _ := platformBrand(chip.String())
				out.WriteString(label)
				chip = nil
			}
		case CoinMark, StarMark:
		default:
			if chip != nil {
				chip.WriteRune(ru)
			} else {
				out.WriteRune(ru)
			}
		}
	}
	if chip != nil {
		out.WriteString(chip.String())
	}
	return out.String()
}

// shinyMaterial is one entry of pito's material palette (application.css
// [data-material=…]): gradient body lo→hi, ink text, edge.
type shinyMaterial struct {
	lo, hi, ink, edge RGB
	iridescent        bool
}

var shinyMaterials = map[string]shinyMaterial{
	"wood":    {RGB{0x57, 0x35, 0x1a}, RGB{0x7d, 0x4f, 0x27}, RGB{0xf7, 0xe6, 0xc4}, RGB{0xa0, 0x6a, 0x35}, false},
	"stone":   {RGB{0x45, 0x4e, 0x5c}, RGB{0x68, 0x75, 0x8a}, RGB{0xf0, 0xf4, 0xfa}, RGB{0x8b, 0x98, 0xad}, false},
	"amber":   {RGB{0x9a, 0x5c, 0x0a}, RGB{0xcf, 0x8a, 0x18}, RGB{0x2f, 0x1c, 0x00}, RGB{0xff, 0xca, 0x5f}, false},
	"coral":   {RGB{0xb8, 0x40, 0x2f}, RGB{0xf0, 0x70, 0x5a}, RGB{0x2a, 0x0b, 0x04}, RGB{0xff, 0x9a, 0x82}, false},
	"jade":    {RGB{0x0b, 0x5a, 0x42}, RGB{0x12, 0x92, 0x6b}, RGB{0xd8, 0xff, 0xee}, RGB{0x2f, 0xd3, 0x9c}, false},
	"pearl":   {RGB{0xd9, 0xd6, 0xd0}, RGB{0xf7, 0xf5, 0xf0}, RGB{0x2e, 0x34, 0x40}, RGB{0xff, 0xff, 0xff}, true},
	"ruby":    {RGB{0x6e, 0x0e, 0x1f}, RGB{0xa8, 0x1c, 0x33}, RGB{0xff, 0xd9, 0xde}, RGB{0xe2, 0x44, 0x5e}, false},
	"opal":    {RGB{0xde, 0xd7, 0xec}, RGB{0xf6, 0xf2, 0xff}, RGB{0x3a, 0x2d, 0x52}, RGB{0xff, 0xff, 0xff}, true},
	"silver":  {RGB{0x8f, 0x97, 0xa1}, RGB{0xe9, 0xed, 0xf2}, RGB{0x1f, 0x28, 0x30}, RGB{0xf4, 0xf7, 0xfa}, false},
	"gold":    {RGB{0x8f, 0x67, 0x00}, RGB{0xff, 0xd7, 0x5e}, RGB{0x2e, 0x20, 0x00}, RGB{0xff, 0xe9, 0xa3}, false},
	"diamond": {RGB{0x8e, 0xcb, 0xe9}, RGB{0xf2, 0xfb, 0xff}, RGB{0x08, 0x2a, 0x44}, RGB{0xff, 0xff, 0xff}, true},
}

func materialFor(name string) shinyMaterial {
	if m, ok := shinyMaterials[strings.ToLower(strings.TrimSpace(name))]; ok {
		return m
	}
	return shinyMaterial{RGB{0x44, 0x44, 0x50}, RGB{0x5c, 0x5c, 0x6c}, RGB{0xee, 0xee, 0xf4}, RGB{0x8a, 0x8a, 0x9a}, false}
}

// shinyCompactMax caps the badge face like the web's compact form —
// longer faces trim with an ellipsis ("200 Vie…").
const shinyCompactMax = 12

// ShinyBadge renders one achievement badge: the material's gradient
// body (lo→hi→lo, the web's 135deg pill), ink text, edge caps, and a
// gleam sweeping the surface on its own stagger. The REUSABLE shiny
// component — detail cards, shinies replies, anything badge-shaped.
func (r *R) ShinyBadge(material, face string) string {
	m := materialFor(material)
	if len([]rune(face)) > shinyCompactMax {
		face = string([]rune(face)[:shinyCompactMax-1]) + "…"
	}
	text := " " + face + " "
	if !r.truecolor {
		return lipgloss.NewStyle().Reverse(true).Bold(true).Render(text)
	}
	grad := StopGradient{Stops: []GradientStop{{m.lo, 0}, {m.hi, 0.45}, {m.lo, 1}}}
	phase := staggered20(r.phase, "shiny-"+face)
	runes := []rune(text)
	gleamMax := 0.35
	if m.iridescent {
		gleamMax = 0.6 // pearl/opal/diamond throw more light
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(hex(m.edge)).Background(hex(m.lo)).Render("▎"))
	for i, ru := range runes {
		bg := grad.At(float64(i) / float64(max(len(runes)-1, 1)))
		bg = gleamBoost(bg, i, len(runes), phase, gleamMax)
		b.WriteString(lipgloss.NewStyle().
			Background(hex(bg)).
			Foreground(hex(m.ink)).
			Bold(true).
			Render(string(ru)))
	}
	b.WriteString(lipgloss.NewStyle().Foreground(hex(m.edge)).Background(hex(m.lo)).Render("▕"))
	return b.String()
}

// gleamBoost is glassBoost with a tunable strength — the badge gleam.
func gleamBoost(c RGB, i, cells int, phase, strength float64) RGB {
	span := float64(cells) + 6
	center := phase*span - 3
	d := float64(i) - center
	if d < 0 {
		d = -d
	}
	if d > 2 {
		return c
	}
	f := strength * (2 - d) / 2
	lerp := func(x uint8) uint8 { return uint8(float64(x) + (255-float64(x))*f) }
	return RGB{lerp(c.R), lerp(c.G), lerp(c.B)}
}
