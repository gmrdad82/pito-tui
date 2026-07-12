package render

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// The Go port of pito's Pito::Analytics::BrailleHeart (app/services) +
// Pito::Analytics::Visualizers::Heart (app/components) — dot-exact. One
// heart is a 17×13 braille CELL canvas (34×52 dots): the classic
// implicit heart curve (x²+y²−1)³ − x²·y³ ≤ 0, sampled per dot, inset by
// a 2-cell-row top margin / 1-cell-row bottom margin so the heart floats
// off the canvas edges (the heart itself is 10 cell rows tall). It fills
// bottom→top proportionally to a 0..100 score; above the waterline only
// the RIM (boundary) cells survive — a hollow heart — while pure
// interior cells above the waterline blank out, exactly like the web's
// is-outline empty-treatment. Reuses braille.go's brailleDot bit table
// and brailleBlank rune (same 2×4 dot packing — no second copy needed).

// Heart canvas dims (CELLS) — pito's HEART_COLS/HEART_ROWS/margins
// (Heart::HEART_COLS = 17, HEART_ROWS = 13, HEART_TOP_MARGIN = 2,
// HEART_BOTTOM_MARGIN = 1), transcribed exactly. This is the single
// heart's canvas; a caller composing 1–2 hearts side by side (the web's
// GAP_CELLS = 3 gutter) owns that layout, not this file.
const (
	heartCols         = 17
	heartRows         = 13
	heartTopMargin    = 2
	heartBottomMargin = 1
)

// heartCellState mirrors BrailleHeart's four per-cell states
// (:filled/:outline/:interior/:outside).
type heartCellState int

const (
	heartOutside heartCellState = iota
	heartInterior
	heartOutline
	heartFilled
)

type heartCellDot struct {
	char  rune
	state heartCellState
}

// heartGrid renders score (0..100, clamped) into heartRows rows of
// heartCols cells, top→bottom — the direct port of BrailleHeart.call.
func heartGrid(score float64) [][]heartCellDot {
	pct := score
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	dotW, dotH := heartCols*2, heartRows*4
	// The heart occupies a SUB-region of the dot canvas — heart_top down
	// to heart_bottom, per pito's top/bottom margin constants.
	top := heartTopMargin * 4
	bottom := dotH - heartBottomMargin*4
	h := bottom - top
	mask := heartMask(dotW, dotH, top, h)
	// Fill is HEART-RELATIVE (no axis): 0% = the cusp, 100% = the lobe
	// tops. The waterline sweeps the heart's own vertical extent only —
	// margins never count toward the fill.
	waterline := float64(bottom) - (pct/100.0)*float64(h)

	grid := make([][]heartCellDot, heartRows)
	for cr := 0; cr < heartRows; cr++ {
		row := make([]heartCellDot, heartCols)
		for cc := 0; cc < heartCols; cc++ {
			row[cc] = heartCellAt(mask, cr, cc, waterline)
		}
		grid[cr] = row
	}
	return grid
}

// heartMask is a boolean dot mask of the heart curve over the full
// dot_w × dot_h canvas, confined to the region [top, top+h) — rows
// outside it (the margins) are forced blank. Within the region the
// coord space spans the heart's full range: x ∈ [-1.28, 1.28]; y runs
// from ≈1.15 (the lobe tops, x=0 lands outside → the cleft) down to
// ≈-1.05 (the cusp) — pito's braille_heart.rb#heart_mask constants,
// transcribed dot-for-dot.
func heartMask(dotW, dotH, top, h int) [][]bool {
	mask := make([][]bool, dotH)
	for dy := 0; dy < dotH; dy++ {
		row := make([]bool, dotW)
		if dy >= top && dy < top+h {
			for dx := 0; dx < dotW; dx++ {
				x := ((float64(dx) + 0.5) - float64(dotW)/2.0) / (float64(dotW) / 2.0) * 1.28
				y := 1.15 - (float64(dy)+0.5-float64(top))/float64(h)*2.2
				a := x*x + y*y - 1.0
				row[dx] = (a*a*a)-(x*x*(y*y*y)) <= 0.0
			}
		}
		mask[dy] = row
	}
	return mask
}

// heartCellAt renders one braille cell: the glyph from its in-heart dots
// + its state vs the waterline (fillTop) — BrailleHeart#cell.
func heartCellAt(mask [][]bool, cellRow, cellCol int, fillTop float64) heartCellDot {
	baseR, baseC := cellRow*4, cellCol*2
	bits, inHeart, filledDots := 0, 0, 0
	for lr := 0; lr < 4; lr++ {
		for lc := 0; lc < 2; lc++ {
			if !mask[baseR+lr][baseC+lc] {
				continue
			}
			bits |= brailleDot[lc][lr]
			inHeart++
			if float64(baseR+lr) >= fillTop {
				filledDots++
			}
		}
	}
	if inHeart == 0 {
		return heartCellDot{char: rune(brailleBlank), state: heartOutside}
	}
	char := rune(brailleBlank | bits)
	if filledDots > 0 { // at/below the waterline
		return heartCellDot{char: char, state: heartFilled}
	}
	if heartBoundary(mask, baseR, baseC) {
		return heartCellDot{char: char, state: heartOutline}
	}
	return heartCellDot{char: char, state: heartInterior}
}

// heartBoundary reports whether a cell sits on the heart's edge: one of
// its in-heart dots has a 4-neighbour off-heart (or off-canvas) — used
// to draw the hollow outline. BrailleHeart#boundary_cell?.
func heartBoundary(mask [][]bool, baseR, baseC int) bool {
	dotH, dotW := len(mask), len(mask[0])
	for lr := 0; lr < 4; lr++ {
		for lc := 0; lc < 2; lc++ {
			r, c := baseR+lr, baseC+lc
			if !mask[r][c] {
				continue
			}
			neighbors := [4][2]int{{r - 1, c}, {r + 1, c}, {r, c - 1}, {r, c + 1}}
			for _, n := range neighbors {
				nr, nc := n[0], n[1]
				if nr < 0 || nc < 0 || nr >= dotH || nc >= dotW || !mask[nr][nc] {
					return true
				}
			}
		}
	}
	return false
}

// heartRowGlyphs mirrors Heart#render_row: a row's glyph string (blanks
// for hollow interior/outside cells, keeping every column's width) plus
// whether the row stays SOLID (has any filled cell, or has no rim cell
// at all — a blank margin row) vs DIMMED (a pure-rim row entirely above
// the waterline). Dimming is a WHOLE-ROW decision, never a single cell —
// so a 100% heart's top rim row rides the same solid color as its
// filled interior, never dims.
func heartRowGlyphs(row []heartCellDot) (glyphs string, solid bool) {
	hasFilled, hasOutline := false, false
	var b strings.Builder
	for _, c := range row {
		switch c.state {
		case heartFilled:
			hasFilled = true
			b.WriteRune(c.char)
		case heartOutline:
			hasOutline = true
			b.WriteRune(c.char)
		default: // interior/outside: hollow — blank keeps the column width
			b.WriteRune(rune(brailleBlank))
		}
	}
	return b.String(), hasFilled || !hasOutline
}

// heartCanvas renders one braille heart (17×13 cells) filled bottom→top
// to score (0..100), plus a likes/dislikes legend line below it — the
// terminal cousin of pito's Visualizers::Heart, one heart's worth (a
// caller composing 1–2 hearts side by side owns that layout). Filled
// cells + the legend ride tint; the hollow rim above the waterline dims
// to ColorFaint, off-truecolor terminals keep the default fg instead of
// tint. Legend follows the web's likes/dislikes/pct structure in words,
// aria-style (no emoji): "<likes> Likes / <dislikes> Dislikes (<pct>)".
func (r *R) heartCanvas(score float64, tint RGB, likes, dislikes int, pct string) string {
	grid := heartGrid(score)
	lines := make([]string, len(grid))
	for i, row := range grid {
		glyphs, solid := heartRowGlyphs(row)
		style := lipgloss.NewStyle().Foreground(ColorFaint)
		if solid {
			style = lipgloss.NewStyle()
			if r.truecolor {
				style = style.Foreground(hex(tint))
			}
		}
		lines[i] = style.Render(glyphs)
	}
	legendStyle := lipgloss.NewStyle()
	if r.truecolor {
		legendStyle = legendStyle.Foreground(hex(tint))
	}
	legend := legendStyle.Render(fmt.Sprintf("%d Likes / %d Dislikes (%s)", likes, dislikes, pct))
	return strings.Join(lines, "\n") + "\n" + legend
}
