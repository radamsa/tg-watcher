// Package telegram is the only package allowed to depend on gotd generated
// types. It adapts gotd to the application's interfaces. See plan §9.1.
package telegram

import (
	"errors"

	gotdauth "github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tgerr"

	"tg-watcher/internal/auth"
)

// Classify maps gotd/Telegram errors onto the application's auth errors. It
// returns the original error when it is not recognized, so the caller can treat
// it as a runtime failure.
func Classify(err error) error {
	if err == nil {
		return nil
	}

	if d, ok := tgerr.AsFloodWait(err); ok {
		return &auth.FloodWaitError{Duration: d}
	}
	if errors.Is(err, gotdauth.ErrPasswordInvalid) {
		return auth.ErrInvalidPassword
	}
	if errors.Is(err, gotdauth.ErrPasswordNotProvided) {
		return auth.ErrPasswordRequired
	}
	var signUp *gotdauth.SignUpRequired
	if errors.As(err, &signUp) {
		return auth.ErrSignUpRequired
	}

	if rpc, ok := tgerr.As(err); ok {
		switch rpc.Type {
		case "PHONE_NUMBER_INVALID", "PHONE_NUMBER_EMPTY":
			return auth.ErrInvalidPhone
		case "PHONE_NUMBER_BANNED":
			return auth.ErrPhoneBanned
		case "PHONE_CODE_INVALID":
			return auth.ErrInvalidCode
		case "PHONE_CODE_EXPIRED":
			return auth.ErrCodeExpired
		case "PASSWORD_HASH_INVALID":
			return auth.ErrInvalidPassword
		case "AUTH_KEY_UNREGISTERED", "AUTH_KEY_INVALID", "AUTH_KEY_DUPLICATED",
			"SESSION_REVOKED", "SESSION_EXPIRED", "USER_DEACTIVATED_BAN":
			return auth.ErrSessionInvalid
		}
	}
	return err
}
