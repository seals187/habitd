package server

import (
	"log/slog"
	"os"
	"testing"
)

// TestMain keeps the request log out of the test output, so a failure stands out.
// Warnings and errors still show.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	os.Exit(m.Run())
}
