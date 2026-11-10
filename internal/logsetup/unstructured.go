package logsetup

import (
	"bytes"
	"context"
	"log/slog"
	"strconv"

	"github.com/abahmed/kwatch/internal/clock"
)

func itoa(n int) string { return strconv.Itoa(n) }

// writeText turns klog's text lines (from klog.Warning and similar
// calls, which carry no key/value pairs) into JSON records so that
// warnings keep their level.
func writeText(h slog.Handler) func([]byte) {
	return func(p []byte) {
		lvl, caller, msg := parseText(p)
		r := slog.NewRecord(clock.RealClock{}.Now(), lvl, msg, 0)
		r.AddAttrs(slog.String("caller", caller))
		_ = h.Handle(context.Background(), r)
	}
}

// parseText splits "W1109 12:00:00.000000 1 file.go:56] message" into
// its level, caller and message.
func parseText(p []byte) (slog.Level, string, string) {
	p = bytes.TrimRight(p, "\n")
	lvl := slog.LevelInfo
	if len(p) > 0 && hasHeader(p) {
		switch p[0] {
		case 'W':
			lvl = slog.LevelWarn
		case 'E', 'F':
			lvl = slog.LevelError
		}
	}
	i := bytes.Index(p, []byte("] "))
	if i < 0 {
		return lvl, "", string(p)
	}
	head := string(p[:i])
	caller := head[bytes.LastIndexByte(p[:i], ' ')+1:]
	return lvl, caller, string(p[i+2:])
}
