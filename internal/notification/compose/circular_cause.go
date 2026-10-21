package compose

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// withoutCircularCause drops a cause that is one of the root workload's
// own pods: "ccm keeps crashing because pod ccm-7d9f has not been ready"
// says the workload fails because the workload fails. The message then
// reads as an incident with nothing outside it to blame.
func withoutCircularCause(p incident.Incident) incident.Incident {
	cause := p.Cause
	if cause == nil || cause.Change != nil ||
		cause.Root.Kind != kube.KindPod || !incident.IsWorkload(p.Root.Kind) {
		return p
	}
	owner, ok := ownerIn([]inventory.EntityID{p.Root}, cause.Root)
	if ok && owner == p.Root {
		p.Cause = nil
	}
	return p
}
