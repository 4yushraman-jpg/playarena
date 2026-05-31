package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// RequestLogger returns a chi-compatible middleware that logs every HTTP request
// using the provided *slog.Logger.
//
// Log levels by response status:
//   - 5xx → ERROR
//   - 4xx → WARN
//   - 2xx / 3xx → INFO
func RequestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return chimw.RequestLogger(&slogFormatter{log: log})
}

type slogFormatter struct{ log *slog.Logger }

func (f *slogFormatter) NewLogEntry(r *http.Request) chimw.LogEntry {
	return &slogEntry{log: f.log, r: r}
}

type slogEntry struct {
	log *slog.Logger
	r   *http.Request
}

func (e *slogEntry) Write(status, bytes int, _ http.Header, elapsed time.Duration, _ interface{}) {
	level := slog.LevelInfo
	if status >= 500 {
		level = slog.LevelError
	} else if status >= 400 {
		level = slog.LevelWarn
	}

	e.log.Log(
		e.r.Context(),
		level,
		"http",
		slog.String("method", e.r.Method),
		slog.String("path", e.r.URL.Path),
		slog.Int("status", status),
		slog.Int("bytes", bytes),
		slog.String("elapsed", elapsed.String()),
		slog.String("request_id", chimw.GetReqID(e.r.Context())),
		slog.String("remote", e.r.RemoteAddr),
	)
}

func (e *slogEntry) Panic(v interface{}, stack []byte) {
	e.log.Error("panic recovered",
		slog.Any("panic", v),
		slog.String("stack", string(stack)),
	)
}
