package api

import (
	"errors"
	"fmt"
)

// StatusError is a non-2xx answer from a server that WAS reachable — the
// transport worked, PITO didn't. Preflight tells this apart from dial
// failures so the owner hears "your box is down", not "check your network".
type StatusError struct {
	Method string
	Path   string
	Code   int
	Status string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("api: %s %s: %s", e.Method, e.Path, e.Status)
}

var (
	// ErrUnauthorized covers 401s and login-page redirects — the session
	// cookie is missing, expired (24h idle timeout server-side), or invalid.
	ErrUnauthorized = errors.New("api: unauthorized")
	// ErrInvalidTOTP is a rejected /session code; the caller may re-prompt.
	ErrInvalidTOTP = errors.New("api: invalid TOTP code")
	// ErrThrottled is the server's per-IP login throttle (10 failures /
	// 5 min). Callers must stop prompting, never retry-loop.
	ErrThrottled = errors.New("api: login throttled, try again later")
	// ErrNotFound is an unknown conversation uuid — the client goes back
	// to the picker instead of erroring.
	ErrNotFound = errors.New("api: not found")
	// ErrInvalidTitle is a rejected rename PATCH (blank/whitespace-only
	// title) — the same 422 conversations_controller.rb#update returns for
	// the web's inline rename.
	ErrInvalidTitle = errors.New("api: invalid conversation title")
)
