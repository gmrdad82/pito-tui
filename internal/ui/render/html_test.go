package render

import (
	"fmt"
	"strings"
	"testing"
)

// TestRenderMessageHTML covers RenderMessageHTML's contract (html.go doc
// comment): the color-preserving sibling of htmlToText used for message
// bodies that never earned a structured card. TestHTMLToText above pins
// the plain-flattening path; this pins the three things that path does
// NOT do — paint text-* spans, keep <pre> whitespace verbatim, and treat
// <br> as a newline — plus the baseline passthrough/entity/strip behavior
// both flatteners share.
func TestRenderMessageHTML(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"plain passthrough":  {"just plain text, nothing fancy", "just plain text, nothing fancy"},
		"br becomes newline": {"line one<br>line two", "line one\nline two"},
		"entities unescape":  {"fish &amp; chips", "fish & chips"},
		"unknown tags strip": {"<div><b>bold text</b></div>", "bold text"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := RenderMessageHTML(tc.in); got != tc.want {
				t.Errorf("RenderMessageHTML(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	t.Run("pre preserves internal newlines", func(t *testing.T) {
		in := "<pre>line1\nline2\nline3</pre>"
		out := stripANSI(RenderMessageHTML(in))
		if !strings.Contains(out, "line1\nline2\nline3") {
			t.Errorf("pre block newlines collapsed, want multi-line layout preserved: %q", out)
		}
	})

	t.Run("every text-X class paints via classStyle", func(t *testing.T) {
		classes := []string{"purple", "yellow", "cyan", "green", "orange", "red", "pito", "fg", "fg-dim", "fg-faded"}
		for _, name := range classes {
			class := "text-" + name
			in := fmt.Sprintf(`<span class="%s">Z</span>`, class)
			out := RenderMessageHTML(in)
			want := classStyle(class).Render("Z")
			if !strings.Contains(out, want) {
				t.Errorf("RenderMessageHTML(%q) = %q, want it to contain classStyle(%q).Render(%q) = %q",
					in, out, class, "Z", want)
			}
		}
	})

	t.Run("disconnect outcome_text shape", func(t *testing.T) {
		// The real payload shape (confirmation outcome_text on a disconnect
		// card): plain sentence, then <br>, then a <pre> block carrying a
		// colored span — html.go's doc comment names this exact call site.
		in := `Disconnected from @x. 8 vids deleted.<br><pre><span class="text-purple">shrug</span></pre>`
		out := RenderMessageHTML(in)
		stripped := stripANSI(out)

		if strings.ContainsAny(stripped, "<>") {
			t.Errorf("tag characters leaked into the flattened output: %q", stripped)
		}
		if !strings.Contains(stripped, "8 vids deleted.\n") {
			t.Errorf("<br> must become a newline between the sentence and the pre block: %q", stripped)
		}
		if !strings.Contains(stripped, "shrug") {
			t.Errorf("pre block content lost: %q", stripped)
		}
		if want := classStyle("text-purple").Render("shrug"); !strings.Contains(out, want) {
			t.Errorf("span inside <pre> lost its color: %q, want it to contain %q", out, want)
		}
	})
}
