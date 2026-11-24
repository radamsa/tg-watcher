package cli

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseBasic(t *testing.T) {
	opts, err := Parse([]string{"+31612345678", "@channel"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := Options{Phone: "+31612345678", Chats: []string{"@channel"}}
	if !reflect.DeepEqual(opts, want) {
		t.Fatalf("opts = %#v, want %#v", opts, want)
	}
}

func TestParseFlagsAnywhere(t *testing.T) {
	opts, err := Parse([]string{
		"--format", "jsonl",
		"+31612345678",
		"--config=/tmp/cfg.toml",
		"@a",
		"--password", "secret",
		"https://t.me/b",
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if opts.Format != "jsonl" || opts.ConfigPath != "/tmp/cfg.toml" || opts.Password != "secret" {
		t.Fatalf("flags not parsed: %#v", opts)
	}
	if got, want := opts.Chats, []string{"@a", "https://t.me/b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("chats = %#v, want %#v", got, want)
	}
}

func TestParseNegativeChatID(t *testing.T) {
	// -100… IDs must be positionals, including directly after the phone and
	// after flag values.
	for _, args := range [][]string{
		{"+31612345678", "-1001234567890"},
		{"+31612345678", "--format", "text", "-1001234567890"},
	} {
		opts, err := Parse(args)
		if err != nil {
			t.Fatalf("Parse(%q): %v", args, err)
		}
		if len(opts.Chats) != 1 || opts.Chats[0] != "-1001234567890" {
			t.Fatalf("Parse(%q) chats = %#v", args, opts.Chats)
		}
	}
}

func TestParseDoubleDashTerminatesFlags(t *testing.T) {
	opts, err := Parse([]string{"+31612345678", "--", "--format"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := opts.Chats, []string{"--format"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("chats = %#v, want %#v", got, want)
	}
	if opts.Format != "" {
		t.Fatalf("format = %q, want empty", opts.Format)
	}
}

func TestParseHelpAndVersion(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}} {
		opts, err := Parse(args)
		if err != nil {
			t.Fatalf("Parse(%q): %v", args, err)
		}
		if !opts.ShowHelp && !opts.ShowVersion {
			t.Fatalf("Parse(%q) did not request help/version", args)
		}
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"no chats", []string{"+31612345678"}},
		{"no phone only chats", []string{"@channel"}},
		{"invalid phone", []string{"31612345678", "@a"}},
		{"too short phone", []string{"+1234", "@a"}},
		{"unknown flag", []string{"--nope", "+31612345678", "@a"}},
		{"missing flag value", []string{"+31612345678", "--format"}},
		{"empty flag value", []string{"+31612345678", "--config=", "@a"}},
		{"version with value", []string{"--version=1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.args)
			if err == nil {
				t.Fatal("expected error")
			}
			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("error %T is not *UsageError: %v", err, err)
			}
		})
	}
}
