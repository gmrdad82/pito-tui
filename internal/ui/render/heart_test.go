package render

import (
	"strings"
	"testing"
)

func TestHeartZeroScoreIsOutlineOnly(t *testing.T) {
	grid := heartGrid(0)
	for cr, row := range grid {
		for cc, c := range row {
			if c.state == heartFilled {
				t.Fatalf("row %d col %d: 0-score heart must never fill: %+v", cr, cc, c)
			}
		}
	}
	// The heart body (rows 2..11) must still show its rim — a hollow
	// heart, not a blank canvas.
	sawOutline := false
	for cr := 2; cr <= 11; cr++ {
		_, solid := heartRowGlyphs(grid[cr])
		if solid {
			t.Errorf("row %d: pure-rim row at score 0 must dim, not solid", cr)
		}
		for _, c := range grid[cr] {
			if c.state == heartOutline {
				sawOutline = true
			}
		}
	}
	if !sawOutline {
		t.Error("0-score heart drew no outline at all")
	}
}

func TestHeartHundredScoreIsFullyFilled(t *testing.T) {
	grid := heartGrid(100)
	for cr, row := range grid {
		for cc, c := range row {
			if c.state == heartOutline || c.state == heartInterior {
				t.Fatalf("row %d col %d: 100-score heart must never leave a hollow cell: %+v", cr, cc, c)
			}
		}
		if _, solid := heartRowGlyphs(row); !solid {
			t.Errorf("row %d: 100-score heart must never dim a row", cr)
		}
	}
	// Sanity: the heart body actually drew something (not an empty mask).
	sawFilled := false
	for _, row := range grid {
		for _, c := range row {
			if c.state == heartFilled {
				sawFilled = true
			}
		}
	}
	if !sawFilled {
		t.Error("100-score heart drew no filled cells at all")
	}
}

func TestHeartFiftyFillsTheBottomHalf(t *testing.T) {
	// 17×17 cols / 13 rows / margins 2-top,1-bottom (pito's constants):
	// the heart body spans cell rows 2..11 (10 rows). At 50% the
	// waterline bisects it exactly — rows 7..11 (the bottom half of the
	// body) read solid/filled, rows 2..6 (the top half) read dimmed
	// rim-only, and the margin rows (0,1,12) carry no heart cell at all.
	grid := heartGrid(50)

	for _, cr := range []int{7, 8, 9, 10, 11} {
		_, solid := heartRowGlyphs(grid[cr])
		if !solid {
			t.Errorf("row %d: bottom-half row must be solid at score 50", cr)
		}
		filled := false
		for _, c := range grid[cr] {
			if c.state == heartFilled {
				filled = true
			}
		}
		if !filled {
			t.Errorf("row %d: bottom-half row must contain filled cells at score 50", cr)
		}
	}

	for _, cr := range []int{2, 3, 4, 5, 6} {
		_, solid := heartRowGlyphs(grid[cr])
		if solid {
			t.Errorf("row %d: top-half row must dim (rim only) at score 50", cr)
		}
		for cc, c := range grid[cr] {
			if c.state == heartFilled {
				t.Errorf("row %d col %d: top-half row must not fill at score 50", cr, cc)
			}
		}
	}

	for _, cr := range []int{0, 1, 12} {
		for cc, c := range grid[cr] {
			if c.state != heartOutside {
				t.Errorf("row %d col %d: margin row must carry no heart cell, got %+v", cr, cc, c)
			}
		}
	}
}

func TestHeartCanvasOnlyBrailleAndSpaces(t *testing.T) {
	out := stripANSI(plain().heartCanvas(63.5, RGB{0xff, 0x5f, 0x87}, 42, 5, "89.36%"))
	lines := strings.Split(out, "\n")
	if len(lines) < heartRows {
		t.Fatalf("heartCanvas returned %d lines, want at least %d canvas rows + legend:\n%s", len(lines), heartRows, out)
	}
	for i := 0; i < heartRows; i++ {
		for _, ru := range lines[i] {
			if ru == ' ' {
				continue
			}
			if ru < 0x2800 || ru > 0x28FF {
				t.Errorf("canvas row %d carries a non-braille, non-space rune %U: %q", i, ru, lines[i])
			}
		}
	}
}

func TestHeartCanvasLegendLine(t *testing.T) {
	out := plain().heartCanvas(88.8, RGB{0x5f, 0xd7, 0x87}, 14453, 1806, "88.80%")
	legend := strings.Split(out, "\n")[heartRows]
	for _, want := range []string{"14453 Likes", "1806 Dislikes", "(88.80%)"} {
		if !strings.Contains(legend, want) {
			t.Errorf("legend %q missing %q", legend, want)
		}
	}
}

func TestHeartCanvasRowCountMatchesTheCanvas(t *testing.T) {
	out := stripANSI(plain().heartCanvas(20, RGB{0xd7, 0x5f, 0x5f}, 1, 1, "50.00%"))
	lines := strings.Split(out, "\n")
	if len(lines) != heartRows+1 { // canvas rows + one legend line
		t.Errorf("heartCanvas produced %d lines, want %d (canvas) + 1 (legend):\n%s", len(lines), heartRows, out)
	}
	for i := 0; i < heartRows; i++ {
		if n := len([]rune(lines[i])); n != heartCols {
			t.Errorf("row %d has %d cells, want %d: %q", i, n, heartCols, lines[i])
		}
	}
}
