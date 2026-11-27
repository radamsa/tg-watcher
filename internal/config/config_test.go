package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tg-watcher/internal/message"
)

const credentials = `
[telegram]
api_id = 12345
api_hash = "0123456789abcdef"
`

func isolateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TG_API_ID", "")
	t.Setenv("TG_API_HASH", "")
}

func TestDefaults(t *testing.T) {
	isolateEnv(t)
	cfg, err := Parse([]byte(credentials))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.DefaultFormat != BuiltinText {
		t.Fatalf("default_format = %q", cfg.DefaultFormat)
	}
	if got := cfg.Watch.PollInterval.Std(); got != 60*time.Second {
		t.Fatalf("poll_interval = %s", got)
	}
	if cfg.Watch.PollPageSize != 100 {
		t.Fatalf("poll_page_size = %d", cfg.Watch.PollPageSize)
	}
	for _, name := range []string{BuiltinText, BuiltinJSONL} {
		if _, ok := cfg.Template(name); !ok {
			t.Fatalf("built-in template %q missing", name)
		}
	}

	tmpl, _ := cfg.Template(BuiltinText)
	got := tmpl.Render(message.Message{
		Date:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local),
		Channel: "ch",
		Sender:  "s",
		Text:    "hi",
	})
	want := "[2026-01-02 03:04:05] [ch] [s] hi"
	if got != want {
		t.Fatalf("text render = %q, want %q", got, want)
	}
}

func TestOverrides(t *testing.T) {
	isolateEnv(t)
	doc := `default_format = "jsonl"

` + credentials + `
[watch]
poll_interval = "45s"
poll_page_size = 50

[formats.text]
template = "{channel}: {text}"
multiline_indent = 2

[formats.compact]
template = "{sender}|{text}"
timezone = "UTC"
datetime_format = "15:04"
`
	cfg, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.DefaultFormat != "jsonl" {
		t.Fatalf("default_format = %q", cfg.DefaultFormat)
	}
	if got := cfg.Watch.PollInterval.Std(); got != 45*time.Second {
		t.Fatalf("poll_interval = %s", got)
	}
	if cfg.Watch.PollPageSize != 50 {
		t.Fatalf("poll_page_size = %d", cfg.Watch.PollPageSize)
	}
	tmpl, ok := cfg.Template(BuiltinText)
	if !ok {
		t.Fatal("text template missing")
	}
	if got := tmpl.Render(message.Message{Channel: "c", Text: "t"}); got != "c: t" {
		t.Fatalf("overridden text render = %q", got)
	}
	if _, ok := cfg.Template("compact"); !ok {
		t.Fatal("custom template missing")
	}
}

func TestEnvOverridesCredentials(t *testing.T) {
	t.Setenv("TG_API_ID", "999")
	t.Setenv("TG_API_HASH", "envhash")
	cfg, err := Parse([]byte(credentials))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Telegram.APIID != 999 || cfg.Telegram.APIHash != "envhash" {
		t.Fatalf("credentials = %#v", cfg.Telegram)
	}
}

func TestInvalid(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want string
	}{
		{"missing api_id", "[telegram]\napi_hash = \"x\"\n", "api_id"},
		{"missing api_hash", "[telegram]\napi_id = 1\n", "api_hash"},
		{"interval too small", credentials + "[watch]\npoll_interval = \"10s\"\n", "poll_interval"},
		{"invalid duration", credentials + "[watch]\npoll_interval = \"soon\"\n", "duration"},
		{"page size zero", credentials + "[watch]\npoll_page_size = 0\n", "poll_page_size"},
		{"page size too large", credentials + "[watch]\npoll_page_size = 101\n", "poll_page_size"},
		{"unknown field", credentials + "nope = 1\n", "unknown field"},
		{"unknown nested field", credentials + "[watch]\nnope = 1\n", "unknown field"},
		{"unknown format field", credentials + "[formats.text]\nnope = 1\n", "unknown field"},
		{"unknown default format", "default_format = \"nope\"\n" + credentials, "default_format"},
		{"empty template", credentials + "[formats.empty]\ntemplate = \"\"\n", "template"},
		{"unknown placeholder", credentials + "[formats.bad]\ntemplate = \"{nope}\"\n", "nope"},
		{"bad timezone", credentials + "[formats.bad]\ntemplate = \"{text}\"\ntimezone = \"Mars/Base\"\n", "timezone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateEnv(t)
			_, err := Parse([]byte(tt.doc))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}

func TestLoadExplicitMissingFile(t *testing.T) {
	isolateEnv(t)
	if _, err := Load(filepath.Join(t.TempDir(), "absent.toml")); err == nil {
		t.Fatal("expected error for missing explicit config")
	}
}

func TestLoadDefaultMissingFileUsesDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TG_API_ID", "12345")
	t.Setenv("TG_API_HASH", "hash")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Telegram.APIID != 12345 {
		t.Fatalf("api_id = %d", cfg.Telegram.APIID)
	}
	if _, err := os.Stat(filepath.Join(home, ".tg-watcher")); err == nil {
		t.Fatal("Load unexpectedly created the config directory")
	}
}

func TestLoadFile(t *testing.T) {
	isolateEnv(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(credentials), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Telegram.APIHash != "0123456789abcdef" {
		t.Fatalf("api_hash = %q", cfg.Telegram.APIHash)
	}
}

func TestBuiltinJSONLRendersValidJSON(t *testing.T) {
	isolateEnv(t)
	cfg, err := Parse([]byte(credentials))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tmpl, ok := cfg.Template(BuiltinJSONL)
	if !ok {
		t.Fatal("jsonl template missing")
	}
	line := tmpl.Render(message.Message{
		Date:    time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
		Channel: "ch",
		Sender:  "s",
		Text:    "a\nb \"q\"",
	})
	var decoded map[string]string
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("jsonl line is not valid JSON: %v (%q)", err, line)
	}
	if decoded["text"] != "a\nb \"q\"" {
		t.Fatalf("text = %q", decoded["text"])
	}
}

func TestBuiltinEventRendersValidJSON(t *testing.T) {
	isolateEnv(t)
	cfg, err := Parse([]byte(credentials))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tmpl, ok := cfg.Template(BuiltinEvent)
	if !ok {
		t.Fatal("event template missing")
	}
	date := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	line := tmpl.Render(message.Message{
		ChatID:           -100123,
		MessageID:        42,
		ReplyToMessageID: 41,
		Date:             date,
		Channel:          "ch",
		Sender:           "s",
		Text:             "a\nb \"q\"",
		IsEdit:           true,
		IsPinned:         false,
		IsMention:        true,
		SenderID:         7,
		SenderUsername:   "u",
		SenderIsBot:      true,
		Media:            []message.MediaRef{{Kind: message.MediaPhoto, FileID: 9}},
		Forward:          &message.Forward{FromName: "f"},
	})

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("event line is not valid JSON: %v (%q)", err, line)
	}
	for _, key := range []string{
		"chat_id", "message_id", "reply_to_message_id", "datetime", "date_unix",
		"channel", "sender", "sender_id", "sender_username", "sender_is_bot",
		"text", "is_edit", "is_pinned", "is_mention", "media", "forward",
	} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("event JSON is missing %q: %s", key, line)
		}
	}
	if string(decoded["chat_id"]) != "-100123" {
		t.Fatalf("chat_id = %s", decoded["chat_id"])
	}
	if string(decoded["date_unix"]) != "1795435200" {
		t.Fatalf("date_unix = %s", decoded["date_unix"])
	}
	if string(decoded["is_edit"]) != "true" || string(decoded["is_pinned"]) != "false" {
		t.Fatalf("flags = %s/%s", decoded["is_edit"], decoded["is_pinned"])
	}
	if string(decoded["media"]) != `[{"kind":"photo","file_id":9}]` {
		t.Fatalf("media = %s", decoded["media"])
	}
	if string(decoded["forward"]) != `{"from_name":"f"}` {
		t.Fatalf("forward = %s", decoded["forward"])
	}
}

func TestFormatNamePriority(t *testing.T) {
	isolateEnv(t)
	cfg, err := Parse([]byte("default_format = \"jsonl\"\n" + credentials))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := cfg.FormatName("text"); got != "text" {
		t.Fatalf("explicit format = %q", got)
	}
	if got := cfg.FormatName(""); got != "jsonl" {
		t.Fatalf("default format = %q", got)
	}
	cfg.DefaultFormat = ""
	if got := cfg.FormatName(""); got != BuiltinText {
		t.Fatalf("fallback format = %q", got)
	}
}
