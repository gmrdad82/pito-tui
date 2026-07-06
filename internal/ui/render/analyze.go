package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The analyze payload (live-captured 2026-07-05): an `analyze` key with
// per-metric entries under `stash`, each carrying a time series and a
// Butler caption. The web draws these with its visualizer components;
// the terminal gets sparkline runes, deltas, target pacing, and hearts.

type analyzePayload struct {
	Analyze *analyzeData `json:"analyze"`
}

type analyzeData struct {
	Intro string                    `json:"intro"`
	Level string                    `json:"level"`
	Stash map[string]analyzeMetric  `json:"stash"`
	Likes *heartMetric              `json:"likes"`
	Meta  map[string]analyzeCaption `json:"-"`
}

type analyzeMetric struct {
	// Data stays raw: slots differ per metric (charts carry a series
	// object, the likes slot carries a bare list) and one alien entry
	// must not poison the whole stash decode.
	Data json.RawMessage `json:"data"`
	Slot string          `json:"slot"`
}

type analyzeSeries struct {
	Dates       []string  `json:"dates"`
	Series      []float64 `json:"series"`
	Total       float64   `json:"total"`
	TotalPct    *float64  `json:"total_pct"`
	Previous    *float64  `json:"previous"`
	TargetDaily float64   `json:"target_daily"`
}

type analyzeCaption struct {
	Caption string `json:"caption"`
}

type heartMetric struct {
	Caption string `json:"caption"`
	Hearts  []struct {
		Color    string  `json:"color"`
		Score    float64 `json:"score"`
		Likes    int64   `json:"likes"`
		Dislikes int64   `json:"dislikes"`
	} `json:"hearts"`
}

// analyzeBlock renders the charts for an analyze payload; "" when the
// payload carries none (callers fall through to plain body rendering).
func (r *R) analyzeBlock(payload []byte) string {
	var p analyzePayload
	if json.Unmarshal(payload, &p) != nil || p.Analyze == nil {
		return ""
	}
	a := p.Analyze

	// Captions live beside the stash under the metric's own top-level key.
	var captions map[string]analyzeCaption
	_ = json.Unmarshal(func() []byte {
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(payload, &raw)
		return raw["analyze"]
	}(), &captions)

	// Stable metric order: the web's chart order, then anything new.
	order := []string{"subs", "views", "watched_hours", "avg_viewed_pct", "avg_view_duration", "ctr"}
	seen := map[string]bool{}
	var names []string
	for _, name := range order {
		if _, ok := a.Stash[name]; ok {
			names = append(names, name)
			seen[name] = true
		}
	}
	var rest []string
	for name := range a.Stash {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	names = append(names, rest...)

	var b strings.Builder
	for _, name := range names {
		m := a.Stash[name]
		if m.Slot != "charts" {
			continue // hearts ride the top-level likes key
		}
		var data analyzeSeries
		if json.Unmarshal(m.Data, &data) != nil || len(data.Series) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		if c, ok := captions[name]; ok && c.Caption != "" {
			b.WriteString(r.paintShimmer(htmlToText(c.Caption)) + "\n")
		} else {
			b.WriteString(strings.ReplaceAll(name, "_", " ") + "\n")
		}
		b.WriteString(r.spark(data) + "\n")
	}

	if a.Likes != nil && len(a.Likes.Hearts) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		if a.Likes.Caption != "" {
			b.WriteString(r.paintShimmer(htmlToText(a.Likes.Caption)) + "\n")
		}
		for _, h := range a.Likes.Hearts {
			heart := fmt.Sprintf("♥ %.1f%%", h.Score)
			if r.truecolor {
				heart = MeterRamp.Colorize(heart, 1-h.Score/100+r.phase) // loved = green, breathing with the sweep
			} else {
				heart = lipgloss.NewStyle().Foreground(ColorAccent).Render(heart)
			}
			b.WriteString(fmt.Sprintf("  %s  %s", heart,
				r.dim(fmt.Sprintf("%d likes · %d dislikes", h.Likes, h.Dislikes))))
		}
	}
	return b.String()
}

// spark renders one metric: the web's 2-row braille curve (BrailleArea,
// the BrailleAreaChart port — braille, never solid runes) with the
// scalar legend below: total, delta vs the previous window, target
// pacing. The ceiling keeps the daily target on-screen, web-style.
func (r *R) spark(s analyzeSeries) string {
	cellW := 42
	if w := r.width - 3; w < cellW {
		cellW = w
	}
	ceiling := s.TargetDaily
	for _, v := range s.Series {
		if v > ceiling {
			ceiling = v
		}
	}
	rows := BrailleArea(s.Series, cellW, 2, ceiling)
	lines := r.paintBraille(rows, cellW, false)
	total := trimFloat(s.Total)
	if s.TotalPct != nil {
		total = trimFloat(*s.TotalPct) + "%"
	}
	meta := "total " + total
	if s.Previous != nil {
		meta += " · prev " + trimFloat(*s.Previous)
	}
	if s.TargetDaily > 0 {
		meta += " · target " + trimFloat(s.TargetDaily) + "/d"
	}
	return strings.Join(lines, "\n") + "\n" + r.dim(meta)
}

func trimFloat(v float64) string {
	out := fmt.Sprintf("%.2f", v)
	out = strings.TrimRight(strings.TrimRight(out, "0"), ".")
	if out == "" || out == "-" {
		out = "0"
	}
	return out
}

// hasAnalyze reports whether the payload is an analyze message at all —
// including the pending state before any metric data lands.
func hasAnalyze(payload []byte) bool {
	var p analyzePayload
	return json.Unmarshal(payload, &p) == nil && p.Analyze != nil
}

// analyzeIntro pulls the intro line out of a chart payload.
func analyzeIntro(payload []byte) string {
	var p analyzePayload
	if json.Unmarshal(payload, &p) != nil || p.Analyze == nil {
		return ""
	}
	return p.Analyze.Intro
}
