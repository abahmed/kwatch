package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// cascadeLine stands for the call in a restart the liveness probe
// caused: the probe runs the same check as readiness, so what that
// check needs from a dependency is what the kill is about. Nothing the
// pod printed names the dependency.
const cascadeLine = "the liveness probe runs the readiness check"

// cascadeCall is the Service a pod's liveness kills are about: the pod
// is killed by a liveness probe that runs its readiness check, and it is
// configured to call a Service that is failing (no ready endpoint, or
// pods behind it that fail). Configuration alone proves nothing; the
// kill and the failing Service together are the evidence, and the
// solver still requires the Service to have failed first.
func (v *view) cascadeCall(pod inventory.EntityID) (clusterCall, bool) {
	if !v.sameCheckKill(pod) {
		return clusterCall{}, false
	}
	called := v.s.Model.Related(pod, inventory.Calls, inventory.Outgoing)
	sortIDs(called)
	for _, service := range called {
		if service.Kind != kube.KindService {
			continue
		}
		// A Service created later changes the walk.
		v.gate(pod, service)
		if v.s.Model.Exists(service) && v.serviceFailing(service) {
			return clusterCall{service: service, line: cascadeLine,
				cascade: true}, true
		}
	}
	return clusterCall{}, false
}

// sameCheckKill reports a pod a liveness probe keeps killing, where the
// probe runs the same check as readiness. The detector writes the
// evidence on the kill.
func (v *view) sameCheckKill(pod inventory.EntityID) bool {
	for _, f := range v.unitFindings(pod) {
		if !f.Mode.Within(detection.ModeCrashLoopLiveness) {
			continue
		}
		for _, e := range f.Evidence {
			if e.Label == detection.EvidenceLivenessSameCheck {
				return true
			}
		}
	}
	return false
}

// serviceFailing reports a Service with no ready endpoint or with a
// pod behind it that fails.
func (v *view) serviceFailing(service inventory.EntityID) bool {
	return v.noReadyEndpoints(service) || v.backendsFailing(service)
}

// backendsFailing reports a pod behind the Service that fails.
func (v *view) backendsFailing(service inventory.EntityID) bool {
	for backend := range v.selected(service) {
		if v.unitFailing(backend) {
			return true
		}
	}
	return false
}
