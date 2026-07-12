package render

import (
	"encoding/json"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"golang.org/x/net/html"
)

// The game enhanced segments — `show game … with similar/channels` (and
// the segment verbs). Both are strips of ScoreBar-carrying cards on the
// web; the terminal renders them as aligned rows, no images: identity
// comes from the id/title line (similar) or the avatar's alt handle
// (channel recommendations). The channel distribution redraws from the
// legend's own data — handle, share percent, color token per bar.

// cssAccent maps the web's CSS var color tokens onto terminal RGBs.
func cssAccent(token string) RGB {
	switch {
	case strings.Contains(token, "brand-pito"):
		return RGB{0x87, 0x5f, 0xff}
	case strings.Contains(token, "accent-green"):
		return heatGreen
	case strings.Contains(token, "accent-purple"):
		return RGB{0xaf, 0x5f, 0xff}
	case strings.Contains(token, "accent-cyan"):
		return RGB{0x5f, 0xd7, 0xff}
	case strings.Contains(token, "accent-yellow"):
		return heatYellow
	case strings.Contains(token, "accent-orange"):
		return scoreOrange
	case strings.Contains(token, "accent-red"):
		return scoreRed
	case strings.Contains(token, "accent-blue"):
		return RGB{0x5f, 0x87, 0xff}
	default:
		return RGB{0x9e, 0x9e, 0x9e}
	}
}

type scoredRow struct {
	label string
	score int
}

type similarStrip struct {
	intro string
	rows  []scoredRow
}

type shareRow struct {
	label string
	pct   float64
	color RGB
}

type gameChannels struct {
	intro       string
	shares      []shareRow
	distCaption string
	reco        []scoredRow
	recoCaption string
}

// parseSimilarStrip recognizes the similar-games strip: one row per
// recommended game — "#id Title" plus its similarity score.
func parseSimilarStrip(fragment string) (*similarStrip, bool) {
	if !strings.Contains(fragment, "similar-games-strip") {
		return nil, false
	}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), nil)
	if err != nil {
		return nil, false
	}
	strip := &similarStrip{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			class := attr(n, "class")
			switch {
			case strings.Contains(class, "__intro"):
				var b strings.Builder
				walkText(n, &b)
				strip.intro = strings.TrimSpace(b.String())
				return
			case strings.Contains(class, "similar-game-card"):
				row := scoredRow{score: -1}
				var id, title string
				var find func(*html.Node)
				find = func(m *html.Node) {
					if m.Type == html.ElementNode {
						mclass := attr(m, "class")
						switch {
						case strings.Contains(mclass, "similar-game-id"):
							id = stripShimmerMarkers(nodeText(m))
						case strings.Contains(mclass, "similar-game-title"):
							title = nodeText(m)
						case strings.Contains(mclass, "pito-score-bar"):
							if v, err := strconv.Atoi(attr(m, "data-score")); err == nil {
								row.score = v
							}
							return
						}
					}
					for child := m.FirstChild; child != nil; child = child.NextSibling {
						find(child)
					}
				}
				find(n)
				row.label = strings.TrimSpace(id + " " + title)
				if row.label != "" {
					strip.rows = append(strip.rows, row)
				}
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
	if len(strip.rows) == 0 {
		return nil, false
	}
	return strip, true
}

// parseGameChannels recognizes the channels overview: distribution
// legend (the coverage data) + recommendation score rows.
func parseGameChannels(fragment string) (*gameChannels, bool) {
	if !strings.Contains(fragment, "pito-game-channels") {
		return nil, false
	}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), nil)
	if err != nil {
		return nil, false
	}
	gc := &gameChannels{}
	captions := []string{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			class := attr(n, "class")
			switch {
			case strings.Contains(class, "__intro"):
				var b strings.Builder
				walkText(n, &b)
				gc.intro = strings.TrimSpace(b.String())
				return
			case strings.Contains(class, "blegend-item"):
				text := strings.TrimSpace(strings.TrimPrefix(nodeText(n), "●"))
				fields := strings.Fields(text)
				if len(fields) < 2 {
					return
				}
				pctText := strings.TrimSuffix(fields[len(fields)-1], "%")
				pct, err := strconv.ParseFloat(pctText, 64)
				if err != nil {
					return
				}
				gc.shares = append(gc.shares, shareRow{
					label: strings.Join(fields[:len(fields)-1], " "),
					pct:   pct,
					color: cssAccent(attr(n, "style")),
				})
				return
			case strings.Contains(class, "reco-row"):
				row := scoredRow{score: -1}
				var find func(*html.Node)
				find = func(m *html.Node) {
					if m.Type == html.ElementNode {
						mclass := attr(m, "class")
						if m.Data == "img" && strings.Contains(mclass, "reco-avatar-img") {
							row.label = attr(m, "alt")
							return
						}
						if strings.Contains(mclass, "pito-score-bar") {
							if v, err := strconv.Atoi(attr(m, "data-score")); err == nil {
								row.score = v
							}
							return
						}
					}
					for child := m.FirstChild; child != nil; child = child.NextSibling {
						find(child)
					}
				}
				find(n)
				if row.label != "" {
					gc.reco = append(gc.reco, row)
				}
				return
			case strings.Contains(class, "__caption"):
				captions = append(captions, nodeText(n))
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
	if len(gc.shares) == 0 && len(gc.reco) == 0 {
		return nil, false
	}
	// Captions arrive in DOM order: distribution column, then reco.
	if len(captions) > 0 {
		gc.distCaption = captions[0]
	}
	if len(captions) > 1 {
		gc.recoCaption = captions[1]
	}
	return gc, true
}

// scoredRows renders "label [====score|==]" rows with one shared label
// width so every bracket aligns — the terminal's card strip. The bars
// cap at scoreBarCap cells INSIDE the column (owner rule), same as
// detail.go's show-card bars.
func (r *R) scoredRows(rows []scoredRow, width int) string {
	if width > scoreBarCap {
		width = scoreBarCap
	}
	labelWidth := 0
	for _, row := range rows {
		if w := lipgloss.Width(row.label); w > labelWidth {
			labelWidth = w
		}
	}
	var out []string
	for _, row := range rows {
		pad := strings.Repeat(" ", labelWidth-lipgloss.Width(row.label))
		out = append(out, r.ScoreBar(row.label+pad+" ", row.score, width))
	}
	return strings.Join(out, "\n")
}

// shareBars renders the distribution as braille bars — the web Bar
// visualizer's own glyph language: ⣿ (U+28FF) runs in the channel's
// color, the empty remainder the SAME glyph dimmed (the web's
// is-outline at 0.45 opacity), one row per channel with its percent.
func (r *R) shareBars(shares []shareRow, width int) string {
	labelWidth := 0
	for _, s := range shares {
		if w := lipgloss.Width(s.label); w > labelWidth {
			labelWidth = w
		}
	}
	barWidth := width - labelWidth - 9
	if barWidth < 10 {
		barWidth = 10
	}
	if barWidth > 40 {
		barWidth = 40
	}
	var out []string
	for _, s := range shares {
		filled := int(s.pct/100*float64(barWidth) + 0.5)
		if filled > barWidth {
			filled = barWidth
		}
		outlineStyle := lipgloss.NewStyle().Foreground(ColorFaint)
		if r.truecolor {
			outlineStyle = outlineStyle.Foreground(hex(dimRGB(s.color)))
		}
		bar := ""
		for i := 0; i < filled; i++ {
			st := lipgloss.NewStyle()
			if r.truecolor {
				// The pito-blue band sweeps the fill, web-style.
				st = st.Foreground(hex(bandBoost(s.color, i, barWidth, r.staggered(s.label))))
			}
			bar += st.Render("⣿")
		}
		if filled < barWidth {
			bar += outlineStyle.Render(strings.Repeat("⣿", barWidth-filled))
		}
		pad := strings.Repeat(" ", labelWidth-lipgloss.Width(s.label))
		pct := strconv.FormatFloat(s.pct, 'f', 1, 64) + "%"
		out = append(out, r.dim(s.label+pad+" ")+bar+r.dim(" "+pct))
	}
	return strings.Join(out, "\n")
}

// dimRGB is the terminal's is-outline: the same color at ~40% intensity.
func dimRGB(c RGB) RGB {
	return RGB{uint8(float64(c.R) * 0.4), uint8(float64(c.G) * 0.4), uint8(float64(c.B) * 0.4)}
}

// similarStrip renders the recommendations: intro, then one scored row
// per similar game.
func (r *R) similarStrip(s *similarStrip) string {
	width := r.width - 3
	parts := []string{}
	if s.intro != "" {
		parts = append(parts, r.paintShimmer(s.intro))
	}
	parts = append(parts, r.scoredRows(s.rows, width))
	return strings.Join(parts, "\n\n")
}

// gameChannels renders the channels overview: intro, coverage bars with
// their caption, then the recommendation rows with theirs.
func (r *R) gameChannels(g *gameChannels) string {
	width := r.width - 3
	parts := []string{}
	if g.intro != "" {
		parts = append(parts, r.paintShimmer(g.intro))
	}
	if len(g.shares) > 0 {
		block := r.shareBars(g.shares, width)
		if g.distCaption != "" {
			block += "\n" + r.dim(g.distCaption)
		}
		parts = append(parts, block)
	}
	if len(g.reco) > 0 {
		block := r.scoredRows(g.reco, width)
		if g.recoCaption != "" {
			block += "\n" + r.dim(g.recoCaption)
		}
		parts = append(parts, block)
	}
	return strings.Join(parts, "\n\n")
}

// hasPendingChannels reports a channels-overview payload whose async
// distribution fill has not landed yet.
func hasPendingChannels(payload []byte) bool {
	var p struct {
		ChannelDistribution struct {
			Status string `json:"status"`
		} `json:"channel_distribution"`
	}
	return json.Unmarshal(payload, &p) == nil && p.ChannelDistribution.Status == "pending"
}

// channelsIntro is the pending state's headline, straight from the
// payload's own copy.
func channelsIntro(payload []byte) string {
	var p struct {
		ChannelDistribution struct {
			Intro string `json:"intro"`
		} `json:"channel_distribution"`
	}
	if json.Unmarshal(payload, &p) == nil && p.ChannelDistribution.Intro != "" {
		return htmlToText(p.ChannelDistribution.Intro)
	}
	// COPY LAW: no server intro → no intro.
	return ""
}
