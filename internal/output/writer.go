// Package output owns the single sequential stdout writer. See plan §7.5, §10.
package output

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"sync"
	"syscall"
)

// ErrClosed is returned by Write after the writer has been closed.
var ErrClosed = errors.New("output writer is closed")

// IsBrokenPipe reports whether err means the consumer closed stdout. Such an
// error is a normal shutdown reason, not a failure.
func IsBrokenPipe(err error) bool {
	return errors.Is(err, syscall.EPIPE)
}

// Writer serializes records from all producers into a single stream. The queue
// is bounded: when it is full, Write blocks (backpressure) instead of dropping
// records silently.
type Writer struct {
	w   io.Writer
	max int

	mu     sync.Mutex
	cond   *sync.Cond
	queue  []string
	closed bool
	done   bool
	err    error
}

// New starts a Writer that writes to w. maxQueue is the maximum number of
// pending records; values below 1 are treated as 1.
func New(w io.Writer, maxQueue int) *Writer {
	if maxQueue < 1 {
		maxQueue = 1
	}
	out := &Writer{w: w, max: maxQueue}
	out.cond = sync.NewCond(&out.mu)
	go out.run()
	return out
}

// Write enqueues one record. Records are separated by a newline, which is added
// unless the record already ends with one. Write blocks while the queue is full
// and returns once the record is accepted, or an error if the writer stopped.
func (o *Writer) Write(record string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	for len(o.queue) >= o.max && !o.closed && !o.done {
		o.cond.Wait()
	}
	if o.done || o.closed {
		if o.err != nil {
			return o.err
		}
		return ErrClosed
	}
	if !strings.HasSuffix(record, "\n") {
		record += "\n"
	}
	o.queue = append(o.queue, record)
	o.cond.Signal()
	return nil
}

// Close stops accepting records, drains the queue and waits for the background
// goroutine to finish. It returns the first write error, if any.
func (o *Writer) Close() error {
	o.mu.Lock()
	if !o.closed {
		o.closed = true
		o.cond.Broadcast()
	}
	for !o.done {
		o.cond.Wait()
	}
	err := o.err
	o.mu.Unlock()
	return err
}

// Err returns the first write error observed by the writer.
func (o *Writer) Err() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.err
}

func (o *Writer) run() {
	bw := bufio.NewWriter(o.w)
	for {
		o.mu.Lock()
		for len(o.queue) == 0 && !o.closed {
			o.cond.Wait()
		}
		if len(o.queue) == 0 { // closed and drained
			o.done = true
			o.cond.Broadcast()
			o.mu.Unlock()
			return
		}
		record := o.queue[0]
		o.queue[0] = ""
		o.queue = o.queue[1:]
		o.cond.Broadcast()
		o.mu.Unlock()

		_, err := bw.WriteString(record)
		if err == nil {
			err = bw.Flush()
		}
		if err != nil {
			o.mu.Lock()
			o.err = err
			o.queue = nil
			o.done = true
			o.cond.Broadcast()
			o.mu.Unlock()
			return
		}
	}
}
