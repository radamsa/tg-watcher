package watch

import (
	"testing"

	"tg-watcher/internal/message"
)

func TestFilterNilMatches(t *testing.T) {
	var f *Filter
	if !f.Match(message.Message{}) {
		t.Fatal("nil filter should match everything")
	}
}

func TestFilterMentionsOnly(t *testing.T) {
	f, err := NewFilter(FilterOptions{MentionsOnly: true})
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	if f.Match(message.Message{IsMention: false}) {
		t.Fatal("should reject non-mention")
	}
	if !f.Match(message.Message{IsMention: true}) {
		t.Fatal("should accept mention")
	}
}

func TestFilterFromID(t *testing.T) {
	f, err := NewFilter(FilterOptions{From: []string{"100"}})
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	if f.Match(message.Message{SenderID: 99}) {
		t.Fatal("should reject different ID")
	}
	if !f.Match(message.Message{SenderID: 100}) {
		t.Fatal("should match ID")
	}
}

func TestFilterFromUsername(t *testing.T) {
	f, err := NewFilter(FilterOptions{From: []string{"@alice"}})
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	if f.Match(message.Message{SenderUsername: "bob"}) {
		t.Fatal("should reject different username")
	}
	if !f.Match(message.Message{SenderUsername: "Alice"}) {
		t.Fatal("should match username case-insensitively")
	}
}

func TestFilterFromSubstring(t *testing.T) {
	f, err := NewFilter(FilterOptions{From: []string{"Ada"}})
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	if f.Match(message.Message{Sender: "Bob"}) {
		t.Fatal("should reject non-matching name")
	}
	if !f.Match(message.Message{Sender: "Ada Lovelace (@ada)"}) {
		t.Fatal("should match substring")
	}
}

func TestFilterMatchAny(t *testing.T) {
	f, err := NewFilter(FilterOptions{Match: []string{"error", "critical"}})
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	if f.Match(message.Message{Text: "hello"}) {
		t.Fatal("should reject non-matching text")
	}
	if !f.Match(message.Message{Text: "something error here"}) {
		t.Fatal("should match 'error'")
	}
	if !f.Match(message.Message{Text: "critical alert"}) {
		t.Fatal("should match 'critical'")
	}
}

func TestFilterMatchAll(t *testing.T) {
	f, err := NewFilter(FilterOptions{Match: []string{"error", "critical"}, All: true})
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	if f.Match(message.Message{Text: "only error"}) {
		t.Fatal("should require both")
	}
	if !f.Match(message.Message{Text: "error is critical"}) {
		t.Fatal("should match both")
	}
}

func TestFilterCombined(t *testing.T) {
	f, err := NewFilter(FilterOptions{
		MentionsOnly: true,
		From:         []string{"@alice"},
		Match:        []string{"hello"},
	})
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	// All three must pass.
	if f.Match(message.Message{IsMention: false, SenderUsername: "alice", Text: "hello"}) {
		t.Fatal("should reject non-mention")
	}
	if f.Match(message.Message{IsMention: true, SenderUsername: "bob", Text: "hello"}) {
		t.Fatal("should reject non-alice")
	}
	if f.Match(message.Message{IsMention: true, SenderUsername: "alice", Text: "goodbye"}) {
		t.Fatal("should reject non-matching text")
	}
	if !f.Match(message.Message{IsMention: true, SenderUsername: "alice", Text: "hello there"}) {
		t.Fatal("should pass all filters")
	}
}

func TestFilterInvalidRegex(t *testing.T) {
	_, err := NewFilter(FilterOptions{Match: []string{"[unclosed"}})
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}
