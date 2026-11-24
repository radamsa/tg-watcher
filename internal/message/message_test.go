package message

import "testing"

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
