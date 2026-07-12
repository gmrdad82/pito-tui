// applyShake is the error-shake's line offsetter — the reveal cascade
// that once shared this file left with the spring purge (owner
// 2026-07-12).
package render

import "strings"

// applyShake prepends max(0, offset) spaces to every non-blank line of
// block — the ambassador wave's error-shake jitter (micro.go effect 1).
// offset <= 0 (no entry in r.shake for this event, a settled event, or
// one of errorShakeOffsets' own resting/overcorrect ticks) is a no-op
// returning block untouched: a block can only be pushed right of its
// resting column, never left of it, so the sequence's "-1" tick reads as
// "back to rest," not an actual leftward shift. Blank lines (the
// trailing empty element strings.Split leaves after the block's own
// final "\n", and any intentional blank spacer inside it) are left
// alone — nothing there to shift, and padding one would only add
// invisible trailing whitespace. Keyed per event at the call site
// (Event's own r.shakeFor(ev.ID) lookup), so this NEVER touches any
// other event's block — only the one currently shaking gets an offset
// above 0 handed in.
func applyShake(block string, offset int) string {
	if offset <= 0 {
		return block
	}
	pad := strings.Repeat(" ", offset)
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}
