// Package logging provides structured logging with log levels,
// correlation ID propagation, and configurable output formatting.
// Built on Go 1.26's log/slog with a thin convenience wrapper.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

type ctxKey string

const correlationIDKey ctxKey = "correlation_id"

// Level enum for the CLI --log-level flag.
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Supported output formats.
const (
	FormatText = "text"
	FormatJSON = "json"
)

// Init initialises the slog default logger from the given level and format
// strings. Valid level values: debug, info, warn, error (case-insensitive).
// Valid format values: text, json. Defaults: info, text.
func Init(levelStr, formatStr string) {
	lvl := parseLevel(levelStr)
	var h slog.Handler

	opts := &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Remove the default time key; we want structured but not
			// redundant timestamp noise on every line for a CLI tool.
			if a.Key == slog.TimeKey && len(groups) == 0 {
				return slog.Attr{}
			}
			return a
		},
	}

	switch strings.ToLower(formatStr) {
	case FormatJSON:
		h = slog.NewJSONHandler(os.Stderr, opts)
	default:
		h = slog.NewTextHandler(os.Stderr, opts)
	}

	slog.SetDefault(slog.New(h))
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithCorrelationID returns a context that carries the given correlation ID.
func WithCorrelationID(ctx context.Context, cid string) context.Context {
	return context.WithValue(ctx, correlationIDKey, cid)
}

// GetCorrelationID extracts the correlation ID from context, if any.
func GetCorrelationID(ctx context.Context) string {
	if cid, ok := ctx.Value(correlationIDKey).(string); ok {
		return cid
	}
	return ""
}

// Logger returns a slog.Logger that includes the correlation ID from context
// (if set) in every log line under the "correlation_id" key.
func Logger(ctx context.Context) *slog.Logger {
	cid := GetCorrelationID(ctx)
	if cid == "" {
		return slog.Default()
	}
	return slog.Default().With("correlation_id", cid)
}

// NewCorrelationID generates a short (16-char hex) correlation ID.
// This is intentionally not crypto-random — speed matters more.
func NewCorrelationID() string {
	b := make([]byte, 8)
	// Use a fast non-crypto PRNG from the OS.
	f, err := os.Open("/dev/urandom")
	if err != nil {
		// Fallback: not great but never blocks.
		return fmt.Sprintf("%016x", os.Getpid()^int(os.Getpid()<<32))
	}
	defer f.Close()
	_, _ = f.Read(b)
	return fmt.Sprintf("%016x", b)
}

// Debug logs at debug level via the context-aware logger.
func Debug(ctx context.Context, msg string, attrs ...any) {
	Logger(ctx).Debug(msg, attrs...)
}

// Info logs at info level via the context-aware logger.
func Info(ctx context.Context, msg string, attrs ...any) {
	Logger(ctx).Info(msg, attrs...)
}

// Warn logs at warn level via the context-aware logger.
func Warn(ctx context.Context, msg string, attrs ...any) {
	Logger(ctx).Warn(msg, attrs...)
}

// Error logs at error level via the context-aware logger.
func Error(ctx context.Context, msg string, attrs ...any) {
	Logger(ctx).Error(msg, attrs...)
}

// SetDefaultOutput changes the output writer of the default logger.
// Used primarily for testing; ordinary callers should use Init.
func SetDefaultOutput(w io.Writer) {
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(h))
}
