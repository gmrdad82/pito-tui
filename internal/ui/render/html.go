package render

import (
	"strings"

	"charm.land/lipgloss/v2"
	"golang.org/x/net/html"
)

// blockTags open on a new line when converting HTML to text.
var blockTags = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "tr": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "table": true,
	"ul": true, "ol": true, "blockquote": true,
}

// FlattenHTML is htmlToText for callers outside the package (the
// notifications panel flattens server messages carrying inline tags).
func FlattenHTML(fragment string) string { return htmlToText(fragment) }

// htmlToText flattens prerendered HTML payloads ({body, html: true}) to
// plain text: text nodes joined, block elements becoming line breaks. The
// server renders these for the web scrollback; the TUI keeps the words and
// drops the markup. Its every-other-caller contract (tables/kv/detail
// parsers across the package) stays exactly as-is — see RenderMessageHTML
// below for the color-preserving sibling used by message bodies.
func htmlToText(fragment string) string {
	nodes, err := html.ParseFragment(strings.NewReader(fragment), nil)
	if err != nil {
		return strings.TrimSpace(fragment)
	}
	var b strings.Builder
	for _, n := range nodes {
		walkText(n, &b)
	}
	return collapseBlankLines(b.String())
}

// RenderMessageHTML is htmlToText's color-preserving sibling: system and
// confirmation message bodies that never earned a structured card
// (parseDetailCard/parseShinies/parseSimilarStrip/parseGameChannels/
// parseGlance all fail to match) still carry the web's inline
// `text-*` span colors and `<pre>` ASCII/braille art — plain htmlToText
// discards both, leaking literal tags or flattening art to gray runes.
// Shares walkTextCore with htmlToText (color=false there, true here) so
// every other structural behavior — block newlines, section-header
// markers, label/value grids, badges, svg alt text — stays identical;
// only bare `<span class="text-X">` runs additionally get painted, `<br>`
// becomes a newline (already true via blockTags), `<pre>` renders
// verbatim-with-color (the fixed preText), and x/net/html unescapes
// entities as it parses. Wired at render.go's default message-body branch
// and the confirmation outcome text.
func RenderMessageHTML(fragment string) string {
	nodes, err := html.ParseFragment(strings.NewReader(fragment), nil)
	if err != nil {
		return strings.TrimSpace(fragment)
	}
	var b strings.Builder
	for _, n := range nodes {
		walkTextCore(n, &b, true)
	}
	return collapseBlankLines(b.String())
}

// collapseBlankLines squashes runs of blank lines left behind by nested
// blocks down to one, and trims trailing whitespace off each line — shared
// by htmlToText and RenderMessageHTML so both flattening entry points
// leave identical blank-line rhythm.
func collapseBlankLines(s string) string {
	lines := strings.Split(s, "\n")
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
	// Token markers — platform chips and price coins survive the html
	// flattening as private-use runes; paintTokens styles them later.
	ChipStart = '\uE010'
	ChipEnd   = '\uE011'
	CoinMark  = '\uE012'
	StarMark  = '\uE013'
	// pito-token reference spans (2.0.0: painted cyan on the web).
	TokenStart = '\uE014'
	TokenEnd   = '\uE015'
	// Bold section headers (help groups, /config groups): the web paints
	// font-bold text-purple/text-yellow — the flattener marks them so
	// paintShimmer can restore the weight+color.
	HeaderStart = '\uE016'
	HeaderEnd   = '\uE017'
	// Shiny badges: E020 material E021 face-text E022; inner spaces ride
	// E023 so value wrapping never splits a badge.
	ShinyStart = '\uE020'
	ShinySep   = '\uE021'
	ShinyEnd   = '\uE022'
	ShinySpace = '\uE023'
)

// walkText is walkTextCore with color painting off — the shape every
// caller outside this function used before RenderMessageHTML existed
// (htmlToText, and the detail/glance/shinies/segments card parsers that
// call it directly for their own cell/fragment text). Keeping this exact
// two-argument signature means those other files — outside this task's
// edit list — never need to change.
func walkText(n *html.Node, b *strings.Builder) {
	walkTextCore(n, b, false)
}

// walkTextCore is walkText's shared implementation. color=true is
// RenderMessageHTML's message-body path: bare `<span class="text-X">`
// runs that don't match any of the more specific cases below (badges,
// help blocks, headers, tokens, shimmer, svg/img, grids) get painted via
// classStyle. color=false (every other caller, via the walkText wrapper
// above) never takes that branch, so htmlToText and its callers are
// byte-for-byte unchanged.
func walkTextCore(n *html.Node, b *strings.Builder, color bool) {
	if n.Type == html.TextNode {
		b.WriteString(strings.ReplaceAll(n.Data, "\n", " "))
	}
	if n.Type == html.ElementNode {
		switch {
		case strings.Contains(attr(n, "class"), "pito-shiny") &&
			!strings.Contains(attr(n, "class"), "shiny-rail"):
			// Achievement badge (pito G127): material + face text as
			// markers, plus — when present — a THIRD ShinySep segment
			// carrying the unlock date's raw web text ("Jun '26",
			// badge_component.rb's unlocked_on.strftime("%b '%y")).
			// Compact badges (detail card strips, form: :compact) never
			// render a __date child at all, so they fall straight through
			// to the two-segment payload exactly as before — no branching
			// on context needed here, the source HTML already encodes
			// compact vs extended by whether this span exists. tokens.go's
			// paintTokens prints the raw date text verbatim
			// (shinyDateSuffix) — badge_component.rb is the single source
			// of truth for its format (the current-year year-drop is
			// landing there too, in the parallel web change).
			b.WriteRune(ShinyStart)
			b.WriteString(attr(n, "data-material"))
			b.WriteRune(ShinySep)
			var face, date strings.Builder
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.ElementNode && strings.Contains(attr(child, "class"), "__date") {
					// Walk the date span's OWN children, not the span via
					// walkText(child, …) itself — its class
					// ("pito-shiny__date block") contains the substring
					// "pito-shiny", which would re-match this very case and
					// wrap the date text in a second, nested marker
					// sequence instead of plain text.
					for dchild := child.FirstChild; dchild != nil; dchild = dchild.NextSibling {
						walkTextCore(dchild, &date, color)
					}
					continue
				}
				walkTextCore(child, &face, color)
			}
			escapeSpaces := func(s string) string {
				// Inner spaces ride ShinySpace so wrapPlain's Fields-based
				// word wrap (detail.go) never splits a badge — the whole
				// ShinyStart…ShinyEnd run must stay ONE word.
				return strings.Map(func(ru rune) rune {
					if ru == ' ' {
						return ShinySpace
					}
					return ru
				}, strings.TrimSpace(s))
			}
			b.WriteString(escapeSpaces(face.String()))
			if d := escapeSpaces(date.String()); d != "" {
				b.WriteRune(ShinySep)
				b.WriteString(d)
			}
			b.WriteRune(ShinyEnd)
			return
		case strings.Contains(attr(n, "class"), "pito-help-block"):
			// Help blocks are pre-formatted (white-space: pre-wrap on the
			// web): their newlines ARE the layout — keep them verbatim.
			preText(n, b)
			return
		case strings.Contains(attr(n, "class"), "whitespace-pre-wrap"):
			// Free-text prose (video/channel descriptions, tags) sets the
			// same white-space: pre-wrap rule: the author's blank lines
			// ARE the paragraph breaks, not incidental source formatting.
			// The default TextNode case below collapses "\n" to a space
			// (correct for ordinary flow text) which glues every
			// paragraph into one wall — preText keeps them verbatim
			// instead, same contract as the help block above.
			preText(n, b)
			return
		case n.Data == "pre":
			// Bare <pre> blocks (ASCII/braille art) — same whitespace-
			// preserving contract as the two pre-wrap cases above; without
			// this case the default TextNode branch collapsed every
			// embedded newline to a space, destroying the art. preText's
			// element/span branch also colors any text-* spans riding
			// along inside, so painted art keeps its color here too.
			preText(n, b)
			return
		case strings.Contains(attr(n, "class"), "font-bold") &&
			(strings.Contains(attr(n, "class"), "text-purple") || strings.Contains(attr(n, "class"), "text-yellow")):
			// Section headers get a breath of air above them (the web's
			// section rhythm) and carry their bold color through markers.
			b.WriteString("\n\n")
			b.WriteRune(HeaderStart)
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walkTextCore(child, b, color)
			}
			b.WriteRune(HeaderEnd)
			if blockTags[n.Data] || n.Data == "div" {
				b.WriteString("\n")
			}
			return
		case strings.Contains(attr(n, "class"), "pito-token"):
			// Reference tokens ("7d", "@handle") — cyan on the web (2.0.0).
			b.WriteRune(TokenStart)
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walkTextCore(child, b, color)
			}
			b.WriteRune(TokenEnd)
			return
		case strings.Contains(attr(n, "class"), "pito-subject-shimmer"):
			b.WriteRune(ShimmerStart)
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walkTextCore(child, b, color)
			}
			b.WriteRune(ShimmerEnd)
			return
		case n.Data == "svg":
			// Icon SVGs carry their meaning in aria-label ("Likes",
			// "Comments") — emit the word, skip the path soup. Detach it
			// from a preceding count ("81<icon>" → "81 Likes").
			if label := attr(n, "aria-label"); label != "" {
				if s := b.String(); s != "" && !strings.HasSuffix(s, " ") && !strings.HasSuffix(s, "\n") {
					b.WriteString(" ")
				}
				b.WriteString(label)
			}
			return
		case n.Data == "img":
			// Images never render — no stand-in glyphs (owner lock).
			// Two DATA exceptions ride marker runes for later styling:
			// platform icons (alt = the platform) become chips, and the
			// Mario coins / FREE star become gold glyphs (owner call
			// 2026-07-06).
			class := attr(n, "class")
			switch {
			case strings.Contains(class, "pito-platform-icon"):
				b.WriteRune(ChipStart)
				b.WriteString(attr(n, "alt"))
				b.WriteRune(ChipEnd)
			case strings.Contains(class, "pito-coin--free"):
				b.WriteRune(StarMark)
			case strings.Contains(class, "pito-coin"):
				b.WriteRune(CoinMark)
			}
			return
		case strings.Contains(attr(n, "class"), "grid"):
			// Label/value grids (detail cards): the web separates the
			// span pairs visually; the terminal gets one pair per line.
			if pairs := gridPairs(n, color); pairs != "" {
				b.WriteString("\n" + pairs)
				return
			}
		case color && strings.Contains(attr(n, "class"), "text-"):
			// Message-body color mode only (RenderMessageHTML) — bare
			// `<span class="text-X">` runs that reached here unmatched by
			// every more specific case above keep their web color instead
			// of flattening to plain runes. color is always false for
			// walkText's callers (htmlToText and the table/kv/detail
			// parsers across this package), so this case never fires for
			// them — they're byte-for-byte unaffected.
			var inner strings.Builder
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walkTextCore(child, &inner, color)
			}
			b.WriteString(classStyle(attr(n, "class")).Render(inner.String()))
			if blockTags[n.Data] {
				b.WriteString("\n")
			}
			return
		case blockTags[n.Data]:
			b.WriteString("\n")
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		walkTextCore(child, b, color)
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

// preText extracts text preserving newlines (pre-formatted / pre-wrap
// blocks: pito-help-block's man pages, bare <pre> ASCII/braille art, and
// video/channel description and tags prose). Section-header spans — the
// web's "text-purple font-bold" / "text-yellow font-bold" — are painted
// bold via headerSpanStyle; every other span carrying a "text-*" class
// (cyan tokens, dim copy, orange/pito accents, art riding colored spans
// inside a <pre>) is painted plain (no bold) via classStyle directly, so
// colored art and inline accents keep their web color here too.
func preText(n *html.Node, b *strings.Builder) {
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
		return
	}
	if n.Type == html.ElementNode {
		class := attr(n, "class")
		if style, ok := headerSpanStyle(class); ok {
			var inner strings.Builder
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				preText(child, &inner)
			}
			b.WriteString(style.Render(inner.String()))
			return
		}
		if strings.Contains(class, "text-") {
			var inner strings.Builder
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				preText(child, &inner)
			}
			b.WriteString(classStyle(class).Render(inner.String()))
			return
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		preText(child, b)
	}
}

// headerSpanStyle recognizes a pito-help-block section-header span (the
// web's "text-purple font-bold" — server-side lib/pito/message_builder/
// man_page.rb's `header` helper; "text-yellow font-bold" covers any sibling
// payload that colors its headers the other reserved accent) and returns
// its paint style. classStyle (render.go) already maps the color from the
// class hint; only the font-bold → Bold(true) pairing is added here.
func headerSpanStyle(class string) (lipgloss.Style, bool) {
	if !strings.Contains(class, "font-bold") {
		return lipgloss.Style{}, false
	}
	if !strings.Contains(class, "text-purple") && !strings.Contains(class, "text-yellow") {
		return lipgloss.Style{}, false
	}
	return classStyle(class).Bold(true), true
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
// "" when the element does not look like a pair grid. color threads
// walkTextCore's message-body coloring into each cell (RenderMessageHTML
// only — htmlToText's callers pass color=false via walkText's own grid
// case, which is unaffected).
func gridPairs(n *html.Node, color bool) string {
	var cells []string
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			var cell strings.Builder
			walkTextCore(child, &cell, color)
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
