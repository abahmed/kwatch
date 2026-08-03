package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

type Action string

const (
	ActionCreate   Action = "create"
	ActionUpdate   Action = "update"
	ActionResolved Action = "resolved"
	ActionSkip     Action = "skip"
)

type Entry struct {
	Timestamp   time.Time `json:"ts"`
	Action      Action    `json:"action"`
	IncidentKey string    `json:"incidentKey"`
	IncidentID  string    `json:"id,omitempty"`
	Namespace   string    `json:"namespace,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	Severity    string    `json:"severity,omitempty"`
	Name        string    `json:"name,omitempty"`
	Count       int       `json:"count,omitempty"`
	Duration    string    `json:"duration,omitempty"`
	SkipReason  string    `json:"skipReason,omitempty"`
}

type Config struct {
	Enabled bool
	Output  string // "stdout" or file path
}

type AuditLogger struct {
	mu     sync.Mutex
	writer io.Writer
	enc    *json.Encoder
	cfg    Config
}

func NewLogger(cfg Config) *AuditLogger {
	l := &AuditLogger{cfg: cfg}
	if !cfg.Enabled {
		return l
	}
	if cfg.Output == "" || cfg.Output == "stdout" {
		l.writer = os.Stdout
	} else {
		f, err := os.OpenFile(cfg.Output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			klog.ErrorS(err, "failed to open audit log file, falling back to stdout", "path", cfg.Output)
			l.writer = os.Stdout
		} else {
			l.writer = f
		}
	}
	l.enc = json.NewEncoder(l.writer)
	return l
}

func (l *AuditLogger) log(entry Entry) {
	if !l.cfg.Enabled || l.enc == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.enc.Encode(entry); err != nil {
		klog.ErrorS(err, "failed to write audit log entry")
	}
}

func (l *AuditLogger) actionFromIncidentAction(a model.IncidentAction) Action {
	switch a {
	case model.ActionCreate:
		return ActionCreate
	case model.ActionUpdate:
		return ActionUpdate
	case model.ActionResolved:
		return ActionResolved
	default:
		return ActionSkip
	}
}

func (l *AuditLogger) LogIncident(inc *model.Incident, action model.IncidentAction) {
	if !l.cfg.Enabled {
		return
	}
	duration := ""
	if !inc.FirstSeen.IsZero() && !inc.LastSeen.IsZero() {
		duration = inc.LastSeen.Sub(inc.FirstSeen).Round(time.Second).String()
	}
	l.log(Entry{
		Timestamp:   time.Now(),
		Action:      l.actionFromIncidentAction(action),
		IncidentKey: inc.Key,
		IncidentID:  inc.ID,
		Namespace:   inc.Namespace,
		Reason:      inc.Reason,
		Severity:    string(inc.Severity),
		Name:        inc.Name,
		Count:       inc.Count,
		Duration:    duration,
	})
}

func (l *AuditLogger) LogSkip(inc *model.Incident, skipReason string) {
	if !l.cfg.Enabled {
		return
	}
	l.log(Entry{
		Timestamp:   time.Now(),
		Action:      ActionSkip,
		IncidentKey: inc.Key,
		IncidentID:  inc.ID,
		Namespace:   inc.Namespace,
		Reason:      inc.Reason,
		SkipReason:  skipReason,
	})
}

func (l *AuditLogger) Close() error {
	if !l.cfg.Enabled || l.writer == nil {
		return nil
	}
	if f, ok := l.writer.(*os.File); ok && f != os.Stdout && f != os.Stderr {
		return f.Close()
	}
	return nil
}

// String returns the human-readable representation of this action.
func (a Action) String() string {
	return string(a)
}

// MarshalJSON implements json.Marshaler.
func (a Action) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(a))
}

// UnmarshalJSON implements json.Unmarshaler.
func (a *Action) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*a = Action(s)
	return nil
}

var _ fmt.Stringer = Action("")
