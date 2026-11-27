// Package format compiles and renders the single universal template engine used
// for all output formats. See plan §7.
package format

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"tg-watcher/internal/message"
)

// Field names accepted in templates.
const (
	FieldDatetime         = "datetime"
	FieldChannel          = "channel"
	FieldSender           = "sender"
	FieldText             = "text"
	FieldChatID           = "chat_id"
	FieldMessageID        = "message_id"
	FieldReplyToMessageID = "reply_to_message_id"
	FieldDateUnix         = "date_unix"
	FieldIsEdit           = "is_edit"
	FieldIsPinned         = "is_pinned"
	FieldIsMention        = "is_mention"
	FieldSenderID         = "sender_id"
	FieldSenderUsername   = "sender_username"
	FieldSenderIsBot      = "sender_is_bot"
	FieldMedia            = "media"
	FieldForward          = "forward"
)

// FilterJSON encodes a value as a complete JSON string, including quotes.
const FilterJSON = "json"

// Spec describes one named template from the config.
type Spec struct {
	// Template is the raw template text.
	Template string
	// DatetimeFormat is a Go time layout used for the datetime field.
	DatetimeFormat string
	// Location is the timezone applied to the datetime field.
	Location *time.Location
	// MultilineIndent is the number of spaces used to indent continuation
	// lines of the text field. Ignored when the json filter is used.
	MultilineIndent int
}

type segmentKind int

const (
	segLiteral segmentKind = iota
	segField
)

type segment struct {
	kind   segmentKind
	text   string // literal contents
	field  string
	asJSON bool
}

// Template is a compiled template. It is immutable and safe for concurrent use.
type Template struct {
	segments []segment
	spec     Spec
}

// Compile parses and validates a template. It is called once at config load
// time; rendering never re-parses the template.
func Compile(spec Spec) (*Template, error) {
	if strings.TrimSpace(spec.Template) == "" {
		return nil, fmt.Errorf("template is empty")
	}
	if spec.Location == nil {
		spec.Location = time.Local
	}
	if spec.MultilineIndent < 0 {
		return nil, fmt.Errorf("multiline_indent must not be negative")
	}

	t := &Template{spec: spec}
	lit := strings.Builder{}

	flush := func() {
		if lit.Len() > 0 {
			t.segments = append(t.segments, segment{kind: segLiteral, text: lit.String()})
			lit.Reset()
		}
	}

	for i := 0; i < len(spec.Template); {
		c := spec.Template[i]
		switch c {
		case '{':
			if i+1 < len(spec.Template) && spec.Template[i+1] == '{' {
				lit.WriteByte('{')
				i += 2
				continue
			}
			if end, ok := placeholderEnd(spec.Template, i); ok {
				body := spec.Template[i+1 : end]
				seg, err := parsePlaceholder(body)
				if err != nil {
					return nil, err
				}
				flush()
				t.segments = append(t.segments, seg)
				i = end + 1
				continue
			}
			// A '{' that starts something identifier-like but never closes is
			// a malformed placeholder. Other braces (e.g. JSON object syntax)
			// are literal.
			if looksLikePlaceholderStart(spec.Template[i+1:]) {
				return nil, fmt.Errorf("unclosed placeholder at offset %d", i)
			}
			lit.WriteByte('{')
			i++
		case '}':
			if i+1 < len(spec.Template) && spec.Template[i+1] == '}' {
				lit.WriteByte('}')
				i += 2
				continue
			}
			lit.WriteByte('}')
			i++
		default:
			lit.WriteByte(c)
			i++
		}
	}
	flush()
	return t, nil
}

// placeholderEnd returns the index of the '}' closing a placeholder that starts
// at start, if the enclosed text looks like a placeholder body. JSON object
// braces such as {"a":1} are left as literals.
func placeholderEnd(s string, start int) (end int, ok bool) {
	rel := strings.IndexByte(s[start+1:], '}')
	if rel < 0 {
		return 0, false
	}
	end = start + 1 + rel
	if !looksLikePlaceholderBody(s[start+1 : end]) {
		return 0, false
	}
	return end, true
}

// looksLikePlaceholderBody reports whether body consists only of identifier
// characters, spaces and '|', which distinguishes placeholders from JSON
// punctuation.
func looksLikePlaceholderBody(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_', c >= '0' && c <= '9', c == '|', c == ' ':
		default:
			return false
		}
	}
	return isIdentStart(body[0])
}

// looksLikePlaceholderStart reports whether s begins (after optional spaces)
// with an identifier start, i.e. the beginning of a placeholder.
func looksLikePlaceholderStart(s string) bool {
	s = strings.TrimLeft(s, " ")
	return s != "" && isIdentStart(s[0])
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func parsePlaceholder(body string) (segment, error) {
	parts := strings.Split(body, "|")
	if len(parts) > 2 {
		return segment{}, fmt.Errorf("placeholder %q has more than one filter", body)
	}

	field := strings.TrimSpace(parts[0])
	switch field {
	case FieldDatetime, FieldChannel, FieldSender, FieldText,
		FieldChatID, FieldMessageID, FieldReplyToMessageID, FieldDateUnix,
		FieldIsEdit, FieldIsPinned, FieldIsMention,
		FieldSenderID, FieldSenderUsername, FieldSenderIsBot,
		FieldMedia, FieldForward:
	default:
		if field == "" {
			return segment{}, fmt.Errorf("empty placeholder")
		}
		return segment{}, fmt.Errorf("unknown placeholder %q", field)
	}

	seg := segment{kind: segField, field: field}
	if len(parts) == 2 {
		filter := strings.TrimSpace(parts[1])
		if filter != FilterJSON {
			return segment{}, fmt.Errorf("unknown filter %q in placeholder %q", filter, body)
		}
		seg.asJSON = true
	}
	return seg, nil
}

// Render formats one message. The result never includes a trailing newline;
// the output writer adds record separation.
func (t *Template) Render(m message.Message) string {
	var b strings.Builder
	for _, seg := range t.segments {
		if seg.kind == segLiteral {
			b.WriteString(seg.text)
			continue
		}
		b.WriteString(t.renderField(seg, m))
	}
	return b.String()
}

func (t *Template) renderField(seg segment, m message.Message) string {
	var value any
	switch seg.field {
	case FieldDatetime:
		value = m.Date.In(t.spec.Location).Format(t.spec.DatetimeFormat)
	case FieldChannel:
		value = m.Channel
	case FieldSender:
		value = m.Sender
	case FieldText:
		value = m.Text
		if !seg.asJSON {
			value = indentLines(m.Text, t.spec.MultilineIndent)
		}
	case FieldChatID:
		value = m.ChatID
	case FieldMessageID:
		value = m.MessageID
	case FieldReplyToMessageID:
		value = m.ReplyToMessageID
	case FieldDateUnix:
		value = m.Date.Unix()
	case FieldIsEdit:
		value = m.IsEdit
	case FieldIsPinned:
		value = m.IsPinned
	case FieldIsMention:
		value = m.IsMention
	case FieldSenderID:
		value = m.SenderID
	case FieldSenderUsername:
		value = m.SenderUsername
	case FieldSenderIsBot:
		value = m.SenderIsBot
	case FieldMedia:
		if m.Media == nil {
			value = []message.MediaRef{}
		} else {
			value = m.Media
		}
	case FieldForward:
		value = m.Forward
	}

	if seg.asJSON {
		encoded, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	// Structured values stay machine-readable even without the explicit
	// filter; scalar values use their natural textual form.
	switch seg.field {
	case FieldMedia, FieldForward:
		encoded, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
	return fmt.Sprint(value)
}

func indentLines(s string, spaces int) string {
	if spaces <= 0 || !strings.Contains(s, "\n") {
		return s
	}
	pad := "\n" + strings.Repeat(" ", spaces)
	return strings.ReplaceAll(s, "\n", pad)
}
