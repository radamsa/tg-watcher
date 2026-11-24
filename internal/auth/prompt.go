package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// PasswordEnv is the environment variable holding the 2FA password.
const PasswordEnv = "TG_PASSWORD"

// Prompter supplies interactive authentication input.
type Prompter interface {
	// Code returns the confirmation code sent by Telegram.
	Code(ctx context.Context) (string, error)
	// Password returns the cloud 2FA password.
	Password(ctx context.Context) (string, error)
}

// Terminal abstracts interactive input so the password/code priority rules can
// be tested without a real TTY.
type Terminal interface {
	IsTerminal() bool
	// ReadLine reads an echoed line (confirmation code).
	ReadLine(prompt string) (string, error)
	// ReadSecret reads a line without echo (2FA password).
	ReadSecret(prompt string) (string, error)
}

// Source resolves authentication input according to plan §4/§5:
//
//	password: --password, then TG_PASSWORD, then a no-echo terminal prompt;
//	code:     terminal prompt only.
type Source struct {
	PasswordFlag string
	Getenv       func(string) (string, bool)
	Terminal     Terminal
}

// NewSource builds a Source backed by the process environment and the given
// terminal (which may be nil when stdin is not a TTY).
func NewSource(passwordFlag string, tty Terminal) *Source {
	return &Source{
		PasswordFlag: passwordFlag,
		Getenv:       os.LookupEnv,
		Terminal:     tty,
	}
}

// Code prompts for the Telegram confirmation code.
func (s *Source) Code(context.Context) (string, error) {
	if s.Terminal == nil || !s.Terminal.IsTerminal() {
		return "", ErrInteractiveNeeded
	}
	code, err := s.Terminal.ReadLine("Enter the Telegram confirmation code: ")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(code) == "" {
		return "", errors.New("empty confirmation code")
	}
	return strings.TrimSpace(code), nil
}

// Password resolves the 2FA password. It only prompts when Telegram actually
// requests a password, so accounts without 2FA never see a prompt.
func (s *Source) Password(context.Context) (string, error) {
	if s.PasswordFlag != "" {
		return s.PasswordFlag, nil
	}
	if s.Getenv != nil {
		if v, ok := s.Getenv(PasswordEnv); ok && v != "" {
			return v, nil
		}
	}
	if s.Terminal == nil || !s.Terminal.IsTerminal() {
		return "", ErrPasswordRequired
	}
	password, err := s.Terminal.ReadSecret("Enter your 2FA password: ")
	if err != nil {
		return "", err
	}
	if password == "" {
		return "", ErrPasswordRequired
	}
	return password, nil
}

// Console is the production Terminal backed by stdin/stdout.
type Console struct {
	in  *bufio.Reader
	out io.Writer
	fd  int
}

// NewConsole creates a Console. fd is the file descriptor used for no-echo
// reads; pass -1 when no TTY is available.
func NewConsole(in io.Reader, out io.Writer, fd int) *Console {
	return &Console{in: bufio.NewReader(in), out: out, fd: fd}
}

// IsTerminal reports whether the input is an interactive terminal.
func (c *Console) IsTerminal() bool {
	return c.fd >= 0 && term.IsTerminal(c.fd)
}

// ReadLine prints prompt and reads one line, trimming surrounding whitespace.
func (c *Console) ReadLine(prompt string) (string, error) {
	fmt.Fprint(c.out, prompt)
	line, err := c.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// ReadSecret prints prompt and reads a line with echo disabled.
func (c *Console) ReadSecret(prompt string) (string, error) {
	if c.fd < 0 {
		return "", ErrInteractiveNeeded
	}
	fmt.Fprint(c.out, prompt)
	data, err := term.ReadPassword(c.fd)
	fmt.Fprintln(c.out)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
