// Package logger provides the shared, level-gated diagnostic logger for
// cloudtab. It is a thin wrapper over log/slog writing to stderr so debug
// output never mixes with report output on stdout.
//
// Default level is WARN: normal runs stay quiet except for genuine warnings
// (e.g. a disabled price cache). Passing --debug on the CLI switches the
// level to DEBUG, surfacing cache hits, retries, and per-call backend latency
// — enough to answer "why is this run slow / why was this SKU re-fetched"
// without attaching a debugger.
package logger

import (
	"log/slog"
	"os"
)

// level gates the shared handler. slog.LevelVar is safe for concurrent use,
// so SetDebug may be called at any time (in practice: once, during CLI flag
// parsing, before any pricing work starts).
var level = func() *slog.LevelVar {
	v := new(slog.LevelVar)
	v.Set(slog.LevelWarn)
	return v
}()

var std = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

// SetDebug enables (or disables) debug-level logging on the shared logger.
func SetDebug(on bool) {
	if on {
		level.Set(slog.LevelDebug)
	} else {
		level.Set(slog.LevelWarn)
	}
}

// Debug logs at DEBUG level; a no-op unless SetDebug(true) was called.
func Debug(msg string, args ...any) { std.Debug(msg, args...) }

// Warn logs at WARN level; always visible.
func Warn(msg string, args ...any) { std.Warn(msg, args...) }
