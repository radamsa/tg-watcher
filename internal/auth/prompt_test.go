package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeTerminal struct {
	terminal bool
	lines    []string
	secrets  []string
	readErr  error
}

func (f *fakeTerminal) IsTerminal() bool { return f.terminal }

func (f *fakeTerminal) ReadLine(string) (string, error) {
	if f.readErr != nil {
		return "", f.readErr
	}
	if len(f.lines) == 0 {
		return "", errors.New("no line queued")
	}
	line := f.lines[0]
	f.lines = f.lines[1:]
	return line, nil
}

func (f *fakeTerminal) ReadSecret(string) (string, error) {
	if f.readErr != nil {
		return "", f.readErr
	}
	if len(f.secrets) == 0 {
		return "", errors.New("no secret queued")
	}
	secret := f.secrets[0]
	f.secrets = f.secrets[1:]
	return secret, nil
}

func env(values map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := values[k]
		return v, ok
	}
}

func TestPasswordPriority(t *testing.T) {
	term := &fakeTerminal{terminal: true, secrets: []string{"from-terminal"}}
	src := &Source{
		PasswordFlag: "from-flag",
		Getenv:       env(map[string]string{PasswordEnv: "from-env"}),
		Terminal:     term,
	}
	if got, err := src.Password(context.Background()); err != nil || got != "from-flag" {
		t.Fatalf("flag priority: %q, %v", got, err)
	}

	src.PasswordFlag = ""
	if got, err := src.Password(context.Background()); err != nil || got != "from-env" {
		t.Fatalf("env priority: %q, %v", got, err)
	}

	src.Getenv = env(nil)
	if got, err := src.Password(context.Background()); err != nil || got != "from-terminal" {
		t.Fatalf("terminal fallback: %q, %v", got, err)
	}
}

func TestPasswordWithoutTerminal(t *testing.T) {
	src := &Source{Getenv: env(nil), Terminal: &fakeTerminal{terminal: false}}
	if _, err := src.Password(context.Background()); !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("err = %v, want ErrPasswordRequired", err)
	}
}

func TestCodeRequiresTerminal(t *testing.T) {
	src := &Source{Terminal: &fakeTerminal{terminal: false}}
	if _, err := src.Code(context.Background()); !errors.Is(err, ErrInteractiveNeeded) {
		t.Fatalf("err = %v, want ErrInteractiveNeeded", err)
	}

	term := &fakeTerminal{terminal: true, lines: []string{" 12345 "}}
	src.Terminal = term
	got, err := src.Code(context.Background())
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if got != "12345" {
		t.Fatalf("code = %q", got)
	}
}

func TestCodeRejectsEmpty(t *testing.T) {
	src := &Source{Terminal: &fakeTerminal{terminal: true, lines: []string{"  "}}}
	if _, err := src.Code(context.Background()); err == nil {
		t.Fatal("expected error for empty code")
	}
}

func TestConsoleReadLine(t *testing.T) {
	var out strings.Builder
	c := NewConsole(strings.NewReader("hello\n"), &out, -1)
	if c.IsTerminal() {
		t.Fatal("fd=-1 must not be a terminal")
	}
	got, err := c.ReadLine("prompt: ")
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "hello" || out.String() != "prompt: " {
		t.Fatalf("got %q, out %q", got, out.String())
	}
	if _, err := c.ReadSecret("pw: "); !errors.Is(err, ErrInteractiveNeeded) {
		t.Fatalf("ReadSecret err = %v", err)
	}
}
