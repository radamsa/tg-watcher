//go:build unix

package main

import (
	"os/signal"
	"syscall"
)

// ignoreBrokenPipeSignal makes writes to a closed pipe return EPIPE instead of
// terminating the process with SIGPIPE, so it can exit cleanly with code 0.
func ignoreBrokenPipeSignal() {
	signal.Ignore(syscall.SIGPIPE)
}
