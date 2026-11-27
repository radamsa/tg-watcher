// Package cli parses the tg-watcher command line and defines process exit codes.
package cli

import (
	"fmt"
	"strings"
)

// Exit codes as defined by the project specification (plan §4.1).
const (
	ExitOK      = 0
	ExitRuntime = 1
	ExitUsage   = 2
	ExitAuth    = 3
	ExitNoChats = 4
)

// Usage is the short synopsis shown on argument errors.
const Usage = "usage: tg-watcher <phone> [--format NAME] [--config PATH] [--password PWD] " +
	"[--mentions-only] [--from SENDER] [--match REGEX] [--all] [--template-per-chat] " +
	"[--version] [--help] [--] <chat>..."

// Help is the full help text.
const Help = Usage + `

tg-watcher watches new messages in the given Telegram channels and groups and
prints them to stdout. Diagnostics, warnings and errors go to stderr.

Arguments:
  <phone>           Phone number in international format, e.g. +31612345678.
  <chat>            One or more targets: numeric ID, @username or t.me link.
                    Use -- to stop flag parsing, so IDs such as -1001234567890
                    are accepted as targets.

Flags:
  --format NAME     Named output template from the config (default: config
                    default_format, then the built-in "text").
  --config PATH     TOML config file (default: ~/.tg-watcher/config.toml).
  --password PWD    Cloud 2FA password. Prefer TG_PASSWORD or the interactive
                    prompt; a command-line password is visible to other users.
  --mentions-only   Only print messages that mention your account.
  --from SENDER     Only print messages from a sender. SENDER is a numeric
                    user ID, @username, or part of the display name. May be
                    repeated; a message matching any value is printed.
  --match REGEX     Only print messages whose text matches REGEX. May be
                    repeated; by default any pattern is enough.
  --all             With multiple --match patterns, require every pattern to
                    match instead of any.
  --template-per-chat
                    Select output templates from [chat_formats] in the config.
  --version         Print the version and exit.
  --help            Print this help and exit.`

// Options is the result of parsing the command line.
type Options struct {
	Phone           string
	Chats           []string
	Format          string
	ConfigPath      string
	Password        string
	MentionsOnly    bool
	From            []string
	Match           []string
	All             bool
	TemplatePerChat bool
	ShowVersion     bool
	ShowHelp        bool
}

// UsageError marks an argument error that must exit with code 2.
type UsageError struct {
	Message string
}

func (e *UsageError) Error() string { return e.Message }

// Parse parses command-line arguments (without the program name). It never
// touches the environment, the filesystem or the network.
func Parse(args []string) (Options, error) {
	var (
		opts       Options
		positional []string
		endFlags   bool
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if endFlags {
			positional = append(positional, arg)
			continue
		}
		if arg == "--" {
			endFlags = true
			continue
		}
		// Negative numeric IDs such as -1001234567890 must be treated as
		// chat identifiers, not as flags.
		if isNegativeChatID(arg) {
			positional = append(positional, arg)
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			name, value, hasValue := splitFlag(arg)
			switch name {
			case "--format", "--config", "--password", "--from", "--match":
				if !hasValue {
					i++
					if i >= len(args) {
						return Options{}, &UsageError{Message: fmt.Sprintf("flag %s requires a value", name)}
					}
					value = args[i]
				}
				if value == "" {
					return Options{}, &UsageError{Message: fmt.Sprintf("flag %s requires a non-empty value", name)}
				}
				switch name {
				case "--format":
					opts.Format = value
				case "--config":
					opts.ConfigPath = value
				case "--password":
					opts.Password = value
				case "--from":
					opts.From = append(opts.From, value)
				case "--match":
					opts.Match = append(opts.Match, value)
				}
			case "--mentions-only", "--all", "--template-per-chat":
				if hasValue {
					return Options{}, &UsageError{Message: fmt.Sprintf("flag %s does not take a value", name)}
				}
				switch name {
				case "--mentions-only":
					opts.MentionsOnly = true
				case "--all":
					opts.All = true
				case "--template-per-chat":
					opts.TemplatePerChat = true
				}
			case "--version":
				if hasValue {
					return Options{}, &UsageError{Message: "flag --version does not take a value"}
				}
				opts.ShowVersion = true
			case "--help", "-h":
				if hasValue {
					return Options{}, &UsageError{Message: fmt.Sprintf("flag %s does not take a value", name)}
				}
				opts.ShowHelp = true
			default:
				return Options{}, &UsageError{Message: fmt.Sprintf("unknown flag %s", name)}
			}
			continue
		}

		positional = append(positional, arg)
	}

	if opts.ShowHelp || opts.ShowVersion {
		return opts, nil
	}

	if len(positional) == 0 {
		return Options{}, &UsageError{Message: "missing phone number"}
	}
	opts.Phone = positional[0]
	if !validPhone(opts.Phone) {
		return Options{}, &UsageError{Message: fmt.Sprintf("invalid phone number %q: use international format such as +31612345678", opts.Phone)}
	}

	opts.Chats = positional[1:]
	if len(opts.Chats) == 0 {
		return Options{}, &UsageError{Message: "no chat specified: pass at least one chat ID, @username or t.me link"}
	}

	return opts, nil
}

// splitFlag splits "--name=value" into its parts. A flag without '=' returns
// hasValue=false.
func splitFlag(arg string) (name, value string, hasValue bool) {
	if idx := strings.IndexByte(arg, '='); idx >= 0 {
		return arg[:idx], arg[idx+1:], true
	}
	return arg, "", false
}

// isNegativeChatID reports whether arg is a negative decimal chat ID, e.g.
// -1001234567890.
func isNegativeChatID(arg string) bool {
	if len(arg) < 2 || arg[0] != '-' {
		return false
	}
	for i := 1; i < len(arg); i++ {
		if arg[i] < '0' || arg[i] > '9' {
			return false
		}
	}
	return true
}

// validPhone performs a lenient check of an international phone number: a
// leading '+' followed by 5 to 15 digits.
func validPhone(s string) bool {
	if len(s) < 6 || s[0] != '+' {
		return false
	}
	digits := s[1:]
	if len(digits) < 5 || len(digits) > 15 {
		return false
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return false
		}
	}
	return true
}
