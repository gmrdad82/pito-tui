package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/net/html"
)

// The shinies message (`shinies channel|vid|game <ref>`, `#handle
// shinies`) — pito G127's achievement lanes. One lane per metric: the
// RAIL (a tick per ladder step in that step's material — reached lit,
// the next step pulsing, the rest dim; awards squared) with its legend,
// then the obtained badges flowing beneath. Badges reuse ShinyBadge.

type railTick struct {
	material string
	lit      bool
	next     bool
	award    bool
}

type shinyLane struct {
	label  string
	ticks  []railTick
	legend string
	badges string // marker-carrying text; paintTokens renders the pills
}

type shiniesMessage struct {
	intro string
	lanes []shinyLane
}

func parseShinies(fragment string) (*shiniesMessage, bool) {
	if !strings.Contains(fragment, "pito-shiny-rail") {
		return nil, false
	}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), nil)
	if err != nil {
		return nil, false
	}
	msg := &shiniesMessage{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			class := attr(n, "class")
			switch {
			case strings.Contains(class, "__intro"):
				var b strings.Builder
				walkText(n, &b)
				msg.intro = strings.TrimSpace(b.String())
				return
			case strings.Contains(class, "pito-achievement-metric-row"):
				msg.lanes = append(msg.lanes, parseLane(n))
				return
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	if len(msg.lanes) == 0 {
		return nil, false
	}
	return msg, true
}

func parseLane(n *html.Node) shinyLane {
	lane := shinyLane{}
	var find func(*html.Node)
	find = func(m *html.Node) {
		if m.Type == html.ElementNode {
			class := attr(m, "class")
			switch {
			case strings.Contains(class, "rail__label"):
				lane.label = nodeText(m)
				return
			case strings.Contains(class, "rail__tick") && !strings.Contains(class, "rail__ticks"):
				lane.ticks = append(lane.ticks, railTick{
					material: attr(m, "data-material"),
					lit:      strings.Contains(class, "is-lit"),
					next:     strings.Contains(class, "is-next"),
					award:    strings.Contains(class, "is-award") || isAwardMaterial(attr(m, "data-material")),
				})
				return
			case strings.Contains(class, "rail__legend"):
				lane.legend = nodeText(m)
				return
			case strings.Contains(class, "metric-row__badges"):
				var b strings.Builder
				walkText(m, &b)
				lane.badges = strings.TrimSpace(b.String())
				return
			}
		}
		for child := m.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(n)
	return lane
}

func isAwardMaterial(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "silver", "gold", "diamond":
		return true
	}
	return false
}

// shiniesMessage renders the lanes: label + rail + legend, badges below,
// a breath between lanes.
func (r *R) shiniesMessage(msg *shiniesMessage) string {
	width := r.width - 3
	labelWidth := 0
	for _, lane := range msg.lanes {
		if w := lipgloss.Width(lane.label); w > labelWidth {
			labelWidth = w
		}
	}
	parts := []string{}
	if msg.intro != "" {
		parts = append(parts, r.paintShimmer(msg.intro))
	}
	for _, lane := range msg.lanes {
		var b strings.Builder
		pad := strings.Repeat(" ", labelWidth-lipgloss.Width(lane.label))
		b.WriteString(r.dim(lane.label) + pad + "  ")
		b.WriteString(r.rail(lane.ticks))
		if lane.legend != "" {
			b.WriteString("  " + r.dim(lane.legend))
		}
		if lane.badges != "" {
			indent := strings.Repeat(" ", labelWidth+2)
			base := lipgloss.NewStyle()
			for _, line := range wrapPlain(lane.badges, width-labelWidth-2) {
				b.WriteString("\n" + indent + r.paintTokens(line, base))
			}
		}
		parts = append(parts, b.String())
	}
	return strings.Join(parts, "\n\n")
}

// rail draws the ladder: lit ticks in their material's bright tone, the
// next step pulsing with the phase, unreached steps dim; award steps
// wear squares.
func (r *R) rail(ticks []railTick) string {
	var b strings.Builder
	for _, tick := range ticks {
		glyph := "●"
		if tick.award {
			glyph = "■"
		}
		m := materialFor(tick.material)
		switch {
		case tick.next && r.truecolor:
			// The next threshold breathes: brightness rides the phase.
			pulse := (1 + phasePulse(r.phase, tick.material)) / 2
			c := RGB{
				uint8(float64(m.hi.R)*0.45 + float64(m.hi.R)*0.55*pulse),
				uint8(float64(m.hi.G)*0.45 + float64(m.hi.G)*0.55*pulse),
				uint8(float64(m.hi.B)*0.45 + float64(m.hi.B)*0.55*pulse),
			}
			b.WriteString(lipgloss.NewStyle().Foreground(hex(c)).Bold(true).Render("◉"))
		case tick.next:
			b.WriteString(lipgloss.NewStyle().Foreground(ColorWarn).Bold(true).Render("◉"))
		case tick.lit && r.truecolor:
			b.WriteString(lipgloss.NewStyle().Foreground(hex(m.hi)).Render(glyph))
		case tick.lit:
			b.WriteString(lipgloss.NewStyle().Foreground(ColorOK).Render(glyph))
		default:
			b.WriteString(lipgloss.NewStyle().Foreground(ColorFaint).Render("·"))
		}
	}
	return b.String()
}
