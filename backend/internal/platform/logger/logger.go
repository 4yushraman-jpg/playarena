package logger

import (
	"log/slog"
	"os"
)

// New creates a *slog.Logger appropriate for the given environment.
//   - development / staging: human-readable text output, DEBUG level.
//   - production:            structured JSON output, INFO level.
func New(env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelDebug}

	var h slog.Handler
	if env == "production" {
		opts.Level = slog.LevelInfo
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(h)
}
