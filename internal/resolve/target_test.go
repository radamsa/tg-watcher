package resolve

import (
	"testing"
)

func atoms(t Target) TargetKind {
	return t.Kind
}

func TestParseTarget(t *testing.T) {
	tests := []struct {
		input string
		kind  TargetKind
		id    int64
		user  string
	}{
		{"123", TargetNumeric, 123, ""},
		{"-1001234567890", TargetNumeric, -1001234567890, ""},
		{"-100", TargetNumeric, -100, ""},
		{"0", TargetNumeric, 0, ""},
		{"@channel", TargetUsername, 0, "channel"},
		{"@", TargetUsername, 0, ""},               // edge: known ok parse
		{"https://t.me/channel", TargetTmeLink, 0, "channel"},
		{"https://t.me/channel?start=1", TargetTmeLink, 0, "channel"},
		{"t.me/channel", TargetTmeLink, 0, "channel"},
		{"https://t.me/c/12345", TargetTmeCLink, 12345, ""},
		{"https://t.me/c/-123", TargetTmeCLink, -123, ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseTarget(tt.input)
			if err != nil {
				t.Fatalf("ParseTarget(%q): %v", tt.input, err)
			}
			if got.Kind != tt.kind || got.NumericID != tt.id || got.Username != tt.user {
				t.Fatalf("ParseTarget(%q) = %#v, want kind=%d id=%d user=%q", tt.input, got, tt.kind, tt.id, tt.user)
			}
		})
	}
}

func TestParseTargetErrors(t *testing.T) {
	inputs := []string{
		"",
		" ",
		"https://t.me/+secret",
		"t.me/joinchat/abc",
		"https://t.me/+",
		"tg://example",
	}
	for _, s := range inputs {
		t.Run(s, func(t *testing.T) {
			_, err := ParseTarget(s)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseTargets(t *testing.T) {
	targets, errs := ParseTargets([]string{"@a", "123", "@a"})
	if len(targets) != 2 || len(errs) != 0 {
		t.Fatalf("targets=%v errs=%v", targets, errs)
	}
}

func TestUnknownFormat(t *testing.T) {
	if _, err := ParseTarget("random"); err == nil {
		t.Fatal("expected error")
	}
}