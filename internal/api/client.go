// Package api is the HTTP half of the PITO contract: TOTP login, scrollback
// snapshots, sends, and the resume list. The cable half lives in
// internal/cable; both share the same PersistentJar session cookie.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	base *url.URL
	jar  *PersistentJar
	hc   *http.Client
}

// New builds a client for the instance, persisting cookies at jarPath.
func New(instanceURL, jarPath string) (*Client, error) {
	base, err := url.Parse(strings.TrimRight(instanceURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("api: invalid instance URL %q", instanceURL)
	}
	jar, err := LoadJar(jarPath)
	if err != nil {
		return nil, err
	}
	return &Client{
		base: base,
		jar:  jar,
		hc: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
			// Never follow redirects: a 302 toward a login page is an
			// auth failure in disguise (the contract says 401, but Rails
			// auth stacks love redirects). The status is inspected below.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// Jar exposes the shared cookie jar for the websocket dialer.
func (c *Client) Jar() *PersistentJar { return c.jar }

// BaseURL returns the instance URL the client was built with.
func (c *Client) BaseURL() *url.URL { return c.base }

// Login POSTs the TOTP code to /session. pito is TOTP-only — there is no
// email or password. The minted session cookie lands in the jar via the
// Set-Cookie header.
func (c *Client) Login(ctx context.Context, otp string) error {
	resp, body, err := c.do(ctx, http.MethodPost, "/session", map[string]string{"otp": otp})
	if err != nil {
		return err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return ErrThrottled
	}
	var reply struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &reply) == nil && strings.Contains(reply.Error, "throttle") {
		return ErrThrottled
	}
	return ErrInvalidTOTP
}

// FetchChat GETs the scrollback snapshot for a conversation.
func (c *Client) FetchChat(ctx context.Context, uuid string) (*ChatPage, error) {
	resp, body, err := c.do(ctx, http.MethodGet, "/chat/"+url.PathEscape(uuid)+".json", nil)
	if err != nil {
		return nil, err
	}
	if err := c.checkAuth(resp); err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("conversation %s: %w", uuid, ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api: GET /chat/%s.json: %s", uuid, resp.Status)
	}
	var page ChatPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("api: decoding scrollback: %w", err)
	}
	return &page, nil
}

// FetchResume GETs one page of the conversation picker list (tui-needs ask
// 9a). after is the opaque cursor from the previous page's
// ResumeList.NextCursor; pass "" for the first page. limit defaults to 50
// when <= 0, mirroring FetchNotifications' cursor idiom — WITH ONE
// deliberate deviation: when after=="" AND limit<=0 (today's zero-value
// first-page call), the request carries NO query string at all, so it is
// byte-identical to the pre-pagination request this endpoint has always
// taken. A server that predates pagination — or any other combination
// that still resolves to that exact request — just answers its one
// complete recent/older payload, which decodes fine since NextCursor and
// Rows simply come back zero-valued. Any other after/limit combination
// rides the params, same as FetchNotifications.
func (c *Client) FetchResume(ctx context.Context, after string, limit int) (*ResumeList, error) {
	path := "/resume.json"
	if after != "" || limit > 0 {
		if limit <= 0 {
			limit = 50
		}
		q := url.Values{}
		if after != "" {
			q.Set("after", after)
		}
		q.Set("limit", strconv.Itoa(limit))
		path += "?" + q.Encode()
	}
	resp, body, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if err := c.checkAuth(resp); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api: GET /resume.json: %s", resp.Status)
	}
	var list ResumeList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("api: decoding resume list: %w", err)
	}
	return &list, nil
}

// ErrNotificationsUnavailable marks a server without /notifications.json —
// the endpoint predates some servers. A missing route answers 404 or, when
// Rails content-negotiates the .json format away, 406 (live-verified on
// dev pre-rollout). Callers gate the notifications panel on this error
// rather than surfacing it as a hard failure.
var ErrNotificationsUnavailable = errors.New("api: notifications endpoint unavailable")

// PatchNotification PATCHes /notifications/:id {read:} — the web
// sidebar's own mark-read call (arrow arrival marks read, click
// toggles); the server broadcasts the fresh unread count to every
// client afterward. 204 on success.
func (c *Client) PatchNotification(ctx context.Context, id int64, read bool) error {
	path := fmt.Sprintf("/notifications/%d", id)
	resp, _, err := c.do(ctx, http.MethodPatch, path, map[string]any{"read": read})
	if err != nil {
		return err
	}
	if err := c.checkAuth(resp); err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("notification patch: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// PickerRow is one row of the game/video picker feeds (GET
// /games/picker.json, /videos/picker.json — pito's PickerController,
// built for this client). Handle is videos-only (the channel it lives on).
type PickerRow struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Handle string `json:"handle"`
}

// PickerPage is one page of a picker feed: 50 rows and the opaque keyset
// cursor for the next page ("" when the list is exhausted) — the house
// after=/next_cursor convention.
type PickerPage struct {
	Rows       []PickerRow `json:"rows"`
	NextCursor string      `json:"next_cursor"`
}

// FetchPickerPage GETs one page of /<noun>/picker.json (noun: "games" or
// "videos"), after "" for page 1. Auth-gated server-side.
func (c *Client) FetchPickerPage(ctx context.Context, noun, after string) (*PickerPage, error) {
	path := "/" + noun + "/picker.json"
	if after != "" {
		q := url.Values{}
		q.Set("after", after)
		path += "?" + q.Encode()
	}
	resp, body, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if err := c.checkAuth(resp); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s picker: unexpected status %d", noun, resp.StatusCode)
	}
	var page PickerPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("%s picker: decoding: %w", noun, err)
	}
	return &page, nil
}

// FetchNotifications GETs one page of the notification list. after is the
// opaque cursor from the previous page's NotificationPage.NextCursor; pass
// "" for the first page. limit defaults to 50 when <= 0. The param is named
// `after` to match pito's own keyset convention (Notification.panel_page).
// FetchVersion GETs /version — the running server build's identity (the
// tag after the mini-status dot; pito's VersionsController, the same
// endpoint the web's cable-health nudge polls). Auth-gated server-side,
// so it rides the same cookie jar as everything else.
func (c *Client) FetchVersion(ctx context.Context) (string, error) {
	resp, body, err := c.do(ctx, http.MethodGet, "/version", nil)
	if err != nil {
		return "", err
	}
	if err := c.checkAuth(resp); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("version: unexpected status %d", resp.StatusCode)
	}
	var out struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("version: decoding: %w", err)
	}
	return out.Version, nil
}

func (c *Client) FetchNotifications(ctx context.Context, after string, limit int) (*NotificationPage, error) {
	if limit <= 0 {
		limit = 50
	}
	q := url.Values{}
	if after != "" {
		q.Set("after", after)
	}
	q.Set("limit", strconv.Itoa(limit))
	resp, body, err := c.do(ctx, http.MethodGet, "/notifications.json?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if err := c.checkAuth(resp); err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNotAcceptable {
		return nil, ErrNotificationsUnavailable
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api: GET /notifications.json: %s", resp.Status)
	}
	var page NotificationPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("api: decoding notifications: %w", err)
	}
	return &page, nil
}

// SendMessage POSTs raw input to /chat. uuid may be empty — the server then
// creates the conversation and the result carries its new uuid. width is
// the terminal column count (viewport_width), like the web client sends.
// The input is NEVER parsed here: slash commands are the server's grammar.
// SendOpt adds optional scope params to a send — the web's hidden
// channel/period fields, included ONLY while their hint is active.
type SendOpt func(map[string]any)

// WithChannelScope rides the channel param (list vids/games sends).
func WithChannelScope(channel string) SendOpt {
	return func(p map[string]any) {
		if channel != "" {
			p["channel"] = channel
		}
	}
}

// WithPeriodScope rides the period param (analyze sends).
func WithPeriodScope(period string) SendOpt {
	return func(p map[string]any) {
		if period != "" {
			p["period"] = period
		}
	}
}

func (c *Client) SendMessage(ctx context.Context, uuid, input string, width int, opts ...SendOpt) (*SendResult, error) {
	payload := map[string]any{"input": input}
	if uuid != "" {
		payload["uuid"] = uuid
	}
	if width > 0 {
		payload["viewport_width"] = width
	}
	for _, opt := range opts {
		opt(payload)
	}
	resp, body, err := c.do(ctx, http.MethodPost, "/chat", payload)
	if err != nil {
		return nil, err
	}
	if err := c.checkAuth(resp); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Server notices (web-only verbs and friends) ride on 422s;
		// classify by body before giving up.
		if notice := decodeNotice(body); notice != nil {
			return &SendResult{Notice: notice}, nil
		}
		return nil, fmt.Errorf("api: POST /chat: %s", resp.Status)
	}

	if notice := decodeNotice(body); notice != nil {
		return &SendResult{Notice: notice}, nil
	}
	// Live-verified reply: always {uuid, turn_id} 201. Whether the
	// conversation was CREATED is a request-side fact — we sent no uuid.
	var reply struct {
		TurnID int64  `json:"turn_id"`
		UUID   string `json:"uuid"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &reply); err != nil {
			return nil, fmt.Errorf("api: decoding POST /chat reply: %w", err)
		}
	}
	res := &SendResult{TurnID: reply.TurnID}
	if uuid == "" && reply.UUID != "" {
		res.CreatedUUID = reply.UUID
	}
	return res, nil
}

func decodeNotice(body []byte) *ServerNotice {
	var notice ServerNotice
	if json.Unmarshal(body, &notice) == nil && notice.Error != "" {
		return &notice
	}
	return nil
}

// Suggest asks the server-side palette what fits at input[:cursor].
// PatchScope persists the cycled channel/period onto the conversation
// (the web's fire-and-forget PATCH /chat/:uuid) so the server's own
// fallbacks resolve the same scope on later verbs.
func (c *Client) PatchScope(ctx context.Context, uuid, channel, period string) error {
	payload := map[string]any{"scope_channel": channel, "stats_period": period}
	resp, _, err := c.do(ctx, http.MethodPatch, "/chat/"+uuid, payload)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("api: PATCH /chat/%s: %s", uuid, resp.Status)
	}
	return nil
}

// RenameConversation PATCHes /chat/:uuid {title:} — the same endpoint the
// web sidebar's inline rename uses (pito--rename's #commitRename,
// Conversation::Rename server-side). Returns the server's canonical title
// on success. A blank title is rejected 422 (ErrInvalidTitle); an unknown
// uuid answers 404 (ErrNotFound).
func (c *Client) RenameConversation(ctx context.Context, uuid, title string) (string, error) {
	path := "/chat/" + url.PathEscape(uuid)
	resp, body, err := c.do(ctx, http.MethodPatch, path, map[string]string{"title": title})
	if err != nil {
		return "", err
	}
	if err := c.checkAuth(resp); err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("conversation %s: %w", uuid, ErrNotFound)
	}
	if resp.StatusCode == http.StatusUnprocessableEntity {
		return "", ErrInvalidTitle
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("api: PATCH /chat/%s: %s", uuid, resp.Status)
	}
	var out struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("api: decoding rename reply: %w", err)
	}
	return out.Title, nil
}

// DeleteConversation DELETEs /chat/:uuid — the same async-delete endpoint
// the web sidebar's dd chord uses (Conversation::RequestDeletion
// server-side: marks the conversation deleting and hands the slow cascade
// to a background job). 204 on success; an unknown uuid answers 404
// (ErrNotFound).
func (c *Client) DeleteConversation(ctx context.Context, uuid string) error {
	path := "/chat/" + url.PathEscape(uuid)
	resp, _, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	if err := c.checkAuth(resp); err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("conversation %s: %w", uuid, ErrNotFound)
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("api: DELETE /chat/%s: %s", uuid, resp.Status)
	}
	return nil
}

func (c *Client) Suggest(ctx context.Context, uuid, input string, cursor int) (*Suggestions, error) {
	payload := map[string]any{"input": input, "cursor": cursor}
	if uuid != "" {
		payload["uuid"] = uuid
	}
	resp, body, err := c.do(ctx, http.MethodPost, "/suggestions", payload)
	if err != nil {
		return nil, err
	}
	if err := c.checkAuth(resp); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api: POST /suggestions: %s", resp.Status)
	}
	var s Suggestions
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("api: decoding suggestions: %w", err)
	}
	return &s, nil
}

// checkAuth maps 401s and redirects (login pages) to ErrUnauthorized.
func (c *Client) checkAuth(resp *http.Response) error {
	if resp.StatusCode == http.StatusUnauthorized ||
		(resp.StatusCode >= 300 && resp.StatusCode < 400) {
		return ErrUnauthorized
	}
	return nil
}

// do runs one JSON request and slurps the body. Every request advertises
// Accept: application/json so the Rails side picks the JSON paths.
func (c *Client) do(ctx context.Context, method, path string, payload any) (*http.Response, []byte, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base.String()+path, body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, nil, err
	}
	return resp, raw, nil
}
