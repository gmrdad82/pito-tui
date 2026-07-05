package render

import (
	"strings"

	"golang.org/x/net/html"
)

// blockTags open on a new line when converting HTML to text.
var blockTags = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "tr": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "table": true,
	"ul": true, "ol": true, "blockquote": true,
}

// htmlToText flattens prerendered HTML payloads ({body, html: true}) to
// plain text: text nodes joined, block elements becoming line breaks. The
// server renders these for the web scrollback; the TUI keeps the words and
// drops the markup.
func htmlToText(fragment string) string {
	nodes, err := html.ParseFragment(strings.NewReader(fragment), nil)
	if err != nil {
		return strings.TrimSpace(fragment)
	}
	var b strings.Builder
	for _, n := range nodes {
		walkText(n, &b)
	}
	// Collapse runs of blank lines left behind by nested blocks.
	lines := strings.Split(b.String(), "\n")
	var out []string
	blank := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if strings.TrimSpace(trimmed) == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		blank = false
		out = append(out, trimmed)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// Shimmer markers: the web tags eye-catching words with
// pito-subject-shimmer spans; the extractor wraps them in private-use
// runes so the renderer can gradient-paint exactly the same words.
const (
	ShimmerStart = '\uE000'
	ShimmerEnd   = '\uE001'
)

func walkText(n *html.Node, b *strings.Builder) {
	if n.Type == html.TextNode {
		b.WriteString(strings.ReplaceAll(n.Data, "\n", " "))
	}
	if n.Type == html.ElementNode {
		switch {
		case strings.Contains(attr(n, "class"), "pito-subject-shimmer"):
			b.WriteRune(ShimmerStart)
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walkText(child, b)
			}
			b.WriteRune(ShimmerEnd)
			return
		case n.Data == "svg":
			// Icon SVGs carry their meaning in aria-label ("Likes",
			// "Comments") — emit the word, skip the path soup.
			if label := attr(n, "aria-label"); label != "" {
				b.WriteString(label)
			}
			return
		case n.Data == "img":
			// Images never render as text: the pinned display carries
			// them (avatars included — owner call: no stand-in glyphs).
			return
		case strings.Contains(attr(n, "class"), "grid"):
			// Label/value grids (detail cards): the web separates the
			// span pairs visually; the terminal gets one pair per line.
			if pairs := gridPairs(n); pairs != "" {
				b.WriteString("\n" + pairs)
				return
			}
		case blockTags[n.Data]:
			b.WriteString("\n")
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		walkText(child, b)
		// Adjacent inline elements with no text between them (grid
		// neighbors, badge runs) must not glue their words together.
		if child.Type == html.ElementNode && child.NextSibling != nil &&
			child.NextSibling.Type == html.ElementNode {
			b.WriteString(" ")
		}
	}
	if n.Type == html.ElementNode && (n.Data == "td" || n.Data == "th") {
		b.WriteString("  ")
	}
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// gridPairs renders a label/value grid as aligned lines. Any element can
// be a cell (the channel card mixes spans with an avatar <img>). Returns
// "" when the element does not look like a pair grid.
func gridPairs(n *html.Node) string {
	var cells []string
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			var cell strings.Builder
			walkText(child, &cell)
			cells = append(cells, strings.TrimSpace(cell.String()))
		}
	}
	if len(cells) < 4 || len(cells)%2 != 0 {
		return ""
	}
	// Pairs whose value vanished (avatar images) drop entirely.
	var pairs [][2]string
	labelWidth := 0
	for i := 0; i < len(cells); i += 2 {
		if cells[i+1] == "" {
			continue
		}
		pairs = append(pairs, [2]string{cells[i], cells[i+1]})
		if w := len([]rune(cells[i])); w > labelWidth {
			labelWidth = w
		}
	}
	var b strings.Builder
	for i, pair := range pairs {
		if i > 0 {
			b.WriteString("\n")
		}
		pad := labelWidth - len([]rune(pair[0]))
		b.WriteString(pair[0] + strings.Repeat(" ", pad) + "  " + pair[1])
	}
	return b.String()
}
