package app

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gmrdad82/pito-tui/internal/ui"
)

func TestLooksLikeUUID(t *testing.T) {
	cases := map[string]bool{
		"50189b77-1234-4abc-8def-0123456789ab": true,
		"50189B77-1234-4ABC-8DEF-0123456789AB": true,
		"Library Sync":                         false,
		"":                                     false,
		"50189b77-1234-4abc-8def":              false,
	}
	for in, want := range cases {
		if got := looksLikeUUID(in); got != want {
			t.Errorf("looksLikeUUID(%q) = %v, want %v", in, got, want)
		}
	}
}

// resumeMux serves GET /resume.json with the given rows, one page, no
// next_cursor — enough for resolution tests that don't exercise pagination.
func resumeMux(t *testing.T, rows string) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rows))
	})
	return mux
}

func TestResolveResumeArgEmptyIsNoop(t *testing.T) {
	client, _ := newTestClient(t, resumeMux(t, `{"recent":[],"older":[]}`))
	uuid, err := resolveResumeArg(t.Context(), client, "", true)
	if err != nil || uuid != "" {
		t.Fatalf("resolveResumeArg(\"\") = %q, %v", uuid, err)
	}
}

func TestResolveResumeArgUUIDSkipsResolution(t *testing.T) {
	// No /resume.json handler at all: a uuid-shaped --resume must never
	// hit the network.
	mux := http.NewServeMux()
	client, _ := newTestClient(t, mux)
	const in = "50189b77-1234-4abc-8def-0123456789ab"
	uuid, err := resolveResumeArg(t.Context(), client, in, true)
	if err != nil || uuid != in {
		t.Fatalf("resolveResumeArg(uuid) = %q, %v, want %q, nil", uuid, err, in)
	}
}

func TestResolveResumeArgUnauthedNameYieldsEmpty(t *testing.T) {
	// No session to ask: an unauthenticated run silently yields "" and
	// lets the ordinary /login flow win, rather than erroring.
	mux := http.NewServeMux()
	client, _ := newTestClient(t, mux)
	uuid, err := resolveResumeArg(t.Context(), client, "Library Sync", false)
	if err != nil || uuid != "" {
		t.Fatalf("resolveResumeArg(unauthed name) = %q, %v, want \"\", nil", uuid, err)
	}
}

func TestResolveResumeArgNameExactMatch(t *testing.T) {
	client, _ := newTestClient(t, resumeMux(t, `{
		"recent": [{"uuid": "aaa", "title": "Library Sync", "display_name": "Library Sync"}],
		"older": [{"uuid": "bbb", "title": "thumbnail ideas", "display_name": "thumbnail ideas"}]
	}`))
	uuid, err := resolveResumeArg(t.Context(), client, "library sync", true)
	if err != nil || uuid != "aaa" {
		t.Fatalf("resolveResumeArg(name) = %q, %v, want \"aaa\", nil", uuid, err)
	}
}

func TestResolveResumeArgNameMissingListsCloseCandidates(t *testing.T) {
	client, _ := newTestClient(t, resumeMux(t, `{
		"recent": [{"uuid": "aaa", "title": "Library Sync 2", "display_name": "Library Sync 2"}],
		"older": [{"uuid": "bbb", "title": "thumbnail ideas", "display_name": "thumbnail ideas"}]
	}`))
	_, err := resolveResumeArg(t.Context(), client, "Library Sync", true)
	if err == nil {
		t.Fatal("resolveResumeArg(missing name) = nil error, want one")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Library Sync") || !strings.Contains(msg, "Library Sync 2") || !strings.Contains(msg, "aaa") {
		t.Errorf("error must name the search and its close candidate:\n%s", msg)
	}
	if strings.Contains(msg, "thumbnail ideas") {
		t.Errorf("error must not list an unrelated conversation as a close candidate:\n%s", msg)
	}
}

func TestResolveResumeArgNameMissingNoCandidates(t *testing.T) {
	client, _ := newTestClient(t, resumeMux(t, `{
		"recent": [{"uuid": "bbb", "title": "thumbnail ideas", "display_name": "thumbnail ideas"}],
		"older": []
	}`))
	_, err := resolveResumeArg(t.Context(), client, "nothing like it", true)
	if err == nil || !strings.Contains(err.Error(), "nothing like it") {
		t.Fatalf("err = %v, want it to name the missing search", err)
	}
}

func TestResolveResumeArgNameAmbiguous(t *testing.T) {
	client, _ := newTestClient(t, resumeMux(t, `{
		"recent": [{"uuid": "aaa", "title": "release prep", "display_name": "release prep"}],
		"older": [{"uuid": "bbb", "title": "release prep", "display_name": "release prep"}]
	}`))
	_, err := resolveResumeArg(t.Context(), client, "release prep", true)
	if err == nil {
		t.Fatal("resolveResumeArg(ambiguous name) = nil error, want one")
	}
	msg := err.Error()
	if !strings.Contains(msg, "aaa") || !strings.Contains(msg, "bbb") || !strings.Contains(msg, "uuid") {
		t.Errorf("ambiguous error must list every match and point at the uuid escape hatch:\n%s", msg)
	}
}

func TestResolveResumeArgNamePaginatesAllPages(t *testing.T) {
	mux := http.NewServeMux()
	calls := 0
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after") == "" {
			_, _ = w.Write([]byte(`{"recent":[{"uuid":"aaa","title":"page one","display_name":"page one"}],"older":[],"next_cursor":"page2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"recent":[],"older":[{"uuid":"bbb","title":"target","display_name":"target"}],"next_cursor":""}`))
	})
	client, _ := newTestClient(t, mux)
	uuid, err := resolveResumeArg(t.Context(), client, "target", true)
	if err != nil || uuid != "bbb" {
		t.Fatalf("resolveResumeArg across pages = %q, %v, want \"bbb\", nil", uuid, err)
	}
	if calls != 2 {
		t.Errorf("expected the resolver to walk both pages, got %d calls", calls)
	}
}

func TestResumeHintLine(t *testing.T) {
	cases := []struct {
		name         string
		uuid, label  string
		instanceFlag *string
		want         string
	}{
		{
			name:  "titled conversation, no --instance",
			uuid:  "50189b77-1234-4abc-8def-0123456789ab",
			label: "Library Sync",
			want:  `To resume this conversation: pito-tui --resume "Library Sync"`,
		},
		{
			name:  "untitled conversation falls back to the uuid",
			uuid:  "50189b77-1234-4abc-8def-0123456789ab",
			label: "",
			want:  "To resume this conversation: pito-tui --resume 50189b77-1234-4abc-8def-0123456789ab",
		},
		{
			name:         "instance flag is mirrored into the command",
			uuid:         "50189b77-1234-4abc-8def-0123456789ab",
			label:        "Library Sync",
			instanceFlag: strPtr("https://dev.pitomd.com"),
			want:         `To resume this conversation: pito-tui --instance https://dev.pitomd.com --resume "Library Sync"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resumeHintLine(tc.uuid, tc.label, tc.instanceFlag); got != tc.want {
				t.Errorf("resumeHintLine() = %q, want %q", got, tc.want)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

func TestPrintResumeHintPrintsOnCleanQuitWithConversation(t *testing.T) {
	var out strings.Builder
	m := ui.NewModel(nil, nil, ui.WithConversation("50189b77-1234-4abc-8def-0123456789ab"), ui.WithSplash(false))
	printResumeHint(Options{Stdout: &out}, m, nil)
	got := out.String()
	if !strings.Contains(got, "--resume 50189b77-1234-4abc-8def-0123456789ab") {
		t.Errorf("printResumeHint output = %q", got)
	}
}

func TestPrintResumeHintSilentOnRunError(t *testing.T) {
	var out strings.Builder
	m := ui.NewModel(nil, nil, ui.WithConversation("50189b77-1234-4abc-8def-0123456789ab"))
	printResumeHint(Options{Stdout: &out}, m, fmt.Errorf("boom"))
	if out.Len() != 0 {
		t.Errorf("printResumeHint must stay silent on a non-nil runErr, got %q", out.String())
	}
}

func TestPrintResumeHintSilentWithoutAConversation(t *testing.T) {
	var out strings.Builder
	// A brand-new conversation that never sent its first message still
	// carries a blank uuid — nothing to resume.
	m := ui.NewModel(nil, nil, ui.WithNewConversation())
	printResumeHint(Options{Stdout: &out}, m, nil)
	if out.Len() != 0 {
		t.Errorf("printResumeHint must stay silent with no conversation to resume, got %q", out.String())
	}
}

func TestPrintResumeHintSilentOnNonModelFinalState(t *testing.T) {
	var out strings.Builder
	printResumeHint(Options{Stdout: &out}, quitOnlyModel{}, nil)
	if out.Len() != 0 {
		t.Errorf("printResumeHint must stay silent when the final tea.Model isn't ui.Model, got %q", out.String())
	}
}

// quitOnlyModel is a minimal tea.Model stand-in for the "not a ui.Model"
// branch of printResumeHint's type assertion.
type quitOnlyModel struct{}

func (quitOnlyModel) Init() tea.Cmd                       { return nil }
func (quitOnlyModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return quitOnlyModel{}, nil }
func (quitOnlyModel) View() tea.View                      { return tea.NewView("") }
