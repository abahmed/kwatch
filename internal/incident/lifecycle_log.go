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
		"lasted", now.Sub(p.Opened).Round(time.Second).String())
}
