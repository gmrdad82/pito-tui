package render

import (
	"encoding/json"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/net/html"
)

// The at-a-glance panel (`show … with at-a-glance`, the glance verb):
// five metric cells, each a 2-row braille sparkline with a label+scalar
// pair beneath — the legend that gives the curve its scalar meaning. The
// braille rows are the web's own BrailleAreaChart output (42 cols),
// lifted verbatim from the payload so the curve is identical by
// construction. Cells sit two-up when the terminal is wide enough,
// stacked otherwise (owner call).

type glanceCell struct {
	label  string
	value  string
	rows   []string
	noData bool
}

type glancePanel struct {
	intro string
	cells []glanceCell
	nudge string
	note  string
}

// hasPendingGlance reports a glance payload whose AnalyticsFillJob has
// not landed yet (analytics.status marker — the glance sibling of the
// analyze `analyze` marker).
func hasPendingGlance(payload []byte) bool {
	var p struct {
		Analytics struct {
			Status string `json:"status"`
		} `json:"analytics"`
	}
	return json.Unmarshal(payload, &p) == nil && p.Analytics.Status == "pending"
}

func glanceIntroText(payload []byte) string {
	var p struct {
		Analytics struct {
			Intro string `json:"intro"`
		} `json:"analytics"`
	}
	if json.Unmarshal(payload, &p) == nil && p.Analytics.Intro != "" {
		return htmlToText(p.Analytics.Intro)
	}
	return "The numbers, at a glance."
}

// parseGlance recognizes a ready glance body and pulls the cells out.
func parseGlance(fragment string) (*glancePanel, bool) {
	if !strings.Contains(fragment, "pito-analytics-scalars") {
		return nil, false
	}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), nil)
	if err != nil {
		return nil, false
	}
	g := &glancePanel{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			class := attr(n, "class")
			switch {
			case strings.Contains(class, "__intro"):
				var b strings.Builder
				walkText(n, &b)
				g.intro = strings.TrimSpace(b.String())
				return
			case strings.Contains(class, "__nudge"):
				g.nudge = nodeText(n)
				return
			case strings.Contains(class, "__note"):
				g.note = nodeText(n)
				return
			case strings.Contains(class, "scalars__cell"):
				g.cells = append(g.cells, parseGlanceCell(n))
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
	if len(g.cells) == 0 && g.note == "" {
		return nil, false
	}
	return g, true
}

func parseGlanceCell(n *html.Node) glanceCell {
	cell := glanceCell{}
	var find func(*html.Node)
	find = func(m *html.Node) {
		if m.Type == html.ElementNode {
			class := attr(m, "class")
			switch {
			case strings.Contains(class, "pito-metric--nodata"):
				cell.noData = true
			case strings.Contains(class, "pito-metric__row") && !strings.Contains(class, "bg-row"):
				// The plot rows — the web's pre-rendered braille chart.
				// The dotted-paper background (__bg-row) stays behind.
				cell.rows = append(cell.rows, textOf(m))
				return
			case strings.Contains(class, "scalars__label"):
				cell.label = nodeText(m)
				return
			case strings.Contains(class, "scalars__value"):
				cell.value = nodeText(m)
				return
			}
		}
		for child := m.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(n)
	return cell
}

// glancePanel renders the parsed panel: intro, metric cells (two-up when
// they fit, stacked otherwise), nudge below.
func (r *R) glancePanel(g *glancePanel) string {
	width := r.width - 3
	parts := []string{}
	if g.intro != "" {
		parts = append(parts, r.paintShimmer(g.intro))
	}
	if g.note != "" {
		parts = append(parts, r.dimCopy(g.note))
		return strings.Join(parts, "\n\n")
	}

	cellW := 42
	if cellW > width {
		cellW = width
	}
	blocks := make([]string, 0, len(g.cells))
	for _, cell := range g.cells {
		blocks = append(blocks, r.glanceCell(cell, cellW))
	}
	twoUp := width >= cellW*2+4
	var laid []string
	if twoUp {
		for i := 0; i < len(blocks); i += 2 {
			if i+1 < len(blocks) {
				laid = append(laid, lipgloss.JoinHorizontal(lipgloss.Top, blocks[i], "    ", blocks[i+1]))
			} else {
				laid = append(laid, blocks[i])
			}
		}
	} else {
		laid = blocks
	}
	parts = append(parts, strings.Join(laid, "\n\n"))
	if g.nudge != "" {
		parts = append(parts, r.dimCopy(g.nudge))
	}
	return strings.Join(parts, "\n\n")
}

// glanceCell renders one metric: the 2-row braille curve (dim when the
// metric has no data) over its label+scalar legend line. The web's
// dotted-paper grid shows through wherever the curve is blank, and the
// pito-blue shimmer band sweeps the curve like every other chart.
func (r *R) glanceCell(cell glanceCell, cellW int) string {
	lines := r.paintBraille(cell.rows, cellW, cell.noData)
	value := cell.value
	if cell.noData && value == "" {
		value = "—"
	}
	pair := r.dim(cell.label) + " " + value
	if pairW := lipgloss.Width(pair); pairW < cellW {
		pair += strings.Repeat(" ", cellW-pairW)
	}
	lines = append(lines, pair)
	return strings.Join(lines, "\n")
}
