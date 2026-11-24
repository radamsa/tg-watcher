// Package watch orchestrates live message monitoring: update processing,
// periodic polling, backfill, deduplication and ordered output.
// See plan §9.
package watch

import (
	"time"

	"tg-watcher/internal/message"
	"tg-watcher/internal/resolve"
)

// ChatState holds the in-memory monitoring state for one target chat.
// All state is lost on restart; only the auth session file persists.
type ChatState struct {
	Info resolve.ChatInfo

	// BaselineID is the message ID obtained from messages.getHistory(limit=1)
	// at startup. Messages with ID <= BaselineID are never printed.
	BaselineID int

	// CursorID is the highest message ID processed (either from updates or
	// from the poller). Used as min_id in the next poll request.
	CursorID int

	// Dedup tracks already-seen (chatID, messageID) pairs within this run.
	// Entries are dropped after dedupRetention to limit memory.
	Dedup map[int64]bool // messageID -> seen

	// LastPoll is the timestamp of the last successful poll cycle.
	LastPoll      time.Time
	BackoffUntil  time.Time // don't poll before this time
	PollFailures  int
}

// NewChatState creates a fresh state for one chat.
func NewChatState(info resolve.ChatInfo, baselineID int) *ChatState {
	return &ChatState{
		Info: info,
		BaselineID: baselineID,
		CursorID:   baselineID,
		Dedup:      make(map[int64]bool),
	}
}

// Seen checks and records a deduplication key.
func (s *ChatState) Seen(msgID int) bool {
	if s.Dedup == nil {
		return false
	}
	_, ok := s.Dedup[int64(msgID)]
	if !ok {
		s.Dedup[int64(msgID)] = true
	}
	return ok
}

// NormalizeMessage converts a raw message from an update or polling page into
// the internal message.Message and determines whether it should be output.
func (s *ChatState) ShouldOutput(m *message.Message) bool {
	if m.MessageID <= s.BaselineID {
		return false
	}
	if s.Seen(m.MessageID) {
		return false
	}
	if m.MessageID > s.CursorID {
		s.CursorID = m.MessageID
	}
	return true
}