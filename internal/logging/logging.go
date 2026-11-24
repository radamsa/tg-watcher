// Package logging configures diagnostics. All log output goes to stderr; stdout
// is reserved for message records. See plan §7.5, §5.8.
package logging

import (
	"io"
	"log/slog"
)

// New returns a logger that writes human-readable logs to w (normally
// os.Stderr). When debug is true the level is Debug, otherwise Info.
func New(w io.Writer, debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}
