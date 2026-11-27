// Package message defines the application's transport-independent message model
// and helpers that normalize Telegram content into it. See plan §7.3, §7.4, §9.3.
package message

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Message is the internal representation passed to the formatter. Telegram-
// specific types must be converted to this model in internal/telegram.
//
// The first six fields are the original output model. The remaining fields
// carry context that is available in Telegram updates but was not part of the
// first version; they are used by the built-in "event" template and by the
// monitoring filters.
type Message struct {
	ChatID    int64
	MessageID int
	Date      time.Time
	Channel   string
	Sender    string
	Text      string

	// ReplyToMessageID is the message this one replies to, or 0.
	ReplyToMessageID int
	// IsEdit reports that Telegram marked the message as edited.
	IsEdit bool
	// IsPinned reports that the message is pinned in its chat.
	IsPinned bool
	// IsMention reports that the message mentions the authenticated account.
	IsMention bool
	// SenderID is the Telegram ID of the author, or 0 when unknown.
	SenderID int64
	// SenderUsername is the author's username without "@", or empty.
	SenderUsername string
	// SenderIsBot reports that the author is a bot.
	SenderIsBot bool
	// Media lists the attachments carried by the message. It is never nil, so
	// JSON output uses [] rather than null.
	Media []MediaRef
	// Forward describes the original source when the message was forwarded.
	Forward *Forward
}

// MediaRef is a transport-independent reference to one attachment. FileID is
// the Telegram media identifier; URL is set for webpage previews.
type MediaRef struct {
	Kind   MediaKind `json:"kind"`
	FileID int64     `json:"file_id,omitempty"`
	URL    string    `json:"url,omitempty"`
}

// Forward describes the provenance of a forwarded message.
type Forward struct {
	FromID       int64  `json:"from_id,omitempty"`
	FromName     string `json:"from_name,omitempty"`
	FromUsername string `json:"from_username,omitempty"`
	PostAuthor   string `json:"post_author,omitempty"`
	ChannelPost  int    `json:"channel_post,omitempty"`
	DateUnix     int64  `json:"date_unix,omitempty"`
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
	MediaWebPage
)

// String returns the stable name used in JSON output.
func (k MediaKind) String() string {
	switch k {
	case MediaNone:
		return "none"
	case MediaPhoto:
		return "photo"
	case MediaVideo:
		return "video"
	case MediaDocument:
		return "document"
	case MediaVoice:
		return "voice"
	case MediaAudio:
		return "audio"
	case MediaSticker:
		return "sticker"
	case MediaPoll:
		return "poll"
	case MediaLocation:
		return "location"
	case MediaContact:
		return "contact"
	case MediaWebPage:
		return "web_page"
	default:
		return "unknown"
	}
}

// MarshalJSON encodes a media kind as its string name.
func (k MediaKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

// UnmarshalJSON decodes a media kind from its string name.
func (k *MediaKind) UnmarshalJSON(b []byte) error {
	var name string
	if err := json.Unmarshal(b, &name); err != nil {
		return err
	}
	switch name {
	case "none":
		*k = MediaNone
	case "photo":
		*k = MediaPhoto
	case "video":
		*k = MediaVideo
	case "document":
		*k = MediaDocument
	case "voice":
		*k = MediaVoice
	case "audio":
		*k = MediaAudio
	case "sticker":
		*k = MediaSticker
	case "poll":
		*k = MediaPoll
	case "location":
		*k = MediaLocation
	case "contact":
		*k = MediaContact
	case "web_page":
		*k = MediaWebPage
	default:
		return fmt.Errorf("unknown media kind %q", name)
	}
	return nil
}

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
	ID        int64
	FirstName string
	LastName  string
	Username  string
	IsBot     bool
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
