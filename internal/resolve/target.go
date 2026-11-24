// Package resolve parses chat identifiers (numeric IDs, @username, t.me links)
// without depending on gotd. Actual Telegram API calls are in internal/telegram.
// See plan §8.1.
package resolve

import (
	"fmt"
	"strconv"
	"strings"
)

// TargetKind indicates the type of a chat identifier.
type TargetKind int

const (
	TargetNumeric  TargetKind = iota // -1001234567890 or 123
	TargetUsername                   // @channel
	TargetTmeLink                    // https://t.me/channel
	TargetTmeCLink                   // t.me/c/12345
)

// Target is a parsed chat identifier before resolution.
type Target struct {
	Original string
	Kind     TargetKind
	// NumericID is set for Numeric and TmeCLink kinds.
	NumericID int64
	// Username is set for Username and TmeLink kinds.
	Username string
}

// ParseTarget parses a single chat identifier. It returns an error for
// unsupported link types (invite links, joinchat).
func ParseTarget(s string) (Target, error) {
	orig := strings.TrimSpace(s)
	if orig == "" {
		return Target{}, fmt.Errorf("empty chat identifier")
	}

	// Numeric ID.
	if id, err := strconv.ParseInt(orig, 10, 64); err == nil {
		return Target{Original: orig, Kind: TargetNumeric, NumericID: id}, nil
	}

	// @username.
	if strings.HasPrefix(orig, "@") {
		username := orig[1:]
		if username == "" {
			return Target{}, fmt.Errorf("empty username after @")
		}
		return Target{Original: orig, Kind: TargetUsername, Username: username}, nil
	}

	// t.me or https://t.me (or tg://)
	if strings.HasPrefix(orig, "https://t.me/") || strings.HasPrefix(orig, "t.me/") {
		path := strings.TrimPrefix(orig, "https://t.me/")
		path = strings.TrimPrefix(path, "t.me/")

		// t.me/c/<id>
		if strings.HasPrefix(path, "c/") {
			idStr := path[2:]
			// Remove query/slash parts.
			if idx := strings.IndexAny(idStr, "?/#"); idx >= 0 {
				idStr = idStr[:idx]
			}
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				return Target{}, fmt.Errorf("invalid t.me/c/… ID %q", idStr)
			}
			return Target{Original: orig, Kind: TargetTmeCLink, NumericID: id}, nil
		}

		// Invite links: t.me/+<hash> or t.me/joinchat/<hash>
		if strings.HasPrefix(path, "+") || strings.HasPrefix(path, "joinchat/") {
			return Target{}, fmt.Errorf("invite links (%s) are not supported: add the chat via @username or numeric ID instead", orig)
		}

		// t.me/message?single … strip query/slash
		if idx := strings.IndexAny(path, "?/#"); idx >= 0 {
			path = path[:idx]
		}
		if path == "" {
			return Target{}, fmt.Errorf("empty username in t.me link %q", orig)
		}
		return Target{Original: orig, Kind: TargetTmeLink, Username: path}, nil
	}

	return Target{}, fmt.Errorf("unsupported chat identifier %q; use numeric ID, @username or t.me link", orig)
}

// ParseTargets parses a list of chat identifiers, deduplicating by the original
// string. It returns all errors and may return partial valid targets.
func ParseTargets(raw []string) ([]Target, []error) {
	var targets []Target
	var errs []error
	seen := make(map[string]bool, len(raw))
	for _, s := range raw {
		if seen[s] {
			continue
		}
		seen[s] = true
		t, err := ParseTarget(s)
		if err != nil {
			errs = append(errs, err)
		} else {
			targets = append(targets, t)
		}
	}
	return targets, errs
}