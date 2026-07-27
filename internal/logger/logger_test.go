package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestSetDebugGating asserts that Debug output is suppressed at the default
// (WARN) level and emitted only after SetDebug(true) — the whole point of the
// --debug flag. It swaps in a buffer-backed handler bound to the package-level
// LevelVar so the real gating logic is exercised.
func TestSetDebugGating(t *testing.T) {
	var buf bytes.Buffer
	orig := std
	std = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level}))
	t.Cleanup(func() { std = orig; SetDebug(false) })

	SetDebug(false)
	Debug("should-not-appear", "k", "v")
	if strings.Contains(buf.String(), "should-not-appear") {
		t.Fatalf("debug line leaked at default level: %q", buf.String())
	}

	SetDebug(true)
	Debug("should-appear", "k", "v")
	if !strings.Contains(buf.String(), "should-appear") {
		t.Fatalf("debug line missing after SetDebug(true): %q", buf.String())
	}

	// Warn is always visible regardless of level.
	SetDebug(false)
	Warn("warn-visible", "k", "v")
	if !strings.Contains(buf.String(), "warn-visible") {
		t.Fatalf("warn line missing at default level: %q", buf.String())
	}
}
