package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/ui"
)

// uuidPattern matches the standard 8-4-4-4-12 hex shape pito's conversation
// uuids come back in (Rails SecureRandom.uuid) — a --resume argument in
// this shape skips resolution entirely, no network call needed.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func looksLikeUUID(s string) bool { return uuidPattern.MatchString(s) }

// resolveResumeArg turns --resume's argument into a conversation uuid.
// "" is a no-op (the flag wasn't passed). A uuid-shaped argument is
// returned as-is with no network call. Any other string resolves by title
// against GET /resume.json — but only when authed: an unauthenticated run
// has no session to ask, so it silently yields "" and lets the ordinary
// /login flow win, the same tolerance cfg.Conversation already gets from
// Run's mode switch.
func resolveResumeArg(ctx context.Context, client *api.Client, arg string, authed bool) (string, error) {
	switch {
	case arg == "":
		return "", nil
	case looksLikeUUID(arg):
		return arg, nil
	case !authed:
		return "", nil
	default:
		return resolveConversationName(ctx, client, arg)
	}
}

// resolveConversationName GETs every /resume.json page — the same endpoint
// and pagination the /resume picker itself walks (model.go's
// fetchResumeMoreCmd) — and matches name against each row's Label()
// case-insensitively. An exact single match returns its uuid; zero or
// several matches return a clean, listable error rather than guessing.
func resolveConversationName(ctx context.Context, client *api.Client, name string) (string, error) {
	rows, err := fetchAllResumeRows(ctx, client)
	if err != nil {
		return "", fmt.Errorf("resolving --resume %q: %w", name, err)
	}
	var exact []api.ResumeRow
	for _, r := range rows {
		if strings.EqualFold(r.Label(), name) {
			exact = append(exact, r)
		}
	}
	switch len(exact) {
	case 1:
		return exact[0].UUID, nil
	case 0:
		return "", missingConversationError(name, rows)
	default:
		return "", ambiguousConversationError(name, exact)
	}
}

// maxResumePages caps the pagination walk (fetchAllResumeRows) so a
// misbehaving server (a next_cursor that never resolves to "") can't spin
// this forever — 200 pages at 50 rows/page is 10,000 conversations, well
// past anything a single owner's channel manager will ever hold.
const maxResumePages = 200

func fetchAllResumeRows(ctx context.Context, client *api.Client) ([]api.ResumeRow, error) {
	var rows []api.ResumeRow
	after := ""
	for range maxResumePages {
		page, err := client.FetchResume(ctx, after, 0)
		if err != nil {
			return nil, err
		}
		rows = append(rows, page.Recent...)
		rows = append(rows, page.Older...)
		if page.NextCursor == "" || page.NextCursor == after {
			break
		}
		after = page.NextCursor
	}
	return rows, nil
}

// closeMatchLimit caps how many candidates a missing-name error lists.
const closeMatchLimit = 5

func missingConversationError(name string, rows []api.ResumeRow) error {
	lower := strings.ToLower(name)
	var candidates []api.ResumeRow
	for _, r := range rows {
		if strings.Contains(strings.ToLower(r.Label()), lower) {
			candidates = append(candidates, r)
			if len(candidates) >= closeMatchLimit {
				break
			}
		}
	}
	if len(candidates) == 0 {
		return fmt.Errorf("no conversation named %q", name)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "no conversation named %q — close candidates:\n", name)
	for _, r := range candidates {
		fmt.Fprintf(&b, "  %s (%s)\n", r.Label(), r.UUID)
	}
	b.WriteString("use one of those names verbatim, or the uuid directly")
	return errors.New(b.String())
}

func ambiguousConversationError(name string, matches []api.ResumeRow) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d conversations — use the uuid instead:\n", name, len(matches))
	for i, r := range matches {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "  %s (%s)", r.Label(), r.UUID)
	}
	return errors.New(b.String())
}

// printResumeHint prints the post-quit "resume this conversation" line
// (owner 2026-07-14, "like claude does") after the TUI has fully torn
// down — Bubble Tea owns the terminal until program.Run() returns, so this
// must never run while the program is still alive. Silent whenever there
// is nothing to resume (ui.Model.ResumeHint's ok is false) or the run
// didn't end in a clean tea.Quit (runErr != nil).
func printResumeHint(opts Options, final tea.Model, runErr error) {
	if runErr != nil {
		return
	}
	m, ok := final.(ui.Model)
	if !ok {
		return
	}
	uuid, label, ok := m.ResumeHint()
	if !ok {
		return
	}
	out := opts.Stdout
	if out == nil {
		out = os.Stdout
	}
	fmt.Fprintln(out, resumeHintLine(uuid, label, opts.InstanceURL))
}

// resumeHintLine formats the hint's suggested command: the conversation's
// NAME quoted when it has one, else its uuid (owner 2026-07-14 detail 1),
// mirroring -instance into the command IFF this run itself was started
// with one (instanceFlag non-nil — a config-file instance stays invisible,
// same as any other config default the CLI surface doesn't echo back).
func resumeHintLine(uuid, label string, instanceFlag *string) string {
	target := uuid
	if label != "" {
		target = strconv.Quote(label)
	}
	var b strings.Builder
	b.WriteString("pito-tui")
	if instanceFlag != nil {
		fmt.Fprintf(&b, " --instance %s", *instanceFlag)
	}
	fmt.Fprintf(&b, " --resume %s", target)
	return "To resume this conversation: " + b.String()
}
