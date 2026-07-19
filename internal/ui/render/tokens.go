package render

import (
	"strings"

	"charm.land/lipgloss/v2"
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
		strings.ContainsRune(s, StarMark) || strings.ContainsRune(s, ShinyStart) ||
		strings.ContainsRune(s, TokenStart) || strings.ContainsRune(s, HeaderStart)
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
	goldStyle := func(runIdx int) lipgloss.Style {
		st := lipgloss.NewStyle().Bold(true).Background(base.GetBackground())
		if !r.truecolor {
			return st.Foreground(ColorWarn)
		}
		// A glint travels the coin run — each coin catches the light in
		// turn, Mario-style; a lone coin (or the star) twinkles gently.
		c := glossBoost(coinGold, RGB{0xff, 0xfa, 0xdc}, runIdx, 0, 4, r.staggered20("coins"), 0.6, 1.1)
		return st.Foreground(hex(c))
	}
	// lastGlyph survives buffered spaces so a coin RUN stacks tight
	// (●●●) while exactly one space separates the run from the number.
	lastGlyph := false
	coinRun := 0
	coin := func(glyph string) {
		if lastGlyph && strings.TrimSpace(plain.String()) == "" {
			plain.Reset() // swallow inter-glyph spaces
		} else {
			coinRun = 0
		}
		flush()
		out.WriteString(goldStyle(coinRun).Render(glyph))
		coinRun++
		lastGlyph = true
	}
	var token *strings.Builder
	var shiny *strings.Builder
	for _, ru := range text {
		if token != nil {
			if ru == TokenEnd {
				// Reference token — static cyan, like the web (2.0.0).
				out.WriteString(lipgloss.NewStyle().Foreground(ColorCyan).
					Background(base.GetBackground()).Render(token.String()))
				token = nil
				lastGlyph = false
			} else {
				token.WriteRune(ru)
			}
			continue
		}
		if shiny != nil {
			if ru == ShinyEnd {
				// Up to THREE segments: material, face, and (extended
				// badges only) the raw web unlock date. SplitN(...,3) keeps
				// old two-segment payloads (no date) rendering exactly as
				// before — len(parts) just comes back 2, date stays "".
				parts := strings.SplitN(shiny.String(), string(ShinySep), 3)
				if len(parts) >= 2 {
					face := decodeShinySpace(parts[1])
					date := ""
					if len(parts) == 3 {
						date = r.shinyDateSuffix(decodeShinySpace(parts[2]))
					}
					out.WriteString(r.ShinyBadge(parts[0], face, date))
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
		case TokenStart:
			flush()
			token = &strings.Builder{}
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

// decodeShinySpace restores the ShinySpace placeholders (html.go's
// escapeSpaces) back into literal spaces — shared by paintTokens and
// plainTokens so both decode a shiny segment's face/date text the same
// way.
func decodeShinySpace(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ShinySpace {
			return ' '
		}
		return r
	}, s)
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
	phase := r.staggered20("chip-" + label)
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
// Gaussian sheen (blur-soft, never hard-edged) toward a warm white.
var glassTint = RGB{0xff, 0xf6, 0xe8}

func glassBoost(c RGB, i, cells int, phase float64) RGB {
	sigma := float64(cells)/2.4 + 1.4
	return glossBoost(c, glassTint, i, 0, cells, phase, 0.38, sigma)
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
				// Plain surfaces (table cells) show the face only — the
				// date (when a third segment is present) stays out, same
				// as coins/stars carry no text of their own here. SplitN
				// limit 3 keeps this in step with paintTokens so a
				// three-segment payload never leaks a stray ShinySep rune
				// into parts[1].
				parts := strings.SplitN(shiny.String(), string(ShinySep), 3)
				if len(parts) >= 2 {
					out.WriteString(decodeShinySpace(parts[1]))
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
		case TokenStart, TokenEnd, HeaderStart, HeaderEnd:
			// Reference tokens and headers keep their text, lose the styling.
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

// shinyGlowEnabled is the badge glow-up's single kill switch (owner smoke
// 2026-07-12: "Charm's magic to make them look cooler here too" — the web
// badge's breathing halo, travelling specular gleam and iridescent fire).
// true ships all of it: a sine-breathing edge color, a tightened gleam
// that also catches 1-2 characters of the ink text as it crosses, an
// occasional trailing ✦ twinkle on pearl/opal/diamond, and the extended
// unlock-date suffix. false — the one-line rollback — renders the badge
// exactly as it did before this pass: original wide gleam, static edge,
// plain ink, no sparkle, no date. Every effect below is ALSO gated on
// r.truecolor already; this const only ever touches that branch, never
// the non-truecolor static pill (which this flag never changes).
const shinyGlowEnabled = true

// shinyDateSuffix prints the web's unlock date exactly as it arrives —
// single-source-of-truth rule: a pre-formatted payload string prints
// verbatim, the TUI never re-derives it. This used to drop the year
// itself when it matched stamp()'s "now" (mirroring stamp()'s day-aware
// rule at month granularity); that year-drop is moving server-side into
// badge_component.rb (current year → "%b", other years → "%b '%y" — until
// that lands the web still sends "%b '%y" for every year), and
// re-formatting here would only risk drifting out of sync with it.
func (r *R) shinyDateSuffix(webDate string) string {
	return webDate
}

// inkGlintThreshold/inkGlintStrength bound the specular gleam's reach
// into the badge's ink text (ShinyBadge): below the threshold a glyph's
// background gleam intensity is too faint to read as a text highlight at
// all, so the ink stays exactly m.ink; above it, the ink lerps toward the
// gleam's own tint by at most inkGlintStrength — never further, so the
// text catches the light without ever bleaching past the material's
// palette. With the tightened gleam sigma (below), only the 1-2 cells
// nearest the peak ever clear the threshold on a typical badge width.
const (
	inkGlintThreshold = 0.4
	inkGlintStrength  = 0.5
)

// inkGlintFactor turns a raw background-gleam intensity g (glossFactor's
// Gaussian, 0..1) into how far a glyph's ink lerps toward the gleam's
// tint — a pure, bounded function so the brightening can be asserted
// directly without parsing rendered ANSI.
func inkGlintFactor(g float64) float64 {
	if g < inkGlintThreshold {
		return 0
	}
	f := g * inkGlintStrength
	if f > inkGlintStrength {
		f = inkGlintStrength
	}
	return f
}

// sparkleCycleTicks/sparkleWindowTicks translate the brief's "once every
// ~4s, 3 ticks long" into shimmerTick (model.go's real 40ms animation
// tick) counts — the exact same shape ui/micro.go's
// glintCycleTicks/glintWindowTicks use for the confirm dialog's own
// glint. r.phase alone can't drive this: it's already wrapped at 1 with
// its OWN ~2.667s period (1/shimmerStep ticks), and scaling an
// already-wrapped ramp can only shorten a period, never lengthen one — so
// this rides r.ticks instead, model.go's raw aliveTicks forwarded via
// SetTicks every real tick (not a new ticker).
const (
	sparkleCycleTicks  = int64(250) // 4.000s at the house's 16ms shimmer tick (60fps, 2026-07-12)
	sparkleWindowTicks = int64(8)   // ~130ms burst, same wall-clock as the old 3×40ms
)

// sparkleActive reports whether seed's iridescent twinkle is inside its
// 3-tick visible window at r.ticks' current position. phaseOffset hashes
// the seed into a per-badge starting offset within the cycle (the same
// primitive staggered/staggered20 use) so a wall of iridescent badges
// never twinkles in sync — deterministic for a pinned r.ticks, so tests
// can assert the exact window. The shinyGlowEnabled/iridescent gate lives
// at the call site (ShinyBadge), not here, so this cadence math stays
// testable on its own.
func (r *R) sparkleActive(seed string) bool {
	offset := int64(phaseOffset(seed) * float64(sparkleCycleTicks))
	pos := (r.ticks + offset) % sparkleCycleTicks
	if pos < 0 {
		pos += sparkleCycleTicks
	}
	return pos < sparkleWindowTicks
}

// ShinyBadge renders one achievement badge: the material's gradient body
// (lo→hi→lo, the web's 135deg pill), ink text, edge caps, a gleam
// sweeping the surface on its own stagger, and — when shinyGlowEnabled —
// a breathing halo on the edge caps, the gleam also catching the ink
// text, an iridescent trailing twinkle, and (when date is non-empty) a
// dim unlock-date suffix. date is "" for compact badges (detail card
// strips never carry one) and extended badges with no recorded unlock
// date; non-empty only for extended badges (tokens.go's paintTokens,
// fed by html.go's three-segment shiny marker). The REUSABLE shiny
// component — detail cards, shinies replies, anything badge-shaped.
func (r *R) ShinyBadge(material, face, date string) string {
	m := materialFor(material)
	if len([]rune(face)) > shinyCompactMax {
		face = string([]rune(face)[:shinyCompactMax-1]) + "…"
	}
	if !r.truecolor {
		// Static badges only — today's look, byte-identical; the date
		// suffix and every glow effect below are truecolor-only.
		return lipgloss.NewStyle().Reverse(true).Bold(true).Render(" " + face + " ")
	}
	if !shinyGlowEnabled {
		date = "" // the rollback: pre-glow-up badges never carried a date
	}
	grad := StopGradient{Stops: []GradientStop{{m.lo, 0}, {m.hi, 0.5}, {m.lo, 1}}}
	gleamPhase := r.staggered20("shiny-" + face)
	// The gleam is TINTED like the web's per-material --gleam: the edge
	// color lifted toward white — amber gleams warm, jade gleams green.
	tint := RGB{
		uint8(float64(m.edge.R) + (255-float64(m.edge.R))*0.6),
		uint8(float64(m.edge.G) + (255-float64(m.edge.G))*0.6),
		uint8(float64(m.edge.B) + (255-float64(m.edge.B))*0.6),
	}
	gleamMax := 0.34
	if m.iridescent {
		// Pearl/opal/diamond IRIDESCE: the gleam's own hue cycles
		// through the brand ramp as it travels.
		tint = PitoShimmer.At(r.staggered("sheen-" + face))
		tint = RGB{
			uint8(float64(tint.R) + (255-float64(tint.R))*0.55),
			uint8(float64(tint.G) + (255-float64(tint.G))*0.55),
			uint8(float64(tint.B) + (255-float64(tint.B))*0.55),
		}
		gleamMax = 0.5
	}
	// label is the pill's full text: " <face> " alone, or " <face> · <date> "
	// when the badge is extended and carries an unlock date. faceEnd marks
	// where the face run (eligible for the ink glint below) ends and the
	// dim date suffix begins.
	label := " " + face
	faceEnd := len([]rune(label))
	if date != "" {
		label += " · " + date
	}
	label += " "
	runes := []rune(label)
	// Wider, gentler wave: the pill spans caps + text as ONE surface so
	// the glint enters and exits with the same softness on both sides.
	total := len(runes) + 2
	sigma := float64(total)/2.4 + 1.4
	if shinyGlowEnabled {
		// Tighter than the surface gloss (owner ask 2026-07-12: "reads as
		// a glint, not a wash") — the peak now covers roughly 2-3 cells
		// instead of washing most of the pill warm at once.
		sigma = float64(total)/7.2 + 0.9
	}
	cell := func(pos int) RGB {
		t := float64(pos) / float64(max(total-1, 1))
		bg := grad.At(t)
		return glossBoost(bg, tint, pos, 0, total, gleamPhase, gleamMax, sigma)
	}
	edge := m.edge
	if shinyGlowEnabled {
		// Breathing halo: the edge caps' own color pulses ±25% around
		// m.edge on a slow per-badge sine — the terminal analog of the
		// web's box-shadow halo (pito-shiny-breathe), since terminals
		// have no glow-radius to animate.
		edge = scaleRGB(m.edge, 1+r.phasePulse20("halo-"+face)*0.25)
	}
	rightCap := "▕"
	if shinyGlowEnabled && m.iridescent && r.sparkleActive("sparkle-"+face) {
		rightCap = "✦"
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(hex(edge)).Background(hex(cell(0))).Render("▎"))
	for i, ru := range runes {
		pos := i + 1
		ink := m.ink
		bold := true
		switch {
		case i < faceEnd && shinyGlowEnabled:
			// The gleam crosses the label — near its peak, 1-2 face
			// characters brighten toward the same tint the surface wash
			// uses, reading as a glint riding the text, not just the fill.
			if g := inkGlintFactor(glossFactor(pos, 0, total, gleamPhase, sigma)); g > 0 {
				ink = lerpRGB(m.ink, tint, g)
			}
		case i >= faceEnd:
			// The unlock date: a dim, non-bold suffix (the web's
			// .pito-shiny__date opacity: .78, blended toward the pill's
			// own background here since terminal cells have no alpha).
			bold = false
			ink = lerpRGB(cell(pos), m.ink, 0.78)
		}
		b.WriteString(lipgloss.NewStyle().
			Background(hex(cell(pos))).
			Foreground(hex(ink)).
			Bold(bold).
			Render(string(ru)))
	}
	b.WriteString(lipgloss.NewStyle().Foreground(hex(edge)).Background(hex(cell(total - 1))).Render(rightCap))
	return b.String()
}
