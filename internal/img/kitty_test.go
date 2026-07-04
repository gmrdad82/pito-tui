package img

import (
	"strings"
	"testing"
)

func TestSupportedEnvDetection(t *testing.T) {
	clear := func(t *testing.T) {
		for _, v := range []string{"KITTY_WINDOW_ID", "GHOSTTY_RESOURCES_DIR", "TERM", "TERM_PROGRAM"} {
			t.Setenv(v, "")
		}
	}
	t.Run("plain xterm is not supported", func(t *testing.T) {
		clear(t)
		t.Setenv("TERM", "xterm-256color")
		if Supported() {
			t.Error("xterm must not claim image support")
		}
	})
	cases := map[string][2]string{
		"kitty window id": {"KITTY_WINDOW_ID", "1"},
		"kitty TERM":      {"TERM", "xterm-kitty"},
		"ghostty":         {"TERM_PROGRAM", "ghostty"},
		"wezterm":         {"TERM_PROGRAM", "WezTerm"},
	}
	for name, kv := range cases {
		t.Run(name, func(t *testing.T) {
			clear(t)
			t.Setenv(kv[0], kv[1])
			if !Supported() {
				t.Errorf("%s must enable image support", name)
			}
		})
	}
}

func TestShowerEscapeFraming(t *testing.T) {
	var out strings.Builder
	s := NewShower(&out)

	// Small payload: single chunk, m=0, then a placement at the box.
	s.Show([]byte("png-bytes"), 2, 40, 38, 11)
	first := out.String()
	if !strings.Contains(first, "_Ga=t,f=100,i=") || !strings.Contains(first, ",q=2,m=0;") {
		t.Errorf("transmit frame wrong: %q", first)
	}
	if !strings.Contains(first, "\x1b7\x1b[2;40H") || !strings.Contains(first, ",c=38,r=11,q=2") || !strings.Contains(first, "\x1b8") {
		t.Errorf("placement frame wrong: %q", first)
	}

	// A second image deletes the first before placing.
	out.Reset()
	s.Show([]byte("more-bytes"), 2, 40, 38, 11)
	if !strings.Contains(out.String(), "_Ga=d,d=i,i=") {
		t.Error("second Show must delete the prior image")
	}

	out.Reset()
	s.Clear()
	if !strings.Contains(out.String(), "_Ga=d,d=i,i=") {
		t.Error("Clear must delete the pinned image")
	}
	out.Reset()
	s.Clear()
	if out.Len() != 0 {
		t.Error("Clear with nothing pinned must write nothing")
	}
}

func TestShowerChunksLargePayloads(t *testing.T) {
	var out strings.Builder
	NewShower(&out).Show(make([]byte, 9000), 1, 1, 10, 5) // ~12k base64 → 3 chunks
	got := out.String()
	if strings.Count(got, "m=1;") != 2 || !strings.Contains(got, "\x1b_Gm=0;") {
		t.Errorf("chunking wrong (want 2 continuation frames + final m=0):\n%.200s", got)
	}
}

func TestNilShowerIsSafe(t *testing.T) {
	var s *Shower
	s.Show([]byte("x"), 1, 1, 1, 1)
	s.Clear()
}
