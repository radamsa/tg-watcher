package format

import "tg-watcher/internal/message"

// PerChat selects a template by chat ID and falls back to Default. It is used
// by --template-per-chat; the maps are built once at startup and are read-only
// afterwards.
type PerChat struct {
	Default  *Template
	ByChatID map[int64]*Template
}

// NewPerChat creates a per-chat renderer. A nil byChatID map is allowed.
func NewPerChat(def *Template, byChatID map[int64]*Template) *PerChat {
	return &PerChat{Default: def, ByChatID: byChatID}
}

// Render formats m with the template registered for its chat, if any.
func (p *PerChat) Render(m message.Message) string {
	if p == nil || p.Default == nil {
		return ""
	}
	if tmpl := p.ByChatID[m.ChatID]; tmpl != nil {
		return tmpl.Render(m)
	}
	return p.Default.Render(m)
}
