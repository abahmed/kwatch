package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// appLabels are the pod labels that name the application itself.
var appLabels = map[string]bool{"app": true, "app.kubernetes.io/name": true}

// ownName reports a Service of the pod's namespace named after the pod
// itself: its workload, or the application its labels name. A pod whose
// configuration holds its own name states who it is (a telemetry
// service name, the Service it is meant to back); it does not call
// itself through it, so a missing Service of that name never explains
// the pod's failure. A quoted failed lookup is evidence of its own and
// is not checked here.
func (v *view) ownName(pod, service inventory.EntityID) bool {
	if service.Namespace != pod.Namespace {
		return false
	}
	if rootcause.TopOwner(v.s.Model, pod).Name == service.Name {
		return true
	}
	e, ok := v.s.Model.Entity(pod)
	if !ok {
		return false
	}
	for _, pair := range strings.Split(attrText(e, kube.AttrLabels), ",") {
		key, value, _ := strings.Cut(pair, "=")
		if appLabels[key] && value == service.Name {
			return true
		}
	}
	return false
}
