package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// Prompter supplies the TOTP code. Production reads stdin pre-TUI; tests
// inject canned codes.
type Prompter interface {
	TOTP() (string, error)
}

// LoginFunc is api.Client.Login's shape, injected for tests.
type LoginFunc func(ctx context.Context, otp string) error

const maxLoginAttempts = 3

// EnsureLogin drives the TOTP prompt loop: up to maxLoginAttempts invalid
// codes, an immediate stop when the server throttles (10 failures / 5 min
// per IP — retry-looping would only dig deeper), and any other error
// surfaced as-is.
func EnsureLogin(ctx context.Context, login LoginFunc, prompt Prompter, out io.Writer) error {
	for attempt := 1; ; attempt++ {
		code, err := prompt.TOTP()
		if err != nil {
			return err
		}
		err = login(ctx, code)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, api.ErrThrottled):
			fmt.Fprintln(out, "Login throttled by the server — wait a few minutes and try again.")
			return err
		case errors.Is(err, api.ErrInvalidTOTP):
			if attempt >= maxLoginAttempts {
				fmt.Fprintln(out, "Too many invalid codes.")
				return err
			}
			fmt.Fprintln(out, "Invalid code, try again.")
		default:
			return err
		}
	}
}
