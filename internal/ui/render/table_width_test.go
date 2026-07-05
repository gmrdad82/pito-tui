package render

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// A live-captured `ls games with platform, price, views` payload wide
// enough to overflow most terminals. At EVERY width the table must keep
// one line per row inside the message bar — width 146 once wrapped each
// row's last char onto a zebra-painted stub line (owner capture
// 2026-07-05). The frame is 3 dash rules (top, header, bottom); the body
// sits between the header rule and the bottom rule, one line per row.
func TestWideTableKeepsOneLinePerRowAtAnyWidth(t *testing.T) {
	withTrueColor(t)
	raw, err := os.ReadFile("testdata/list_games_wide.json")
	if err != nil {
		t.Fatal(err)
	}
	for width := 40; width <= 220; width++ {
		out := New(width, WithTruecolor(true)).Event(event("system", string(raw)))
		var ruleLines []int
		over := 0
		lines := strings.Split(out, "\n")
		for i, line := range lines {
			if strings.Count(line, "─") > 10 {
				ruleLines = append(ruleLines, i)
			}
			if lipgloss.Width(line) > width {
				over++
			}
		}
		if len(ruleLines) != 3 || over > 0 {
			t.Errorf("width %d: rules=%d (want 3) over-wide lines=%d", width, len(ruleLines), over)
			continue
		}
		if body := ruleLines[2] - ruleLines[1] - 1; body != 12 {
			t.Errorf("width %d: %d body lines between header and bottom rule (want 12 rows)", width, body)
		}
	}
}
