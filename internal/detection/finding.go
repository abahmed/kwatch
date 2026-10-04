package detection

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Severity is a detector's hint of how serious a finding is. The policy
// layer decides the final tier from severity and impact.
type Severity uint8

// Severities, from least to most serious.
const (
	Info Severity = iota + 1
	Warning
	Critical
)

// Finding is one abnormal condition of one entity.
type Finding struct {
	Entity   inventory.EntityID
	Reason   string
	Severity Severity
	// Health is how the entity is doing given this finding. Classify
	// derives it from Severity unless the detector set it.
	Health Health
	// Mode is a short, stable failure identity such as "CrashLoop" or
	// "MemoryPressure". Classify derives it from Reason unless the
	// detector set a finer one. The tracker compares it, not Summary.
	Mode Mode
	// Since is when the condition started, as reported by Kubernetes when
	// available, otherwise when it was first detected.
	Since time.Time
	// Summary is a short, human sentence describing the condition.
	Summary string
	// Evidence holds the observations that support the finding, already redacted.
	Evidence []Evidence
	// Symptom marks conditions that are consequences by construction, such
	// as a Service without endpoints. They never become a root on their own
	// when a cause can be found.
	Symptom bool
	// Advisory marks a configuration risk, not a failure: a workload
	// with no readiness probe, a single replica, an image tag that can
	// change. It is reported in the digest, never leads a message about
	// a failure, is never a failure to explain nor a cause, and only
	// adds a consequence to a failure it made worse.
	Advisory bool `json:",omitempty"`
}

// Key identifies a finding across evaluations.
func (s Finding) Key() Key {
	return Key{Entity: s.Entity, Reason: s.Reason}
}

// Key identifies one finding of one entity.
type Key struct {
	Entity inventory.EntityID
	Reason string
}

// Evidence is one supporting observation.
type Evidence struct {
	Label string
	Value string
}
