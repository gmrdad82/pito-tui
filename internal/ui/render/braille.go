package render

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The Go port of pito's Pito::Analytics::BrailleAreaChart — dot-exact.
// Each braille cell packs a 2×4 dot grid; the area under the curve fills
// bottom-up from a one-dot baseline so zeros read as a floor, never a
// gap, and any strictly-positive value clears the floor by at least one
// dot. Pure: series in, braille rows out; the caller owns color.

const brailleBlank = 0x2800

// brailleDot[localCol][localRow] — dots 1,2,3,7 left, 4,5,6,8 right.
var brailleDot = [2][4]int{
	{0x01, 0x02, 0x04, 0x40},
	{0x08, 0x10, 0x20, 0x80},
}

// BrailleArea renders series as `rows` braille strings (top→bottom),
// each `cols` cells wide. max fixes the y ceiling (pass the series peak
// or max(peak, target)); <=0 falls back to the peak.
func BrailleArea(series []float64, cols, rows int, max float64) []string {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	dotW, dotH := cols*2, rows*4
	ceiling := max
	if ceiling <= 0 {
		for _, v := range series {
			if v > ceiling {
				ceiling = v
			}
		}
	}
	heights := columnHeights(series, ceiling, dotW, dotH)

	out := make([]string, rows)
	for cellRow := 0; cellRow < rows; cellRow++ {
		var b strings.Builder
		for cellCol := 0; cellCol < cols; cellCol++ {
			mask := 0
			for lc := 0; lc < 2; lc++ {
				h := heights[cellCol*2+lc]
				if h <= 0 {
					continue
				}
				for lr := 0; lr < 4; lr++ {
					if y := cellRow*4 + lr; y >= dotH-h {
						mask |= brailleDot[lc][lr]
					}
				}
			}
			b.WriteRune(rune(brailleBlank + mask))
		}
		out[cellRow] = b.String()
	}
	return out
}

// columnHeights mirrors the Ruby: baseline floor, per-dot-column lerp
// sampling, positive values always at least one dot above the floor.
func columnHeights(values []float64, ceiling float64, dotW, dotH int) []int {
	floor := 1
	if floor > dotH {
		floor = dotH
	}
	heights := make([]int, dotW)
	if ceiling <= 0 || len(values) == 0 {
		for i := range heights {
			heights[i] = floor
		}
		return heights
	}
	for x := 0; x < dotW; x++ {
		v := sampleAt(values, x, dotW)
		h := int(math.Round(v / ceiling * float64(dotH)))
		if h < 0 {
			h = 0
		}
		if h > dotH {
			h = dotH
		}
		if h < floor {
			h = floor
		}
		if v > 0 && h <= floor {
			h = floor + 1
			if h > dotH {
				h = dotH
			}
		}
		heights[x] = h
	}
	return heights
}

func sampleAt(values []float64, x, dotW int) float64 {
	if len(values) == 1 {
		return values[0]
	}
	pos := 0.0
	if dotW > 1 {
		pos = float64(x) * float64(len(values)-1) / float64(dotW-1)
	}
	lo, hi := int(math.Floor(pos)), int(math.Ceil(pos))
	if lo == hi {
		return values[lo]
	}
	frac := pos - float64(lo)
	return values[lo]*(1-frac) + values[hi]*frac
}

// paintBraille styles chart rows the shared way: curve runes in the
// default fg with the pito-blue band sweeping over them, blanks showing
// the dotted paper grid (⠂ dots, ⣀ baseline), all dim when noData.
func (r *R) paintBraille(rows []string, cellW int, noData bool) []string {
	fgDefault := RGB{0xda, 0xda, 0xda}
	chartPhase := staggered(r.phase, strings.Join(rows, ""))
	var lines []string
	for ri, row := range rows {
		// Bottom-up grow-in (the web's area-chart reveal): the bottom row
		// surfaces first, upper rows as the fraction climbs.
		rowVisible := r.revealFrac >= float64(len(rows)-1-ri)/float64(len(rows))
		runes := []rune(row)
		if len(runes) > cellW {
			runes = runes[:cellW]
		}
		bg := '⠂'
		if ri == len(rows)-1 {
			bg = '⣀'
		}
		var b strings.Builder
		for i := 0; i < cellW; i++ {
			ru := bg
			blank := true
			if rowVisible && i < len(runes) && runes[i] != '⠀' && runes[i] != ' ' {
				ru, blank = runes[i], false
			}
			switch {
			case blank || noData:
				b.WriteString(lipgloss.NewStyle().Foreground(ColorFaint).Render(string(ru)))
			case r.truecolor:
				b.WriteString(lipgloss.NewStyle().Foreground(hex(bandBoostRow(fgDefault, i, ri, cellW, chartPhase))).Render(string(ru)))
			default:
				b.WriteString(string(ru))
			}
		}
		lines = append(lines, b.String())
	}
	return lines
}
