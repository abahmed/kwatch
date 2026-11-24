package compose

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// leftoverLead leads an incident that groups the leftovers of a
// removed or stopped controller (incident.Leftovers): the group's root
// is the news, not whichever of its objects is listed first. The
// words come from the root only:
//   - a backend its webhooks call, or a controller that may only be
//     stopped: "Leftovers of a removed or stopped controller (4
//     objects): Service policy-system/policy-svc, which its webhooks
//     call, is gone";
//   - a namespace of workloads parked at zero replicas: "Scaled to 0
//     but still routed in namespace demo (3 workloads): cart, search,
//     wishlist";
//   - a controller without webhooks that the members say is gone (an
//     autoscaler lost it, its release is uninstalling): "Leftovers of
//     removed controller mesh-control in mesh-system: 2 objects".
//
// One object alone keeps its own lead.
func leftoverLead(f caseFacts) (string, bool) {
	objects := f.p.LeftoverObjects()
	switch {
	case objects < 2:
		return "", false
	case f.p.Root.Kind == kube.KindNamespace:
		return parkedLead(f, objects), true
	case !calledByWebhooks(f) && rootRemoved(f):
		return "Leftovers of removed controller " +
			f.leadName(f.p.Root) + ": " + strconv.Itoa(objects) +
			" objects", true
	}
	return "Leftovers of a removed or stopped controller (" +
		strconv.Itoa(objects) + " objects): " + f.leadName(f.p.Root) +
		", which its webhooks call, " + leftoverState(f), true
}

// rootRemoved reports a group whose root the members say is gone: a
// Helm release stuck uninstalling, or a workload an autoscaler lost.
// A controller that may only be stopped keeps the words that say so.
func rootRemoved(f caseFacts) bool {
	if f.p.Root.Kind == kube.KindHelmRelease {
		return true
	}
	for _, m := range f.members {
		if m.Reason == reasons.HPATargetMissing {
			return true
		}
	}
	return false
}

// parkedShown bounds the workloads a parked lead names.
const parkedShown = 3

// parkedLead names the workloads of a namespace that were scaled to
// zero while routes still reach them.
func parkedLead(f caseFacts, objects int) string {
	members, _ := f.p.Leftovers()
	var names []string
	for _, m := range members {
		if n := len(names); n == 0 || names[n-1] != m.Entity.Name {
			names = append(names, m.Entity.Name)
		}
	}
	more := ""
	if len(names) > parkedShown {
		more = " +" + strconv.Itoa(len(names)-parkedShown)
		names = names[:parkedShown]
	}
	return "Scaled to 0 but still routed in namespace " +
		f.p.Root.Name + f.clusterTag() + " (" + strconv.Itoa(objects) +
		" workloads): " + strings.Join(names, ", ") + more
}

// calledByWebhooks reports a webhook among the members that calls the
// group's backend.
func calledByWebhooks(f caseFacts) bool {
	for _, m := range f.members {
		if webhookBackendReason(m.Reason) {
			return true
		}
	}
	return false
}

// webhookBackendReason reports a webhook finding about its backend.
func webhookBackendReason(reason string) bool {
	return reason == reasons.WebhookBackendNotFound ||
		reason == reasons.WebhookNoEndpoints
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
