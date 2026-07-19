package api

import (
	"encoding/json"
	"strconv"
	"time"
)

// Known event kinds (Event::KINDS on the Rails side). The TUI renders these
// specially and falls back to a generic renderer for anything else — an
// unknown kind must degrade, never crash, so old clients survive new
// servers.
const (
	KindEcho                 = "echo"
	KindSystem               = "system"
	KindError                = "error"
	KindEnhanced             = "enhanced"
	KindThinking             = "thinking"
	KindConfirmation         = "confirmation"
	KindSystemFollowUp       = "system_follow_up"
	KindEnhancedFollowUp     = "enhanced_follow_up"
	KindConfirmationFollowUp = "confirmation_follow_up"
	KindThemeDiff            = "theme_diff"
	KindAi                   = "ai"
)

// Event is the canonical scrollback unit. Payload stays raw: kind-specific
// decoding happens in the renderers, so decoding a page can never fail on
// an unknown kind.
type Event struct {
	ID     int64  `json:"id"`
	TurnID int64  `json:"turn_id"`
	Kind   string `json:"kind"`
	// Position is the server's per-conversation event order
	// (Event.create_with_position!, serialized by Pito::Stream::EventJson on
	// both the cable and the backfill). It is the ONE order that holds when
	// the two transports interleave: a broadcast missed during a reconnect's
	// subscribe-confirm gap arrives late via the re-sync Merge, AFTER
	// later-positioned events already delivered live (owner screenshot
	// 2026-07-17 02:01: a turn's echo merged in under its own thinking
	// indicator, so the spinner read as stacked on the previous turn).
	// Zero when a server predating the field sent the event.
	Position  int64           `json:"position"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// AiBlock is one entry in an "ai" payload's blocks array. Raw always holds
// the whole block object — not just its type-specific keys — so a renderer
// can decode further layers itself; this package only peeks at "type" to
// route the block, it never interprets a block's body.
type AiBlock struct {
	Type string
	Raw  json.RawMessage
}

// UnmarshalJSON keeps Raw pointed at the whole block, even when the block
// isn't shaped like {type, ...}. An unknown block type or a malformed one
// (missing "type", or not an object at all) still round-trips its bytes
// untouched and never errors — the server already degrades its side, and
// the renderer degrades a bad block to a raw-JSON line rather than the
// whole event failing to decode.
func (b *AiBlock) UnmarshalJSON(raw []byte) error {
	b.Raw = append(json.RawMessage(nil), raw...)
	var head struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(raw, &head) // best-effort: a bad head just leaves Type == ""
	b.Type = head.Type
	return nil
}

// AiPayload is the kind "ai" event body (pito 2.0.0, @ai tool). Status is
// "pending" while the model is still working the turn and "done" once
// Blocks is final. CostAmount is a pointer because the server distinguishes
// "no price yet" (nil, e.g. a pending payload or unknown-pricing model)
// from an actual $0.00 (free model, still a settled cost). CostEstimated
// marks a cost pito computed itself because the provider reported none
// (e.g. OpenCode Zen) rather than a genuine provider receipt; a reported
// cost carries no flag at all, so an absent key decodes to false here
// exactly like every other tolerant bool in this package.
type AiPayload struct {
	Status        string    `json:"status"`
	Blocks        []AiBlock `json:"blocks"`
	Prompt        string    `json:"prompt"`
	Model         string    `json:"model"`
	Provider      string    `json:"provider"`
	Effort        string    `json:"effort"`
	CostAmount    *float64  `json:"cost_amount"`
	CostCurrency  string    `json:"cost_currency"`
	CostEstimated bool      `json:"cost_estimated"`
	ReplyHandle   string    `json:"reply_handle"`
	ReplyConsumed bool      `json:"reply_consumed"`
	AnchorEventID int64     `json:"anchor_event_id"`
}

// DecodeAiPayload decodes an "ai" event's payload. It is tolerant of
// unknown top-level fields — encoding/json already ignores JSON keys with
// no matching struct field — so the server can ship new payload keys ahead
// of the TUI knowing about them without breaking the decode. Per-block
// tolerance is AiBlock.UnmarshalJSON's job; this only ever errors when the
// payload isn't a JSON object at all.
func DecodeAiPayload(raw json.RawMessage) (AiPayload, error) {
	var p AiPayload
	err := json.Unmarshal(raw, &p)
	return p, err
}

// ConversationHit is one row of a kind "system" event's conversation-search
// hits card — the REAL contract, the generic list-card shape
// (table_heading/table_rows, the same mechanism `ls vids/games` use)
// Pito::MessageBuilder::Conversation::Hits builds (pito
// lib/pito/message_builder/conversation/hits.rb; wired from
// Pito::Chat::Handlers::SearchConversations#ok, a `search conversations
// for/like …` reply). This REPLACES a top-level `conversation_hits` array
// this type used to decode — a contract the server deleted before this
// client shipped the code that read it (pito-tui 3.0.1 fix); no server
// build ever sent that shape.
//
// A row belongs to a hits card, not some other list card that also happens
// to be kind "system" with table_rows (e.g. an `ls games` reply), by
// carrying row-level `data: {anchor_event_id, conversation_uuid}` —
// hits.rb's #row_for is the ONLY builder that stamps data at the ROW level
// (every other list card's `data` lives on individual cells, for the
// click-to-prefill affordance) — see DecodeSystemPayload for where that
// discriminator is applied.
//
// ConversationID is gone: every route that opens a conversation
// (GET /chat/:uuid.json, Conversation#to_param, and the `/resume` slash
// command — config/pito/tools.yml `resume:`) is uuid-keyed, so
// ConversationUUID alone is enough to resolve a hit for real. See the
// TUI's jump affordance (internal/ui/hitpicker.go) for how a
// cross-conversation hit is handled now that there's a uuid to act on.
type ConversationHit struct {
	ConversationUUID string
	AnchorEventID    int64
	Title            string
	// Score is the 0-100 similarity bar cell (a semantic `like` hit's
	// second cell — hits.rb's #like_cells, the same {score:} cell shape
	// ScoreBarComponent renders for similar-games/channel-recommendation
	// cards) — nil for a lexical `for`/bare hit.
	Score *int
	// OccurrenceCount is the match-count cell (a lexical `for`/bare hit's
	// second cell — hits.rb's #for_cells, a plain {text:} cell parsed back
	// to an int here) — nil for a semantic `like` hit. A card's hits share
	// ONE mode (hits.rb's like_mode?: every hit in one `call` carries the
	// same non-nil field), so exactly one of Score/OccurrenceCount is ever
	// non-nil per hit — never both, never neither.
	OccurrenceCount *int
}

// SystemPayload decodes the parts of a kind "system" event's payload the
// hit picker needs beyond the generic table_heading/table_rows render
// (render.go's textPayload already covers the on-screen table — this is a
// second, independent decode for a different purpose, the house's existing
// parallel-decode pattern; transcript.go's LiveHandles does the same with
// reply_handle/reply_consumed). Hits is empty for every system event that
// isn't a search-conversations reply — an old server, a plain list card,
// or anything else that doesn't carry the row-level data contract — so
// callers never special-case "missing" vs "not a hits card":
// hitsFromLatestEvent's gate (hitpicker.go) is simply len(Hits) == 0.
type SystemPayload struct {
	Hits []ConversationHit
}

// hitsPayloadWire is the raw table_rows shape (hits.rb's #row_for output,
// serialized), decoded once and filtered into the friendlier
// ConversationHit slice by DecodeSystemPayload. Unexported: callers only
// ever want SystemPayload.Hits.
type hitsPayloadWire struct {
	TableRows []hitRowWire `json:"table_rows"`
}

// hitRowWire mirrors one table_rows entry exactly: Cells[0] is the
// clickable conversation-name cell (only its `text` matters here — the
// `class`/`data` prefill-token fields drive the WEB's click-to-type
// affordance and are irrelevant to a client that submits the equivalent
// `/resume` command itself, see hitpicker.go#resumeHit), Cells[1] is the
// mode-dependent value cell (see hitCellWire), and Data is the row-level
// anchor_event_id/conversation_uuid contract hits.rb's comment calls out
// as "do not drop" — it's what tells this row apart from any other list
// card's table_rows (DecodeSystemPayload's discriminator).
type hitRowWire struct {
	Cells []hitCellWire `json:"cells"`
	Data  struct {
		AnchorEventID    int64  `json:"anchor_event_id"`
		ConversationUUID string `json:"conversation_uuid"`
	} `json:"data"`
}

// hitCellWire covers both shapes a hits row's second cell takes: the
// like-mode score bar ({score:}) and the for-mode occurrence count
// ({text:}). Score is a pointer so its PRESENCE, not its value, is what
// tells the two modes apart — a genuine 0 score must still decode as
// "like mode", never fall through to a bogus occurrence count.
type hitCellWire struct {
	Text  string `json:"text"`
	Score *int   `json:"score"`
}

// DecodeSystemPayload decodes a "system" event's payload for its hits, if
// any (see SystemPayload's doc comment for the row-level discriminator
// that separates a real hits card from any other table_rows-bearing list
// card). A row missing the row-level data contract, or with fewer than two
// cells, is simply skipped — not a hits row, not an error. Errors only
// when the payload isn't a JSON object at all, or a table_rows entry isn't
// shaped like a row object (e.g. a bare string) — a clean error return,
// never a panic, exactly as before this rewrite; hitsFromLatestEvent
// already treats any error as "no hits".
func DecodeSystemPayload(raw json.RawMessage) (SystemPayload, error) {
	var wire hitsPayloadWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return SystemPayload{}, err
	}

	hits := make([]ConversationHit, 0, len(wire.TableRows))
	for _, row := range wire.TableRows {
		if row.Data.ConversationUUID == "" || row.Data.AnchorEventID == 0 || len(row.Cells) < 2 {
			continue // not a hits row — no row-level data contract, or too few cells
		}
		hit := ConversationHit{
			ConversationUUID: row.Data.ConversationUUID,
			AnchorEventID:    row.Data.AnchorEventID,
			Title:            row.Cells[0].Text,
		}
		switch {
		case row.Cells[1].Score != nil:
			hit.Score = row.Cells[1].Score
		case row.Cells[1].Text != "":
			if n, err := strconv.Atoi(row.Cells[1].Text); err == nil {
				hit.OccurrenceCount = &n
			}
		}
		hits = append(hits, hit)
	}
	return SystemPayload{Hits: hits}, nil
}

type Conversation struct {
	UUID string `json:"uuid"`
	// Title and DisplayName mirror the Rails serializer (live-verified:
	// there is no "name" key). DisplayName is the human label.
	Title       string `json:"title"`
	DisplayName string `json:"display_name"`
	// Context is the server-computed context meter (tui-needs.md item 1).
	// The server is the source of truth — the TUI never computes this.
	Context *ContextMeter `json:"context"`
	// Scope is the persisted channel/period filter (tui-needs.md ask,
	// gate on presence): the web seeds its cyclers from these columns.
	Scope *Scope `json:"scope"`
}

// Scope mirrors conversation.scope_channel / stats_period.
type Scope struct {
	Channel string `json:"channel"`
	Period  string `json:"period"`
}

// ContextMeter is the conversation's context fill — served on the chat
// backfill and patched live by conversation.update cable messages.
type ContextMeter struct {
	Pct       float64 `json:"pct"`
	Count     int     `json:"count"`
	Threshold int     `json:"threshold"`
}

// NotifCount carries the unread notification count.
type NotifCount struct {
	Unread int `json:"unread"`
}

// NotificationRow is one entry in GET /notifications.json.
type NotificationRow struct {
	ID        int64     `json:"id"`
	Message   string    `json:"message"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

// NotificationPage is GET /notifications.json — a cursor-paginated slice of
// the notification list (newest first, live-verified contract). NextCursor
// is a plain string rather than a pointer: encoding/json already leaves a
// non-pointer field at its zero value on a JSON null, so null, an absent
// key, and an explicit "" all decode to "" here — every one of those means
// "no more pages" per the contract, so callers never need to special-case
// null themselves.
type NotificationPage struct {
	Rows       []NotificationRow `json:"rows"`
	NextCursor string            `json:"next_cursor"`
}

// Label is the human-facing conversation name for status bars and pickers.
func (c Conversation) Label() string {
	if c.DisplayName != "" {
		return c.DisplayName
	}
	return c.Title
}

// ChatPage is GET /chat/:uuid.json — the scrollback snapshot used for the
// initial paint and for every reconnect re-sync (the cable has no replay).
type ChatPage struct {
	Conversation  Conversation `json:"conversation"`
	Events        []Event      `json:"events"`
	Notifications *NotifCount  `json:"notifications"`
	// Channels is the cycler list (["@all", "@handle", …]) — tui-needs
	// ask, gate on presence.
	Channels []string `json:"channels"`
}

// ResumeRow is one conversation in the picker.
type ResumeRow struct {
	UUID           string    `json:"uuid"`
	Title          string    `json:"title"`
	DisplayName    string    `json:"display_name"`
	LastActivityAt time.Time `json:"last_activity_at"`
	// AI marks a conversation with any ai-kind event — the picker's
	// sparkle badge (tui-needs ask 9b). The wire key isn't named yet: pito
	// may ship "ai" or "has_ai", so UnmarshalJSON decodes either spelling
	// until the contract answer picks one.
	AI bool `json:"-"`
}

// Label mirrors Conversation.Label for picker rows.
func (r ResumeRow) Label() string {
	if r.DisplayName != "" {
		return r.DisplayName
	}
	return r.Title
}

// UnmarshalJSON decodes ResumeRow's normal fields, then best-effort peeks
// at "ai" and "has_ai" for AI (tui-needs ask 9b, still unnamed — see AI's
// doc comment). Either key true sets it; both absent, both false, or a
// non-bool value under either key all degrade to false rather than erroring
// the row.
func (r *ResumeRow) UnmarshalJSON(raw []byte) error {
	type resumeRowFields ResumeRow
	var fields resumeRowFields
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	*r = ResumeRow(fields)

	var flags struct {
		AI    *bool `json:"ai"`
		HasAI *bool `json:"has_ai"`
	}
	_ = json.Unmarshal(raw, &flags) // best-effort: a bad flag just leaves it nil
	r.AI = (flags.AI != nil && *flags.AI) || (flags.HasAI != nil && *flags.HasAI)
	return nil
}

// ResumeList is GET /resume.json — Conversation.recency_groups serialized:
// everything within 24h of the most recent activity is "recent". Paginated
// per tui-needs ask 9a: page 1 (no `after`) keeps this exact shape, and
// gains NextCursor, the opaque cursor for the next page. Like
// NotificationPage.NextCursor, it is a plain string rather than a pointer —
// a JSON null, an absent key, and an explicit "" all decode to "" here, and
// every one of those means "no more pages" — which is ALSO what today's
// unpaginated server's single response looks like (it never sends the key
// at all), so an old server's one complete page and a genuinely last page
// are indistinguishable by design: exactly the tolerance old-server support
// wants.
//
// Pages past the first may flatten recency grouping: since nothing beyond
// page 1 is ever "recent" relative to page 1's own window, later rows may
// arrive under a flat `rows` key instead of split into recent/older. Rows
// decodes that key tolerantly; UnmarshalJSON folds it into Older so callers
// only ever need to read Recent/Older regardless of which shape a given
// page used.
type ResumeList struct {
	Recent        []ResumeRow `json:"recent"`
	Older         []ResumeRow `json:"older"`
	Rows          []ResumeRow `json:"rows"`
	Notifications *NotifCount `json:"notifications"`
	NextCursor    string      `json:"next_cursor"`
}

// UnmarshalJSON decodes ResumeList's normal fields, then folds any `rows`
// entries onto the end of Older (see the type doc comment) so a flat later
// page reads exactly like a page with an empty `recent` and its rows under
// `older`. Rows itself is left populated too — it mirrors the wire, in case
// a caller ever needs to tell "arrived flat" apart from "arrived grouped" —
// but nothing in this package reads it once the merge is done.
func (l *ResumeList) UnmarshalJSON(raw []byte) error {
	type resumeListFields ResumeList
	var fields resumeListFields
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	*l = ResumeList(fields)
	if len(l.Rows) > 0 {
		l.Older = append(l.Older, l.Rows...)
	}
	return nil
}

// SendResult is the classified POST /chat reply. Live-verified: the server
// always answers {uuid, turn_id} 201, so "created" is a REQUEST-side fact —
// a blank-uuid send that came back with a uuid.
type SendResult struct {
	// TurnID identifies the in-flight turn (pending spinner bookkeeping).
	TurnID int64
	// CreatedUUID is set only when a blank-uuid send created the
	// conversation — the caller then fetches scrollback and subscribes.
	CreatedUUID string
	// Notice carries the server's error/message reply (web-only verbs and
	// friends) — rendered as a dim notice in the product's own voice, not
	// as an error.
	Notice *ServerNotice
}

// ServerNotice is the {error, message} reply shape (live-verified:
// {"error":"web_only","message":"That command wears a mouse cursor…"}).
// pito 2.0.0 renamed the fallback key verb→tool; both decode for
// back-compat with pre-2.0.0 servers.
type ServerNotice struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Tool    string `json:"tool"`
	Verb    string `json:"verb"`
}

// Text is the line the UI shows — the server's prose when it sent some.
func (n ServerNotice) Text() string {
	if n.Message != "" {
		return n.Message
	}
	if name := n.Tool; name != "" || n.Verb != "" {
		if name == "" {
			name = n.Verb
		}
		return name + " is web-only — open the web app for it"
	}
	return "the server declined that one (" + n.Error + ")"
}

// Suggestions is POST /suggestions — the server-driven palette. The
// grammar lives in tools.yml server-side; the TUI never hardcodes tools,
// it just renders what the ontology answers per keystroke.
type Suggestions struct {
	Mode      string       `json:"mode"`
	Stage     string       `json:"stage"`
	Ghost     Ghost        `json:"ghost"`
	MenuItems []Suggestion `json:"menu_items"`
}

type Ghost struct {
	CompleteCurrent string `json:"complete_current"`
	NextHint        string `json:"next_hint"`
}

type Suggestion struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Insert      string `json:"insert"`
	Masked      bool   `json:"masked"`
}
