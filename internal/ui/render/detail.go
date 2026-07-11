package render

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/net/html"
)

// The show-card renderer — `show game/vid/channel` detail bodies. The web
// lays these out as a two-column card (cover art left, kv grid right);
// the terminal renders one column, no images (owner lock): intro, a
// zebra kv table of the details (stats + shinies folded in), the score +
// TTB bars 1:1 with the web components, and the description on its own.

type scoreData struct {
	label    string
	score    int // -1 = muted (no data)
	chipLeft bool
}

type ttbTickData struct {
	key string
	pct float64
}

type ttbData struct {
	label        string
	stops        []GradientStop
	ticks        []ttbTickData
	footageText  string
	footagePct   float64
	footageLeft  bool
	values       []positionedText
	legend       []string
	legendColors []RGB
}

type detailCard struct {
	intro string
	// statsPairs is the Stats/Shinies block — its own table ABOVE the
	// details (owner call 2026-07-06), a blank row between them.
	statsPairs [][2]string
	pairs      [][2]string
	score      *scoreData
	ttb        *ttbData
	descLabel  string
	descText   string
	// Tags moved out of the kv grid into their own hairline section
	// (pito db74203f) — video cards only today.
	tagsLabel string
	tagsText  string
}

// tickColor maps the web's data-accent keys onto the shared heat palette
// ([data-accent=…] rules in application.css).
func tickColor(key string) RGB {
	switch key {
	case "main":
		return heatGreen
	case "extras":
		return heatLime
	case "completionist":
		return heatPink
	default: // footage → fg-default
		return RGB{0xda, 0xda, 0xda}
	}
}

// parseDetailCard recognizes a pito detail-card body and pulls out the
// renderable pieces. Returns false for anything that isn't a show card.
func parseDetailCard(fragment string) (*detailCard, bool) {
	// Full show cards carry the stats column; the vid's linked-game card
	// is the same kv anatomy without it.
	if !strings.Contains(fragment, "pito-detail-stats") &&
		!strings.Contains(fragment, "pito-video-linked-game-card") {
		return nil, false
	}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), nil)
	if err != nil {
		return nil, false
	}
	card := &detailCard{}
	for _, n := range nodes {
		card.walk(n)
	}
	if len(card.pairs) == 0 {
		return nil, false
	}
	return card, true
}

func hasClass(n *html.Node, token string) bool {
	return strings.Contains(" "+attr(n, "class")+" ", token)
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	walkText(n, &b)
	return strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(b.String(), "\n", " ")), " "))
}

func (c *detailCard) walk(n *html.Node) {
	if n.Type == html.ElementNode {
		class := attr(n, "class")
		switch {
		case strings.Contains(class, "__intro"), strings.Contains(class, "linked-game-intro"):
			var b strings.Builder
			walkText(n, &b)
			c.intro = strings.TrimSpace(b.String())
			return
		case strings.Contains(class, "pito-detail-stats"):
			c.parseStats(n)
			return
		case strings.Contains(class, "grid-cols"):
			c.parseGrid(n)
			return
		case strings.Contains(class, "pito-score-bar"):
			c.parseScore(n)
			return
		case strings.Contains(class, "pito-ttb") && attr(n, "data-component") == "time-to-beat":
			c.parseTTB(n)
			return
		case strings.Contains(class, "__description"):
			c.descText = descriptionText(n)
			if c.descLabel == "" {
				c.descLabel = "Description"
			}
			return
		case strings.Contains(class, "__tags"):
			c.tagsText = descriptionText(n)
			if c.tagsLabel == "" {
				c.tagsLabel = "Tags"
			}
			return
		case strings.Contains(class, "text-fg-dim") && n.Data == "div":
			// Section labels are anonymous dim divs right before their
			// bodies (description / tags sections).
			if sib := nextElement(n.NextSibling); sib != nil {
				sibClass := attr(sib, "class")
				switch {
				case strings.Contains(sibClass, "__description"):
					c.descLabel = nodeText(n)
				case strings.Contains(sibClass, "__tags"):
					c.tagsLabel = nodeText(n)
				}
			}
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		c.walk(child)
	}
}

func nextElement(n *html.Node) *html.Node {
	for ; n != nil; n = n.NextSibling {
		if n.Type == html.ElementNode {
			return n
		}
	}
	return nil
}

// descriptionText keeps the description's paragraph text (pre-wrap on the
// web) as plain trimmed prose.
func descriptionText(n *html.Node) string {
	var b strings.Builder
	walkText(n, &b)
	return strings.TrimSpace(b.String())
}

// parseStats folds the left column's Stats / Shinies rows into kv pairs:
// heading divs are keys, the sibling counter/badge containers are values.
func (c *detailCard) parseStats(n *html.Node) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}
		class := attr(child, "class")
		if !strings.Contains(class, "-heading") {
			continue
		}
		value := nextElement(child.NextSibling)
		if value == nil {
			continue
		}
		key := nodeText(child)
		var vals []string
		for cell := value.FirstChild; cell != nil; cell = cell.NextSibling {
			if cell.Type == html.ElementNode {
				if t := nodeText(cell); t != "" {
					vals = append(vals, t)
				}
			}
		}
		text := strings.Join(vals, " · ")
		if text == "" {
			text = nodeText(value)
		}
		if key != "" && text != "" {
			c.statsPairs = append(c.statsPairs, [2]string{key, text})
		}
	}
}

// parseGrid extracts the kv rows: alternating label/value cells. Values
// that flatten to nothing (inline avatars — images never render) drop.
func (c *detailCard) parseGrid(n *html.Node) {
	var cells []string
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			var cell strings.Builder
			walkText(child, &cell)
			cells = append(cells, strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(cell.String(), "\n", " ")), " ")))
		}
	}
	for i := 0; i+1 < len(cells); i += 2 {
		if cells[i+1] == "" {
			continue
		}
		c.pairs = append(c.pairs, [2]string{stripShimmerMarkers(cells[i]), cells[i+1]})
	}
}

func stripShimmerMarkers(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ShimmerStart || r == ShimmerEnd {
			return -1
		}
		return r
	}, s)
}

func (c *detailCard) parseScore(n *html.Node) {
	sd := &scoreData{score: -1}
	if v, err := strconv.Atoi(attr(n, "data-score")); err == nil && !hasClass(n, "pito-score-bar--muted") {
		sd.score = v
	}
	var find func(*html.Node)
	find = func(m *html.Node) {
		if m.Type == html.ElementNode {
			class := attr(m, "class")
			if strings.Contains(class, "__label") {
				// Keep the server's ljust padding verbatim — it aligns the
				// score and TTB brackets in the same card.
				sd.label = textOf(m)
			}
			if strings.Contains(class, "--value-left") {
				sd.chipLeft = true
			}
		}
		for child := m.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(n)
	c.score = sd
}

func textOf(n *html.Node) string {
	var b strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			b.WriteString(child.Data)
		}
	}
	return b.String()
}

func (c *detailCard) parseTTB(n *html.Node) {
	td := &ttbData{}
	var find func(*html.Node)
	find = func(m *html.Node) {
		if m.Type == html.ElementNode {
			class := attr(m, "class")
			switch {
			case strings.Contains(class, "__label"):
				td.label = textOf(m) // server-padded, aligns brackets
				return
			case strings.Contains(class, "__fill"):
				td.stops = parseHeatStops(attr(m, "style"))
				return
			case strings.Contains(class, "__footage-marker"):
				td.footageText = strings.TrimSpace(textOf(m))
				td.footagePct = leftPct(attr(m, "style"))
				td.footageLeft = strings.Contains(class, "--value-left")
				return
			case strings.Contains(class, "__tick") && !strings.Contains(class, "legend"):
				td.ticks = append(td.ticks, ttbTickData{key: attr(m, "data-accent"), pct: leftPct(attr(m, "style"))})
				return
			case strings.Contains(class, "__value--pillar"):
				td.values = append(td.values, positionedText{
					Text:  strings.TrimSpace(nodeText(m)),
					Pct:   leftPct(attr(m, "style")),
					AtEnd: strings.Contains(class, "--at-end"),
				})
				return
			case strings.Contains(class, "__legend-item"):
				key, label := "", ""
				for child := m.FirstChild; child != nil; child = child.NextSibling {
					if child.Type != html.ElementNode {
						continue
					}
					if strings.Contains(attr(child, "class"), "legend-tick") {
						key = attr(child, "data-accent")
					} else {
						label = nodeText(child)
					}
				}
				if label != "" {
					td.legend = append(td.legend, label)
					td.legendColors = append(td.legendColors, tickColor(key))
				}
				return
			}
		}
		for child := m.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(n)
	c.ttb = td
}

// leftPct pulls the percent out of an inline `left: N%;` style.
func leftPct(style string) float64 {
	i := strings.Index(style, "left:")
	if i < 0 {
		return 0
	}
	rest := strings.TrimSpace(style[i+5:])
	end := strings.IndexByte(rest, '%')
	if end < 0 {
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(rest[:end]), 64)
	if err != nil {
		return 0
	}
	return v
}

// parseHeatStops reads the TTB fill's inline linear-gradient: stop
// positions come through verbatim; colors are classified by their CSS
// accent mix (the terminal palette stands in for the theme mixes).
func parseHeatStops(style string) []GradientStop {
	i := strings.Index(style, "linear-gradient(to right")
	if i < 0 {
		return nil
	}
	body := style[i:]
	if j := strings.IndexByte(body, ','); j >= 0 {
		body = body[j+1:]
	}
	// Split on top-level commas (color-mix() nests its own).
	var segments []string
	depth, start := 0, 0
	for k, ch := range body {
		switch ch {
		case '(':
			depth++
		case ')':
			if depth == 0 { // the gradient's own closing paren
				segments = append(segments, body[start:k])
				body = ""
			} else {
				depth--
			}
		case ',':
			if depth == 0 {
				segments = append(segments, body[start:k])
				start = k + 1
			}
		}
		if body == "" {
			break
		}
	}
	var stops []GradientStop
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		fields := strings.Fields(seg)
		if len(fields) == 0 {
			continue
		}
		pctText := strings.TrimSuffix(fields[len(fields)-1], "%")
		pct, err := strconv.ParseFloat(pctText, 64)
		if err != nil {
			continue
		}
		stops = append(stops, GradientStop{Color: heatColorFor(seg), Pos: pct / 100})
	}
	return stops
}

// heatColorFor classifies one gradient stop expression by its accents —
// mirrors HEAT_THRESHOLDS and the partial-data terminal colors.
func heatColorFor(expr string) RGB {
	switch {
	case strings.Contains(expr, "accent-red"), strings.Contains(expr, "accent-purple"):
		return heatPink
	case strings.Contains(expr, "accent-orange"):
		return heatAmber
	case strings.Contains(expr, "accent-green") && strings.Contains(expr, "accent-yellow"):
		return heatLime
	case strings.Contains(expr, "accent-yellow"):
		return heatYellow
	default:
		return heatGreen
	}
}

// detailCard renders the parsed card: intro / zebra kv table / hairline /
// score + TTB bars / hairline / description. Pieces are optional — the
// video and channel cards simply have no bars.
func (r *R) detailCard(c *detailCard) string {
	width := r.width - 3
	var parts []string

	if c.intro != "" {
		parts = append(parts, r.paintShimmer(c.intro))
	}
	// Two tables: Stats/Shinies first (plain), a blank row, then the
	// zebra details.
	if len(c.statsPairs) > 0 {
		parts = append(parts, r.kvRows(c.statsPairs, width, false))
	}
	if len(c.pairs) > 0 {
		parts = append(parts, r.kvRows(c.pairs, width, true))
	}

	var bars []string
	if c.score != nil {
		label := c.score.label
		if label == "" {
			label = "Score"
		}
		// The server ljusts the score/TTB labels to a shared width and
		// the web adds a CSS gap — one space is that gap, for both bars.
		label += " "
		bars = append(bars, r.ScoreBar(label, c.score.score, width))
	}
	if c.ttb != nil {
		bars = append(bars, r.ttbBlock(c.ttb, width))
	}
	if len(bars) > 0 {
		parts = append(parts, r.hairline(width), strings.Join(bars, "\n"))
	}

	if c.descText != "" {
		parts = append(parts, r.hairline(width), r.dim(c.descLabel)+"\n"+c.descText)
	}
	if c.tagsText != "" {
		parts = append(parts, r.hairline(width), r.dim(c.tagsLabel)+"\n"+c.tagsText)
	}
	return strings.Join(parts, "\n\n")
}

func (r *R) hairline(width int) string {
	if width < 1 {
		width = 1
	}
	return lipgloss.NewStyle().Foreground(ColorFaint).Render(strings.Repeat("─", width))
}

// kvRows renders label/value pairs — zebra-striped for the details
// table, plain for the Stats/Shinies block. Long values wrap INSIDE the
// value column (the web grid's behavior); every wrapped line keeps its
// row's stripe.
func (r *R) kvRows(pairs [][2]string, width int, zebra bool) string {
	keyWidth := 0
	for _, pair := range pairs {
		if w := lipgloss.Width(pair[0]); w > keyWidth {
			keyWidth = w
		}
	}
	valWidth := width - keyWidth - 3
	if valWidth < 20 {
		valWidth = 20
	}
	var rows []string
	for i, pair := range pairs {
		key, value := pair[0], stripShimmerMarkers(pair[1])
		keyStyle := lipgloss.NewStyle().Foreground(ColorDim)
		valStyle := lipgloss.NewStyle()
		if pair[1] != value { // value carried shimmer markers → accent it
			valStyle = valStyle.Foreground(ColorAccent)
		}
		if zebra && i%2 == 1 {
			keyStyle = keyStyle.Background(ColorZebra)
			valStyle = valStyle.Background(ColorZebra)
		}
		for li, vline := range wrapPlain(value, valWidth) {
			keyCell := strings.Repeat(" ", keyWidth+3)
			if li == 0 {
				pad := keyWidth - lipgloss.Width(key)
				keyCell = " " + key + strings.Repeat(" ", pad) + "  "
			}
			line := keyStyle.Render(keyCell) + r.paintTokens(vline, valStyle)
			if fill := width - lipgloss.Width(line); fill > 0 && zebra && i%2 == 1 {
				line += lipgloss.NewStyle().Background(ColorZebra).Render(strings.Repeat(" ", fill))
			}
			rows = append(rows, line)
		}
	}
	return strings.Join(rows, "\n")
}

// wrapPlain word-wraps plain text to width; words longer than a line
// stand alone (the bar's own wrap would shear them anyway).
func wrapPlain(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(current)+1+lipgloss.Width(word) <= width {
			current += " " + word
			continue
		}
		lines = append(lines, current)
		current = word
	}
	return append(lines, current)
}

// ttbBlock renders the TTB bar with its hour-values row and legend — the
// three-row anatomy of the web component.
func (r *R) ttbBlock(t *ttbData, width int) string {
	label := t.label
	if label == "" {
		label = "Time to beat"
	}
	label += " " // the web's CSS gap — matches the score bar's spacing
	var ticks []BarTick
	for _, tick := range t.ticks {
		bt := BarTick{Pct: tick.pct, Color: tickColor(tick.key), Bold: true}
		if tick.key == "footage" && t.footageText != "" {
			bt.Chip = t.footageText
			bt.ChipLeft = t.footageLeft
		}
		ticks = append(ticks, bt)
	}
	fill := StopGradient{Stops: t.stops}
	if len(fill.Stops) == 0 {
		fill = StopGradient{Stops: []GradientStop{{heatGreen, 0}, {heatLime, 1}}}
	}
	lines := []string{r.barLine(label, fill, ticks, width, false)}
	if len(t.values) > 0 {
		lines = append(lines, r.positionRow(label, t.values, width, lipgloss.NewStyle().Foreground(ColorDim)))
	}
	if len(t.legend) > 0 {
		var items []string
		for i, name := range t.legend {
			tickStyle := lipgloss.NewStyle().Bold(true)
			if r.truecolor {
				tickStyle = tickStyle.Foreground(hex(t.legendColors[i]))
			}
			items = append(items, tickStyle.Render("|")+r.dim(" "+name))
		}
		lines = append(lines, strings.Repeat(" ", lipgloss.Width(label)+1)+strings.Join(items, "  "))
	}
	return strings.Join(lines, "\n")
}
