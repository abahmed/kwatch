package incident

import (
	"time"

	"k8s.io/klog/v2"
)

// logLifecycle writes one default-level line for a lifecycle step
// people later ask kwatch's logs about: why an incident closed, came
// back, went quiet or changed tier. The timeline note is what people
// see; this line lets an operator audit the decisions from logs alone,
// the way "incident opened" does for the start.
func logLifecycle(p *Incident, operation, message string, kv ...any) {
	args := append([]any{
		"component", "incident", "operation", operation,
		"incident", p.ID, "namespace", p.Root.Namespace,
		"kind", string(p.Root.Kind), "name", p.Root.Name,
		"tier", p.Tier.String(),
	}, kv...)
	klog.InfoS(message, args...)
}

// logResolved logs a resolve and how long the incident lasted.
func logResolved(p *Incident, now time.Time, why Reason, quiet bool) {
	logLifecycle(p, "resolve", "incident resolved",
		"reason", string(why), "quiet", quiet,
		"lasted", max(0, now.Sub(p.Opened)).Round(time.Second).String())
}

// reasonUntold is the logged reason of an incident that ended before
// anyone was told about it.
const reasonUntold = "settled before alerting"

// reasonMovedUntold is the logged reason of one whose failures all
// moved to another incident before anyone was told: it was the same
// failure seen through an object whose owner was not listed yet, or
// blamed on another cause a moment later.
const reasonMovedUntold = "moved before alerting"

// closeUntold ends a settling incident that lost its failures. One
// whose failures all went to other incidents was a duplicate nobody
// heard of: it is dropped, so it never shows as a problem of its own.
// One with a failure that recovered while a member settled before
// alerting, even when its other failures moved: it is kept as that.
// Either way the close is logged, like every resolve.
func (m *Manager) closeUntold(p *Incident, now time.Time) {
	moved := len(p.movedTo) > 0 && !p.recoveredHere &&
		p.SupersededBy == "" && len(p.History) == 0
	why := reasonUntold
	if moved {
		why = reasonMovedUntold
	}
	logResolved(p, now, Reason(why), true)
	if moved {
		m.forget(p)
	}
}
