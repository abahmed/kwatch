package app

import (
	"os"
	"strings"
	"sync"

	"github.com/abahmed/kwatch/internal/logsetup"
)

// applyLogFormat applies app.logFormatter: "json" writes structured JSON
// lines (errors to stderr, the rest to stdout), anything else keeps klog's
// text format. KWATCH_LOG_FORMAT, when set, wins over the configured value.
// It runs again after the KwatchConfig overlay, so an overlay can also
// switch back to text.
func applyLogFormat(format string) {
	if logsetup.Wants(logsetup.Format(format)) {
		logsetup.UseJSON(os.Stdout, os.Stderr)
		return
	}
	logsetup.Reset()
}

// logFormat remembers the format applied to klog for the whole process.
// klog's logger is process global, so an edition rebuild must not reset it
// while the previous generation's health server still logs through it: the
// format is changed only when it really changes.
type logFormat struct {
	mu      sync.Mutex
	current string
	applied bool
	set     func(string)
}

func newLogFormat() *logFormat {
	return &logFormat{set: applyLogFormat}
}

// apply sets format on klog unless it is already the active format.
func (l *logFormat) apply(format string) {
	if l == nil {
		applyLogFormat(format)
		return
	}
	format = strings.ToLower(strings.TrimSpace(format))
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.applied && l.current == format {
		return
	}
	l.set(format)
	l.current, l.applied = format, true
}
