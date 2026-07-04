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

func walkText(n *html.Node, b *strings.Builder) {
	if n.Type == html.TextNode {
		b.WriteString(strings.ReplaceAll(n.Data, "\n", " "))
	}
	if n.Type == html.ElementNode && blockTags[n.Data] {
		b.WriteString("\n")
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		walkText(child, b)
	}
	if n.Type == html.ElementNode && (n.Data == "td" || n.Data == "th") {
		b.WriteString("  ")
	}
}
