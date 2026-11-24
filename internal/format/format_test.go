package format

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tg-watcher/internal/message"
)

func utcSpec(template string) Spec {
	return Spec{
		Template:        template,
		DatetimeFormat:  "2006-01-02 15:04:05",
		Location:        time.UTC,
		MultilineIndent: 4,
	}
}

func sampleMessage() message.Message {
	return message.Message{
		ChatID:    42,
		MessageID: 7,
		Date:      time.Date(2026, 11, 23, 12, 34, 56, 0, time.UTC),
		Channel:   "Example Channel",
		Sender:    "Ada Lovelace (@ada)",
		Text:      "hello",
	}
}

func TestRenderText(t *testing.T) {
	tmpl, err := Compile(utcSpec("[{datetime}] [{channel}] [{sender}] {text}"))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got := tmpl.Render(sampleMessage())
	want := "[2026-11-23 12:34:56] [Example Channel] [Ada Lovelace (@ada)] hello"
	if got != want {
		t.Fatalf("Render = %q, want %q", got, want)
	}
}

func TestRenderJSONFilterRoundTrip(t *testing.T) {
	tmpl, err := Compile(utcSpec(`{"datetime":{datetime|json},"channel":{channel|json},"sender":{sender|json},"text":{text|json}}`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	m := sampleMessage()
	m.Text = "Привет, \"мир\"\\\t\nвторая строка"

	encoded := tmpl.Render(m)
	if strings.Contains(encoded, "\n") {
		t.Fatalf("JSONL record contains a raw newline: %q", encoded)
	}

	var decoded struct {
		Datetime string `json:"datetime"`
		Channel  string `json:"channel"`
		Sender   string `json:"sender"`
		Text     string `json:"text"`
	}
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%q): %v", encoded, err)
	}
	if decoded.Text != m.Text {
		t.Fatalf("text round-trip = %q, want %q", decoded.Text, m.Text)
	}
	if decoded.Channel != m.Channel || decoded.Sender != m.Sender {
		t.Fatalf("unexpected fields: %#v", decoded)
	}
	if decoded.Datetime != "2026-11-23 12:34:56" {
		t.Fatalf("datetime = %q", decoded.Datetime)
	}
}

func TestJSONFilterDoesNotIndentText(t *testing.T) {
	tmpl, err := Compile(utcSpec("{text|json}"))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	m := message.Message{Text: "a\nb"}
	var got string
	if err := json.Unmarshal([]byte(tmpl.Render(m)), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != "a\nb" {
		t.Fatalf("text = %q, want %q", got, "a\nb")
	}
}

func TestMultilineIndent(t *testing.T) {
	tmpl, err := Compile(utcSpec("{text}"))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got := tmpl.Render(message.Message{Text: "a\nb\nc"})
	want := "a\n    b\n    c"
	if got != want {
		t.Fatalf("Render = %q, want %q", got, want)
	}
}

func TestLiteralBraces(t *testing.T) {
	tmpl, err := Compile(utcSpec("{{{channel}}} {{ {text} }}"))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got := tmpl.Render(message.Message{Channel: "ch", Text: "tx"})
	want := "{ch} { tx }"
	if got != want {
		t.Fatalf("Render = %q, want %q", got, want)
	}
}

func TestJSONBracesAreLiteral(t *testing.T) {
	tmpl, err := Compile(utcSpec(`{"channel":{channel|json},"text":{text|json}}`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got := tmpl.Render(message.Message{Channel: "ch", Text: "tx"})
	want := `{"channel":"ch","text":"tx"}`
	if got != want {
		t.Fatalf("Render = %q, want %q", got, want)
	}
}

func TestCompileErrors(t *testing.T) {
	tests := []struct {
		name     string
		template string
	}{
		{"empty", ""},
		{"blank", "   "},
		{"unclosed", "{text"},
		{"unknown field", "{unknown}"},
		{"unknown filter", "{text|upper}"},
		{"two filters", "{text|json|json}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Compile(utcSpec(tt.template)); err == nil {
				t.Fatalf("Compile(%q) succeeded, want error", tt.template)
			}
		})
	}
}

func TestNegativeIndentRejected(t *testing.T) {
	spec := utcSpec("{text}")
	spec.MultilineIndent = -1
	if _, err := Compile(spec); err == nil {
		t.Fatal("expected error for negative indent")
	}
}
