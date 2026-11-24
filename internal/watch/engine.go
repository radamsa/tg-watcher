package watch

import (
	"context"
	"log/slog"
	"sync"

	"tg-watcher/internal/message"
	"tg-watcher/internal/output"
)

// Renderer formats a Message into the output string.
type Renderer interface {
	Render(m message.Message) string
}

// Engine is the central monitoring pipeline. It accepts normalized messages
// from both the live update stream and the periodic poller, deduplicates them,
// serialises per-chat processing, and writes the result to stdout via the
// output writer.
type Engine struct {
	mu     sync.Mutex
	states map[int64]*ChatState // chatID → state
	ch     chan event
	output *output.Writer
	render Renderer
	log    *slog.Logger
	done   chan struct{}
	closed bool
}

type event struct {
	chatID int64
	msg    message.Message
}

// NewEngine creates an idle engine. Call Start to begin processing.
func NewEngine(states map[int64]*ChatState, w *output.Writer, r Renderer, log *slog.Logger) *Engine {
	return &Engine{
		states: states,
		output: w,
		render: r,
		log:    log,
		done:   make(chan struct{}),
	}
}

// Start launches the background processor goroutine. The caller must eventually
// call Stop.
func (e *Engine) Start() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ch != nil {
		return
	}
	e.ch = make(chan event, 64)
	go e.run()
}

// Stop waits for the processor to drain and shuts it down.
func (e *Engine) Stop() {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	ch := e.ch
	e.ch = nil
	e.mu.Unlock()

	// Drain the channel.
	for range ch {
	}
	<-e.done
}

// Ingest hands a normalized message to the pipeline. It blocks when the
// internal queue is full (backpressure).
func (e *Engine) Ingest(ctx context.Context, chatID int64, msg message.Message) error {
	e.mu.Lock()
	ch := e.ch
	closed := e.closed
	e.mu.Unlock()

	if ch == nil || closed {
		return nil
	}
	select {
	case ch <- event{chatID: chatID, msg: msg}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// State returns the ChatState for a chat, or nil.
func (e *Engine) State(chatID int64) *ChatState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.states[chatID]
}

func (e *Engine) run() {
	defer func() { e.done <- struct{}{} }()

	e.mu.Lock()
	ch := e.ch
	e.mu.Unlock()

	for ev := range ch {
		st := e.State(ev.chatID)
		if st == nil {
			continue
		}
		if !st.ShouldOutput(&ev.msg) {
			continue
		}
		rendered := e.render.Render(ev.msg)
		if err := e.output.Write(rendered); err != nil {
			e.log.Error("output write failed", "error", err, "chat", ev.chatID)
			return
		}
	}
}