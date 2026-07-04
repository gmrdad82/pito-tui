package api

import "errors"

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
)
