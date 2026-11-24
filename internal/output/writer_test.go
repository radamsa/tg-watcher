package output

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestWritesRecordsInOrderWithNewlines(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf, 4)
	for i := 0; i < 100; i++ {
		if err := w.Write(fmt.Sprintf("record-%d", i)); err != nil {
			t.Fatalf("Write(%d): %v", i, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 100 {
		t.Fatalf("got %d lines, want 100", len(lines))
	}
	for i, line := range lines {
		if want := fmt.Sprintf("record-%d", i); line != want {
			t.Fatalf("line %d = %q, want %q", i, line, want)
		}
	}
}

func TestNoDoubleNewline(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf, 1)
	if err := w.Write("a\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "a\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestWriteAfterClose(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf, 1)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Write("late"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Write after Close = %v, want ErrClosed", err)
	}
}

type errWriter struct{ err error }

func (e errWriter) Write([]byte) (int, error) { return 0, e.err }

func TestWriteErrorPropagates(t *testing.T) {
	w := New(errWriter{err: syscall.EPIPE}, 1)
	if err := w.Write("x"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Wait for the worker to fail.
	if err := w.Close(); err == nil {
		t.Fatal("Close returned nil, want write error")
	} else if !IsBrokenPipe(err) {
		t.Fatalf("Close error %v is not a broken pipe", err)
	}
}

// gatedWriter blocks each Write until released and signals when it first enters.
type gatedWriter struct {
	release chan struct{}
	entered chan struct{}
	once    sync.Once
	mu      sync.Mutex
	buf     bytes.Buffer
}

func (g *gatedWriter) Write(p []byte) (int, error) {
	g.once.Do(func() { close(g.entered) })
	<-g.release
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.buf.Write(p)
}

func TestBackpressureBlocksInsteadOfDropping(t *testing.T) {
	gw := &gatedWriter{release: make(chan struct{}), entered: make(chan struct{})}
	w := New(gw, 1)

	if err := w.Write("first"); err != nil {
		t.Fatal(err)
	}
	// Wait until the worker is stuck writing the first record.
	<-gw.entered

	// "second" fills the queue; "third" must block until the worker drains.
	done := make(chan error, 1)
	go func() {
		if err := w.Write("second"); err != nil {
			done <- err
			return
		}
		done <- w.Write("third")
	}()

	select {
	case err := <-done:
		t.Fatalf("Write returned early while the queue was full: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(gw.release)
	if err := <-done; err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	gw.mu.Lock()
	defer gw.mu.Unlock()
	if got := gw.buf.String(); got != "first\nsecond\nthird\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestConcurrentWritersProduceIntactRecords(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf, 8)
	const goroutines, perGoroutine = 8, 50
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				if err := w.Write(fmt.Sprintf("g%d-%d", g, i)); err != nil {
					t.Errorf("Write: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != goroutines*perGoroutine {
		t.Fatalf("got %d lines, want %d", len(lines), goroutines*perGoroutine)
	}
	seen := make(map[string]bool, len(lines))
	for _, line := range lines {
		if seen[line] {
			t.Fatalf("duplicate record %q", line)
		}
		seen[line] = true
	}
}
