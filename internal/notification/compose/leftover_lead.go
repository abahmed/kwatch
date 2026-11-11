package compose

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// leftoverLead leads an incident that groups the leftovers of a
// removed or stopped controller (incident.Leftovers): the absent
// backend is the news, not whichever of its objects is listed first.
// "Leftovers of a removed or stopped controller (4 objects): Service
// policy-system/policy-svc, which its webhooks call, is gone". One
// object alone keeps its own lead.
func leftoverLead(f caseFacts) (string, bool) {
	objects := f.p.LeftoverObjects()
	if objects < 2 {
		return "", false
	}
	return "Leftovers of a removed or stopped controller (" +
		strconv.Itoa(objects) + " objects): " +
		f.leadName(f.p.Root) + ", which its webhooks call, " +
		leftoverState(f), true
}

// leftoverState says what is wrong with the absent backend, from the
// webhook findings among the members.
func leftoverState(f caseFacts) string {
	for _, m := range f.members {
		if m.Reason == reasons.WebhookBackendNotFound {
			return "is gone"
		}
	}
	for _, m := range f.members {
		if m.Reason == reasons.WebhookNoEndpoints {
			return "has no ready pods"
		}
	}
	return "is gone or has nothing ready"
}
