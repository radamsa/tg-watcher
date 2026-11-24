package watch

import (
	"io"
	"sync"
	"testing"
	"time"

	"tg-watcher/internal/message"
	"tg-watcher/internal/output"
	"tg-watcher/internal/resolve"
)

func TestChatStateNew(t *testing.T) {
	info := resolve.ChatInfo{ChatID: -100123, Title: "Test"}
	s := NewChatState(info, 42)
	if s.BaselineID != 42 || s.CursorID != 42 {
		t.Fatalf("baseline: got %d,%d want 42,42", s.BaselineID, s.CursorID)
	}
	if s.Info.ChatID != -100123 {
		t.Fatalf("chat id: got %d", s.Info.ChatID)
	}
}

func TestShouldOutput_BelowBaseline(t *testing.T) {
	s := NewChatState(resolve.ChatInfo{}, 100)
	msg := &message.Message{MessageID: 50}
	if s.ShouldOutput(msg) {
		t.Fatal("should reject message below baseline")
	}
}

func TestShouldOutput_AtBaseline(t *testing.T) {
	s := NewChatState(resolve.ChatInfo{}, 100)
	msg := &message.Message{MessageID: 100}
	if s.ShouldOutput(msg) {
		t.Fatal("should reject message at baseline")
	}
}

func TestShouldOutput_AboveBaseline(t *testing.T) {
	s := NewChatState(resolve.ChatInfo{}, 100)
	msg := &message.Message{MessageID: 101}
	if !s.ShouldOutput(msg) {
		t.Fatal("should accept message above baseline")
	}
}

func TestDedup(t *testing.T) {
	s := NewChatState(resolve.ChatInfo{}, 0)
	msg1 := &message.Message{MessageID: 1}
	msg2 := &message.Message{MessageID: 1}
	if !s.ShouldOutput(msg1) {
		t.Fatal("first should be accepted")
	}
	if s.ShouldOutput(msg2) {
		t.Fatal("duplicate should be rejected")
	}
}

func TestCursorAdvances(t *testing.T) {
	s := NewChatState(resolve.ChatInfo{}, 10)
	s.ShouldOutput(&message.Message{MessageID: 15})
	s.ShouldOutput(&message.Message{MessageID: 12})
	if s.CursorID != 15 {
		t.Fatalf("cursor should be 15, got %d", s.CursorID)
	}
}

func TestSeen(t *testing.T) {
	s := NewChatState(resolve.ChatInfo{}, 0)
	if s.Seen(5) {
		t.Fatal("first Seen should return false")
	}
	if !s.Seen(5) {
		t.Fatal("second Seen should return true")
	}
}

type mockRenderer struct {
	result string
}

func (m *mockRenderer) Render(msg message.Message) string {
	return m.result
}

// testWriter implements io.Writer for capturing engine output.
type testWriter struct {
	mu      sync.Mutex
	records []string
}

func (w *testWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.records = append(w.records, string(p))
	return len(p), nil
}

var _ io.Writer = (*testWriter)(nil)

func TestEngine_IngestAndOutput(t *testing.T) {
	info := resolve.ChatInfo{ChatID: 1, Title: "Ch"}
	states := map[int64]*ChatState{
		1: NewChatState(info, 0),
	}
	tw := &testWriter{}
	wr := output.New(tw, 64)
	r := &mockRenderer{result: "hello\n"}
	e := NewEngine(states, wr, r, nil)
	e.Start()

	msg := message.Message{
		ChatID:    1,
		MessageID: 1,
		Date:      time.Now(),
		Channel:   "Ch",
		Text:      "test",
	}
	if err := e.Ingest(nil, 1, msg); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	e.Stop()
	wr.Close()

	if len(tw.records) != 1 {
		t.Fatalf("got %d records, want 1", len(tw.records))
	}
}

func TestEngine_UnknownChatSkipped(t *testing.T) {
	states := map[int64]*ChatState{
		1: NewChatState(resolve.ChatInfo{}, 0),
	}
	tw := &testWriter{}
	wr := output.New(tw, 64)
	r := &mockRenderer{}
	e := NewEngine(states, wr, r, nil)
	e.Start()

	_ = e.Ingest(nil, 99, message.Message{MessageID: 1})
	e.Stop()
	wr.Close()

	if len(tw.records) != 0 {
		t.Fatalf("expected no output for unknown chat")
	}
}

func TestEngine_DoubleStart(t *testing.T) {
	states := map[int64]*ChatState{}
	tw := &testWriter{}
	wr := output.New(tw, 64)
	r := &mockRenderer{}
	e := NewEngine(states, wr, r, nil)
	e.Start()
	e.Start() // should be a no-op
	e.Stop()
	wr.Close()
}

func TestEngine_StopWithoutStart(t *testing.T) {
	states := map[int64]*ChatState{}
	tw := &testWriter{}
	wr := output.New(tw, 64)
	r := &mockRenderer{}
	e := NewEngine(states, wr, r, nil)
	e.Stop() // should not panic
	wr.Close()
}

func TestEngine_IngestAfterStop(t *testing.T) {
	states := map[int64]*ChatState{
		1: NewChatState(resolve.ChatInfo{}, 0),
	}
	tw := &testWriter{}
	wr := output.New(tw, 64)
	r := &mockRenderer{}
	e := NewEngine(states, wr, r, nil)
	e.Start()
	e.Stop()

	_ = e.Ingest(nil, 1, message.Message{MessageID: 1})
	wr.Close()

	if len(tw.records) != 0 {
		t.Fatalf("no output expected after stop")
	}
}