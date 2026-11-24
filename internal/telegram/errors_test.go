package telegram

import (
	"errors"
	"testing"
	"time"

	gotdauth "github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tgerr"

	"tg-watcher/internal/auth"
)

func TestClassifyAuthErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"nil", nil, nil},
		{"password invalid", gotdauth.ErrPasswordInvalid, auth.ErrInvalidPassword},
		{"password not provided", gotdauth.ErrPasswordNotProvided, auth.ErrPasswordRequired},

		{"PHONE_NUMBER_INVALID", tgerr.New(400, "PHONE_NUMBER_INVALID"), auth.ErrInvalidPhone},
		{"PHONE_NUMBER_BANNED", tgerr.New(406, "PHONE_NUMBER_BANNED"), auth.ErrPhoneBanned},
		{"PHONE_CODE_INVALID", tgerr.New(400, "PHONE_CODE_INVALID"), auth.ErrInvalidCode},
		{"PHONE_CODE_EXPIRED", tgerr.New(400, "PHONE_CODE_EXPIRED"), auth.ErrCodeExpired},
		{"PASSWORD_HASH_INVALID", tgerr.New(400, "PASSWORD_HASH_INVALID"), auth.ErrInvalidPassword},
		{"AUTH_KEY_UNREGISTERED", tgerr.New(401, "AUTH_KEY_UNREGISTERED"), auth.ErrSessionInvalid},
		{"SESSION_EXPIRED", tgerr.New(401, "SESSION_EXPIRED"), auth.ErrSessionInvalid},

		{"FloodWait", tgerr.New(420, "FLOOD_WAIT_5"), &auth.FloodWaitError{Duration: 5 * time.Second}},
		{"FloodPremiumWait", tgerr.New(420, "FLOOD_PREMIUM_WAIT_60"), &auth.FloodWaitError{Duration: 60 * time.Second}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.err)
			if fw, ok := tt.want.(*auth.FloodWaitError); ok {
				var gotFw *auth.FloodWaitError
				if !errors.As(got, &gotFw) || gotFw.Duration != fw.Duration {
					t.Fatalf("Classify = %T %v, want FloodWaitError{%v}", got, got, fw.Duration)
				}
			} else if !errors.Is(got, tt.want) {
				t.Fatalf("Classify = %v %T, want %v", got, got, tt.want)
			}
		})
	}

	// Unknown RPC error should pass through.
	original := tgerr.New(500, "INTERNAL")
	if got := Classify(original); !errors.Is(got, original) {
		t.Fatalf("unknown RPC error should be preserved: %v", got)
	}
}