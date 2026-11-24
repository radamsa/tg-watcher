// Package message defines the application's transport-independent message model
// and helpers that normalize Telegram content into it. See plan §7.3, §7.4, §9.3.
package message

import (
	"strings"
	"time"
)

// Message is the internal representation passed to the formatter. Telegram-
// specific types must be converted to this model in internal/telegram.
type Message struct {
	ChatID    int64
	MessageID int
	Date      time.Time
	Channel   string
	Sender    string
	Text      string
}

// MediaKind identifies the kind of attachment carried by a message.
type MediaKind int

const (
	MediaNone MediaKind = iota
	MediaPhoto
	MediaVideo
	MediaDocument
	MediaVoice
	MediaAudio
	MediaSticker
	MediaPoll
	MediaLocation
	MediaContact
)

// marker returns the text marker for a media kind.
func (k MediaKind) marker() string {
	switch k {
	case MediaPhoto:
		return "[photo]"
	case MediaVideo:
		return "[video]"
	case MediaDocument:
		return "[document]"
	case MediaVoice:
		return "[voice]"
	case MediaAudio:
		return "[audio]"
	case MediaSticker:
		return "[sticker]"
	case MediaPoll:
		return "[poll]"
	case MediaLocation:
		return "[location]"
	case MediaContact:
		return "[contact]"
	default:
		return ""
	}
}

// ComposeText builds the normalized text value: an optional forward prefix, an
// optional media marker and the caption/body, joined by newlines. This value is
// shared by every template.
func ComposeText(kind MediaKind, body, forwardSource string) string {
	var parts []string
	if src := strings.TrimSpace(forwardSource); src != "" {
		parts = append(parts, "[fwd: "+src+"]")
	}
	if marker := kind.marker(); marker != "" {
		parts = append(parts, marker)
	}
	if body != "" {
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n")
}

// User describes a regular message author.
type User struct {
	FirstName string
	LastName  string
	Username  string
}

// DisplayName returns "First Last" with surrounding whitespace trimmed.
func (u *User) DisplayName() string {
	if u == nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimSpace(u.FirstName) + " " + strings.TrimSpace(u.LastName))
}

// ComposeSender builds the sender field (plan §7.3):
//
//  1. known user -> "First Last (@username)" or "First Last";
//  2. otherwise post_author, when Telegram provided it;
//  3. otherwise the chat title for anonymous posts;
//  4. otherwise an empty string.
func ComposeSender(u *User, postAuthor, chatTitle string) string {
	if name := u.DisplayName(); name != "" {
		if username := strings.TrimSpace(u.Username); username != "" {
			return name + " (@" + username + ")"
		}
		return name
	}
	if pa := strings.TrimSpace(postAuthor); pa != "" {
		return pa
	}
	return strings.TrimSpace(chatTitle)
}
