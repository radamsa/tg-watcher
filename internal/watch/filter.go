package watch

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"tg-watcher/internal/message"
)

// FilterOptions configures the engine-level focus filters. The zero value
// matches every message.
type FilterOptions struct {
	// MentionsOnly keeps only messages that mention the authenticated account.
	MentionsOnly bool
	// From keeps messages whose sender matches any of these values. A value is
	// a numeric user ID, an @username, or a case-insensitive substring of the
	// display name.
	From []string
	// Match keeps messages whose text matches at least one pattern. With All
	// set, every pattern must match instead.
	Match []string
	// All switches Match from "any" to "all".
	All bool
}

// Filter is an immutable compiled filter. A nil *Filter matches everything.
type Filter struct {
	mentionsOnly bool
	from         []string
	patterns     []*regexp.Regexp
	all          bool
}

// NewFilter compiles the regular expressions in opts. Invalid patterns are
// reported as argument errors (exit code 2).
func NewFilter(opts FilterOptions) (*Filter, error) {
	f := &Filter{
		mentionsOnly: opts.MentionsOnly,
		from:         append([]string(nil), opts.From...),
		all:          opts.All,
	}
	for _, pattern := range opts.Match {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid --match pattern %q: %w", pattern, err)
		}
		f.patterns = append(f.patterns, re)
	}
	return f, nil
}

// Match reports whether m passes every active filter.
func (f *Filter) Match(m message.Message) bool {
	if f == nil {
		return true
	}
	if f.mentionsOnly && !m.IsMention {
		return false
	}
	if len(f.from) > 0 && !matchesAnySender(m, f.from) {
		return false
	}
	if len(f.patterns) == 0 {
		return true
	}
	if f.all {
		for _, re := range f.patterns {
			if !re.MatchString(m.Text) {
				return false
			}
		}
		return true
	}
	for _, re := range f.patterns {
		if re.MatchString(m.Text) {
			return true
		}
	}
	return false
}

func matchesAnySender(m message.Message, values []string) bool {
	for _, value := range values {
		if matchSender(m, value) {
			return true
		}
	}
	return false
}

func matchSender(m message.Message, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if id, err := strconv.ParseInt(value, 10, 64); err == nil {
		return m.SenderID == id
	}
	if strings.HasPrefix(value, "@") {
		username := value[1:]
		if m.SenderUsername != "" {
			return strings.EqualFold(m.SenderUsername, username)
		}
		return strings.Contains(strings.ToLower(m.Sender), strings.ToLower(value))
	}
	return strings.Contains(strings.ToLower(m.Sender), strings.ToLower(value))
}
