package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/model"
)

// Config contains incident lifecycle policy and its narrow domain seams.
// Keeping this contract separate from grouping implementation makes the
// engine's construction requirements visible to contributors.
type Config struct {
	// Now is the immutable time source for lifecycle decisions. Nil uses the
	// real clock for standalone callers.
	Now               func() time.Time
	Window            time.Duration
	LifecycleInterval time.Duration
	Enricher          enricher.Enricher
	// AuditLogger records intentional lifecycle skips through the incident
	// domain seam. It is supplied during construction so processing never
	// observes partially wired dependencies.
	AuditLogger   SkipLogger
	LifecycleHook func(inc *model.Incident, action model.IncidentAction)
	// MassFailureHook is called during a lifecycle tick.
	MassFailureHook func()
	BaselineTTL     time.Duration
	// OwnerBaselineTTL bounds owner-level baseline entries, which are seeded
	// with an empty pod name and cover every Pod of that owner.
	OwnerBaselineTTL           time.Duration
	Baseline                   map[string]map[string]int64
	OnBaselineChange           func(baseline map[string]map[string]int64)
	EscalationEnabled          bool
	EscalationTiers            []int
	InhibitNodeSuppressesPods  bool
	MaxBaseline                int
	RenotifyIntervalBySeverity map[string]time.Duration
	RenotifyMaxPerIncident     int
	ResolveHoldDown            time.Duration
	Runbooks                   map[string]string
	SmartGroupingWindow        time.Duration
	// DependenciesOf resolves the shared dependencies an incident touches, so
	// the engine can suppress symptoms already covered by a mass-failure
	// alert. Supplied by the app, which owns the resource graph. Nil disables
	// mass-failure suppression.
	DependenciesOf func(*model.Incident) []string
	// NamespaceFanOutThreshold is how many distinct owners must fail the same
	// way, in one namespace, inside one grouping window before their per-owner
	// groups are collapsed into a single namespace-level notification. Zero
	// disables the collapse.
	NamespaceFanOutThreshold int
	// SubjectPresent reports whether the object an incident is about still
	// exists, and whether the caller can answer for that kind at all.
	//
	// Staleness alone is a poor resolve signal. A Deployment wedged on a
	// failed rollout stops producing events once the last replica gives up,
	// and the engine then closed the incident as if the rollout had
	// succeeded. Asking whether the object is still there separates "gone,
	// so genuinely finished" from "still broken, just quiet". Nil keeps the
	// old staleness-only behaviour.
	SubjectPresent func(resource, namespace, name string) (exists, known bool)
}
