package audit

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// Action is what a decision did to an incident's conversation.
type Action string

// Actions. The strings are stable: people grep audit logs for them.
const (
	ActionCreate   Action = "create"
	ActionUpdate   Action = "update"
	ActionResolved Action = "resolved"
)

// Cause states.
const (
	// CauseKnown names a root cause other than the failing object.
	CauseKnown = "known"
	// CauseSelf blames the failing object itself, which tells the reader
	// nothing new.
	CauseSelf = "self"
	// CauseUnknown has no explanation.
	CauseUnknown = "unknown"
)

// Entry is one audit line.
type Entry struct {
	Timestamp time.Time `json:"ts"`
	Action    Action    `json:"action"`
	Incident  string    `json:"incident"`
	Namespace string    `json:"namespace,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	Severity  string    `json:"severity,omitempty"`
	Root      string    `json:"root,omitempty"`
	// Tier is how loudly the message was delivered: page, notify or
	// digest.
	Tier          string `json:"tier,omitempty"`
	Revision      int    `json:"revision,omitempty"`
	AffectedCount int    `json:"affectedCount,omitempty"`
	CauseState    string `json:"causeState,omitempty"`
	RootCause     string `json:"rootCause,omitempty"`
	Confidence    string `json:"confidence,omitempty"`
	// DecisionReason explains why the manager sent this message.
	DecisionReason string `json:"decisionReason,omitempty"`
	// ContentHash fingerprints the announced content; an update with the
	// same hash as the previous message repeated it.
	ContentHash string `json:"contentHash,omitempty"`
	// Previous is the resolved incident this one repeats, when linked.
	Previous string `json:"previous,omitempty"`
	// Delivery says how the decision reaches people when not as a message
	// of its own: "digest" or "startup summary" when that message carries
	// it, "paging" when only alert-tracking providers receive it. Empty
	// for an ordinary message.
	Delivery string `json:"delivery,omitempty"`
}

// Config selects where entries go.
type Config struct {
	Enabled bool
	// Output is "stdout" or a file path.
	Output string
}

// Logger writes entries as JSON lines. The zero value and a disabled
// logger discard everything.
type Logger struct {
	mu     sync.Mutex
	enc    *json.Encoder
	closer io.Closer
}

// NewLogger opens the configured output. A file that cannot be opened
// falls back to stdout so the audit trail is not silently lost.
func NewLogger(cfg Config) *Logger {
	if !cfg.Enabled {
		return &Logger{}
	}
	var writer io.Writer = os.Stdout
	l := &Logger{}
	if cfg.Output != "" && cfg.Output != "stdout" {
		f, err := openRotatingFile(cfg.Output, maxAuditFileBytes)
		if err != nil {
			klog.ErrorS(err, "failed to open audit log file, using stdout",
				"component", "audit", "path", cfg.Output)
		} else {
			writer, l.closer = f, f
		}
	}
	l.enc = json.NewEncoder(writer)
	return l
}

// Record writes one entry.
func (l *Logger) Record(entry Entry) {
	if l == nil || l.enc == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.enc.Encode(entry); err != nil {
		klog.ErrorS(err, "failed to write audit log entry",
			"component", "audit")
	}
}

// Close releases a file output.
func (l *Logger) Close() error {
	if l == nil || l.closer == nil {
		return nil
	}
	return l.closer.Close()
}
