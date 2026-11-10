package logsetup

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/klog/v2"
)

// FormatJSON selects one JSON object per line.
const FormatJSON = "json"

// UseJSON sends klog output through a JSON handler: one object per line
// with ts, level, msg, caller and the key/value pairs as fields. Error
// lines go to stderr, everything else to stdout.
func UseJSON(stdout, stderr io.Writer) {
	h := &splitHandler{
		out: newHandler(stdout), err: newHandler(stderr),
	}
	klog.SetLoggerWithOptions(logr.FromSlogHandler(h),
		klog.ContextualLogger(true),
		klog.WriteKlogBuffer(writeText(h)))
}

// Reset returns klog to its text output.
func Reset() { klog.ClearLogger() }

// Wants reports whether format asks for JSON.
func Wants(format string) bool {
	return strings.EqualFold(strings.TrimSpace(format), FormatJSON)
}

func newHandler(w io.Writer) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{
		AddSource:   true,
		Level:       slog.Level(-10),
		ReplaceAttr: replaceAttr,
	})
}

// replaceAttr renames slog's fields to ts, level and caller.
func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.TimeKey:
		a.Key = "ts"
	case slog.LevelKey:
		a.Value = slog.StringValue(levelName(a.Value.Any()))
	case slog.SourceKey:
		if src, ok := a.Value.Any().(*slog.Source); ok {
			a = slog.String("caller", callerOf(src))
		}
	}
	return a
}

func levelName(v any) string {
	lvl, _ := v.(slog.Level)
	switch {
	case lvl >= slog.LevelError:
		return "error"
	case lvl >= slog.LevelWarn:
		return "warning"
	}
	return "info"
}

func callerOf(src *slog.Source) string {
	return filepath.Base(src.File) + ":" + itoa(src.Line)
}

// splitHandler writes error records to one handler and the rest to
// another.
type splitHandler struct {
	out, err slog.Handler
}

func (s *splitHandler) pick(l slog.Level) slog.Handler {
	if l >= slog.LevelError {
		return s.err
	}
	return s.out
}

func (s *splitHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (s *splitHandler) Handle(ctx context.Context, r slog.Record) error {
	return s.pick(r.Level).Handle(ctx, r)
}

func (s *splitHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return &splitHandler{out: s.out.WithAttrs(a), err: s.err.WithAttrs(a)}
}

func (s *splitHandler) WithGroup(n string) slog.Handler {
	return &splitHandler{out: s.out.WithGroup(n), err: s.err.WithGroup(n)}
}
