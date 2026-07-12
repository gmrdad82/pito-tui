package ui

import (
	"strings"
	"testing"
	"time"
)

// TestPickerSelectionUsesElevatedGrayNotZebra pins the retint (owner
// 2026-07-12 "align to Charm" table restyle): the picker's cursor-row
// highlight moved off the table zebra's old plum (#1B142B, ColorZebra)
// onto the neutral ColorElevated (#2A2E3A). Unlike the table zebra this is
// a SELECTION affordance, not decoration, so it keeps a background — just
// a different one, so the whole app leaves the plum family together.
func TestPickerSelectionUsesElevatedGrayNotZebra(t *testing.T) {
	rows := []pickerRow{
		{isNew: true, title: "start a new conversation"},
		{uuid: "abc", title: "second row", section: "recent"},
	}
	out := pickerView(rows, 1, 60, 20, time.Now(), true, 0, false, false)
	if strings.Contains(out, "48;2;27;20;43") { // #1B142B, the old ColorZebra
		t.Errorf("picker selection must not use the old plum ColorZebra:\n%q", out)
	}
	if !strings.Contains(out, "48;2;42;46;58") { // #2A2E3A, ColorElevated
		t.Errorf("picker selection must use the new ColorElevated:\n%q", out)
	}
}
