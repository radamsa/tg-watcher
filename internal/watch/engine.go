package watch

import (
	"context"
	"log/slog"
	"os"
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
	mu      sync.Mutex
	states  map[int64]*ChatState // chatID → state
	ch      chan event
	output  *output.Writer
	render  Renderer
	filter  *Filter
	log     *slog.Logger
	done    chan struct{}
	stopped bool
}

type event struct {
	chatID int64
	msg    message.Message
}

// NewEngine creates an idle engine. Call Start to begin processing.
func NewEngine(states map[int64]*ChatState, w *output.Writer, r Renderer, log *slog.Logger) *Engine {
	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return &Engine{
		states: states,
		output: w,
		render: r,
		log:    log,
		done:   make(chan struct{}),
	}
}

// SetFilter installs the focus filter. It must be called before Start; a nil
// filter matches everything.
func (e *Engine) SetFilter(f *Filter) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.filter = f
}

// Start launches the background processor goroutine. The caller must eventually
// call Stop.
func (e *Engine) Start() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ch != nil || e.stopped {
		return
	}
	e.ch = make(chan event, 64)
	go e.run(e.ch)
}

// Stop waits for the processor to drain and shuts it down.
func (e *Engine) Stop() {
	e.mu.Lock()
	ch := e.ch
	if ch == nil || e.stopped {
		e.mu.Unlock()
		return
	}
	e.stopped = true
	e.ch = nil
	e.mu.Unlock()

	close(ch)
	<-e.done
}

// Ingest hands a normalized message to the pipeline. It blocks when the
// internal queue is full (backpressure). A nil ctx is treated as non-
// cancellable.
func (e *Engine) Ingest(ctx context.Context, chatID int64, msg message.Message) error {
	e.mu.Lock()
	ch := e.ch
	stopped := e.stopped
	e.mu.Unlock()

	if ch == nil || stopped {
		return nil
	}
	if ctx == nil {
		ch <- event{chatID: chatID, msg: msg}
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

func (e *Engine) match(m message.Message) bool {
	e.mu.Lock()
	f := e.filter
	e.mu.Unlock()
	return f == nil || f.Match(m)
}

func (e *Engine) run(ch chan event) {
	defer func() { close(e.done) }()

	for ev := range ch {
		st := e.State(ev.chatID)
		if st == nil {
			continue
		}
		if !st.ShouldProcess(&ev.msg) {
			continue
		}
		if !e.match(ev.msg) {
			continue
		}
		if st.Seen(ev.msg.MessageID) {
			continue
		}
		rendered := e.render.Render(ev.msg)
		if err := e.output.Write(rendered); err != nil {
			e.log.Error("output write failed", "error", err, "chat", ev.chatID)
			return
		}
	}
}
