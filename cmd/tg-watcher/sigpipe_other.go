//go:build !unix

package main

// ignoreBrokenPipeSignal is a no-op on platforms without SIGPIPE semantics.
func ignoreBrokenPipeSignal() {}
