package signal

import (
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// Severity is a detector's hint of how serious a signal is. The policy
// layer decides the final tier from severity and impact.
type Severity uint8

// Severities, from least to most serious.
const (
	Info Severity = iota + 1
	Warning
	Critical
)

// Signal is one abnormal condition of one entity.
type Signal struct {
	Entity   knowledge.EntityID
	Reason   string
	Severity Severity
	// Since is when the condition started, as reported by Kubernetes when
	// available, otherwise when it was first detected.
	Since time.Time
	// Summary is a short, human sentence describing the condition.
	Summary string
	// Evidence holds the facts that support the signal, already redacted.
	Evidence []Evidence
	// Symptom marks conditions that are consequences by construction, such
	// as a Service without endpoints. They never become a root on their own
	// when a cause can be found.
	Symptom bool
}

// Key identifies a signal across evaluations.
func (s Signal) Key() Key {
	return Key{Entity: s.Entity, Reason: s.Reason}
}

// Key identifies one signal of one entity.
type Key struct {
	Entity knowledge.EntityID
	Reason string
}

// Evidence is one supporting fact.
type Evidence struct {
	Label string
	Value string
}
