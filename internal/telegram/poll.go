package telegram

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gotd/td/tg"

	"tg-watcher/internal/message"
	"tg-watcher/internal/watch"
)

// Poller periodically fetches messages from watched chats using
// messages.getHistory with offset_id = cursor (plan §9.9).
type Poller struct {
	client     *Client
	state      map[int64]*watch.ChatState
	engine     *watch.Engine
	interval   time.Duration
	pageSize   int
	userLib    map[int64]*tg.User
	chatTitles map[int64]string
}

// NewPoller creates a Poller. interval is clamped to >= 30s.
func NewPoller(client *Client, engine *watch.Engine, states map[int64]*watch.ChatState, interval time.Duration, pageSize int) *Poller {
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}
	return &Poller{
		client:     client,
		state:      states,
		engine:     engine,
		interval:   interval,
		pageSize:   pageSize,
		userLib:    make(map[int64]*tg.User),
		chatTitles: make(map[int64]string),
	}
}

// Run starts the polling loop. It runs until ctx is cancelled. Each cycle
// polls every chat that is not in backoff.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pollAll(ctx)
		}
	}
}

func (p *Poller) pollAll(ctx context.Context) {
	for chatID, st := range p.state {
		if !p.backoffOK(st) {
			continue
		}
		p.pollOne(ctx, chatID, st)
	}
}

func (p *Poller) backoffOK(st *watch.ChatState) bool {
	return time.Now().After(st.BackoffUntil)
}

func (p *Poller) pollOne(ctx context.Context, chatID int64, st *watch.ChatState) {
	messages, err := p.client.GetChatHistory(ctx, chatID, st.Info.AccessHash, p.pageSize, st.CursorID)
	if err != nil {
		st.PollFailures++
		backoff := minDuration(30*time.Second<<uint(st.PollFailures-1), 5*time.Minute)
		st.BackoffUntil = time.Now().Add(backoff)
		return
	}
	st.PollFailures = 0
	st.LastPoll = time.Now()
	if len(messages) == 0 {
		return
	}

	// Sort ascending by ID for deterministic output.
	sort.Slice(messages, func(i, j int) bool {
		return extractID(messages[i]) < extractID(messages[j])
	})

	for _, mc := range messages {
		msg, ok := mc.(*tg.Message)
		if !ok {
			continue
		}
		// Skip our own outgoing messages and messages before baseline.
		if msg.Out {
			continue
		}
		if msg.ID <= st.BaselineID {
			continue
		}
		normalized := p.normalize(msg, chatID)
		if err := p.engine.Ingest(ctx, chatID, normalized); err != nil {
			return // ctx cancelled
		}
	}
}

func (p *Poller) normalize(m *tg.Message, chatID int64) message.Message {
	title := p.chatTitles[chatID]
	if title == "" {
		if st, ok := p.state[chatID]; ok {
			title = st.Info.Title
			p.chatTitles[chatID] = title
		}
	}
	return normalizeMessage(m, p.userLib, title)
}

func extractID(mc tg.MessageClass) int {
	if m, ok := mc.(*tg.Message); ok {
		return m.ID
	}
	return 0
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// FetchBaseline fetches the latest message from a chat and returns its message
// ID as the baseline. Called once per chat before READY (plan §9.5).
func (c *Client) FetchBaseline(ctx context.Context, chatID, accessHash int64) (int, error) {
	msgs, err := c.GetChatHistory(ctx, chatID, accessHash, 1, 0)
	if err != nil {
		return 0, fmt.Errorf("fetch baseline for chat %d: %w", chatID, err)
	}
	if len(msgs) == 0 {
		return 0, nil // chat is empty; baseline is 0
	}
	m, ok := msgs[0].(*tg.Message)
	if !ok {
		return 0, nil // non-message entry, treat as empty
	}
	return m.ID, nil
}