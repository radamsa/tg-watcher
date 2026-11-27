package message

import (
	"encoding/json"
	"testing"
)

func TestComposeText(t *testing.T) {
	tests := []struct {
		name    string
		kind    MediaKind
		body    string
		forward string
		want    string
	}{
		{"plain text", MediaNone, "hello", "", "hello"},
		{"photo with caption", MediaPhoto, "cap", "", "[photo]\ncap"},
		{"video no caption", MediaVideo, "", "", "[video]"},
		{"document", MediaDocument, "cap", "", "[document]\ncap"},
		{"voice", MediaVoice, "", "", "[voice]"},
		{"audio", MediaAudio, "", "", "[audio]"},
		{"sticker", MediaSticker, "", "", "[sticker]"},
		{"poll", MediaPoll, "Question?", "", "[poll]\nQuestion?"},
		{"location", MediaLocation, "", "", "[location]"},
		{"contact", MediaContact, "", "", "[contact]"},
		{"forwarded text", MediaNone, "body", "Source", "[fwd: Source]\nbody"},
		{"forwarded photo", MediaPhoto, "cap", "Source", "[fwd: Source]\n[photo]\ncap"},
		{"blank forward source ignored", MediaNone, "body", "   ", "body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ComposeText(tt.kind, tt.body, tt.forward); got != tt.want {
				t.Fatalf("ComposeText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMediaRefJSON(t *testing.T) {
	refs := []MediaRef{
		{Kind: MediaPhoto, FileID: 7},
		{Kind: MediaWebPage, URL: "https://example.com"},
		{Kind: MediaNone},
	}
	b, err := json.Marshal(refs)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `[{"kind":"photo","file_id":7},{"kind":"web_page","url":"https://example.com"},{"kind":"none"}]`
	if string(b) != want {
		t.Fatalf("Marshal = %s, want %s", b, want)
	}
}

func TestMediaKindString(t *testing.T) {
	tests := map[MediaKind]string{
		MediaNone:     "none",
		MediaPhoto:    "photo",
		MediaVideo:    "video",
		MediaDocument: "document",
		MediaVoice:    "voice",
		MediaAudio:    "audio",
		MediaSticker:  "sticker",
		MediaPoll:     "poll",
		MediaLocation: "location",
		MediaContact:  "contact",
		MediaWebPage:  "web_page",
	}
	for kind, want := range tests {
		if got := kind.String(); got != want {
			t.Fatalf("%d.String() = %q, want %q", kind, got, want)
		}
	}
}

func TestComposeSender(t *testing.T) {
	tests := []struct {
		name       string
		user       *User
		postAuthor string
		chatTitle  string
		want       string
	}{
		{"user with username", &User{FirstName: "Ada", LastName: "Lovelace", Username: "ada"}, "", "Chat", "Ada Lovelace (@ada)"},
		{"user without username", &User{FirstName: "Ada", LastName: "Lovelace"}, "", "Chat", "Ada Lovelace"},
		{"user first name only", &User{FirstName: "Ada", Username: "ada"}, "", "Chat", "Ada (@ada)"},
		{"post author when no user", nil, "Channel Admin", "Chat", "Channel Admin"},
		{"anonymous uses chat title", nil, "", "Chat", "Chat"},
		{"unknown source is empty", nil, "", "", ""},
		{"empty user falls back to post author", &User{}, "Channel Admin", "Chat", "Channel Admin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ComposeSender(tt.user, tt.postAuthor, tt.chatTitle); got != tt.want {
				t.Fatalf("ComposeSender = %q, want %q", got, tt.want)
			}
		})
	}
}
