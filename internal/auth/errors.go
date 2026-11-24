// Package auth handles user authorization inputs and the on-disk session store.
// It deliberately avoids gotd types so it can be unit-tested without Telegram.
// See plan §5.
package auth

import (
	"errors"
	"fmt"
	"time"
)

// Authentication errors surfaced by the Telegram adapter. They are sentinel
// values so the application can map them to exit code 3.
var (
	ErrInvalidPhone      = errors.New("invalid phone number")
	ErrInvalidCode       = errors.New("invalid confirmation code")
	ErrCodeExpired       = errors.New("confirmation code expired")
	ErrInvalidPassword   = errors.New("invalid 2FA password")
	ErrPasswordRequired  = errors.New("2FA password required")
	ErrPhoneBanned       = errors.New("phone number is not allowed")
	ErrSessionInvalid    = errors.New("session is invalid or was revoked")
	ErrInteractiveNeeded = errors.New("an interactive terminal is required")
	ErrSignUpRequired    = errors.New("account sign-up is not supported")
)

// FloodWaitError reports a server-imposed rate limit.
type FloodWaitError struct {
	Duration time.Duration
}

func (e *FloodWaitError) Error() string {
	return fmt.Sprintf("Telegram rate limit: wait %s", e.Duration)
}

// IsAuthError reports whether err belongs to the authorization class of
// failures (exit code 3).
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	var flood *FloodWaitError
	if errors.As(err, &flood) {
		return true
	}
	for _, target := range []error{
		ErrInvalidPhone,
		ErrInvalidCode,
		ErrCodeExpired,
		ErrInvalidPassword,
		ErrPasswordRequired,
		ErrPhoneBanned,
		ErrSessionInvalid,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
