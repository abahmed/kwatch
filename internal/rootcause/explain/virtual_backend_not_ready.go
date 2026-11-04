package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// ModeBackendsNotReady is a workload behind a Service whose pods run but
// are not ready while the effect calls the Service, before anything
// about those pods has a finding of its own. A finer mode names the
// Service like ModeBackendsFailing. The Service's empty endpoints only
// repeat that its pods are not ready: the workload is where the cause
// is, but nothing yet says why its pods are not ready, so the row's
// prior keeps the verdict below high confidence.
const ModeBackendsNotReady detection.Mode = "BackendsNotReady"

// backendNotReadyRow says that the workload whose pods are the not-ready
// backends of the Service the error names explains the callers.
var backendNotReadyRow = Row{
	Name: "called-service-backends-not-ready",
	Cause: Side{Group: AnyGroup, Kind: AnyKind,
		Modes: []detection.Mode{ModeBackendsNotReady}},
	Link:   LinkBacks,
	Effect: Side{Kind: kube.KindPod, Modes: callerFailures},
	Prior:  0.5,
}

// notReadyBackendMode is the pseudo mode of a workload one of whose
// pods is a not-ready backend of service, gated on those pods so a pod
// turning ready again solves the effect again.
func (v *view) notReadyBackendMode(
	id, effect, service inventory.EntityID, named detection.Mode,
) []modeHealth {
	if id.Kind == kube.KindPod {
		return nil
	}
	backends := v.selected(service)
	for _, pod := range rootcause.OwnedPods(v.s.Model, id) {
		v.gate(effect, pod)
		if backends[pod] && v.podNotReady(pod) {
			return pseudoFailing(ModeBackendsNotReady + named)
		}
	}
	return nil
}

// backendsNotReady reports a Service at least one of whose selected
// pods belongs to a workload and runs without being ready: its empty
// endpoints are those pods' state, not a fault of the Service.
func (v *view) backendsNotReady(service inventory.EntityID) bool {
	for _, pod := range sortedKeys(v.selected(service)) {
		if rootcause.TopOwner(v.s.Model, pod) != pod &&
			v.podNotReady(pod) {
			return true
		}
	}
	return false
}

// podNotReady reports a pod that is known not to be ready and is neither
// finished nor going away, and that does not itself call a Service with
// no ready endpoint: such a pod may be not ready because of what it
// calls, so its readiness says nothing of its own.
func (v *view) podNotReady(id inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(id)
	if !ok || id.Kind != kube.KindPod || v.callsEmptyService(e) {
		return false
	}
	if _, has := e.Attribute(kube.AttrReady); !has {
		return false
	}
	ready, _ := attributeValue(e, kube.AttrReady).AsBool()
	deleting, _ := attributeValue(e, kube.AttrDeleting).AsBool()
	phase := attrText(e, kube.AttrPhase)
	return !ready && !deleting && phase != "Succeeded" && phase != "Failed"
}

// withoutRepeatedEmptiness drops the no-endpoints finding of a Service
// a failing pod calls when its backends are not-ready pods of a
// workload: it repeats their state, so the Service is not a cause of its
// own. The workload is reached by the backs link instead. An init
// container that waits for the Service keeps it as its cause: the
// init-wait row names the Service it waits for.
func (v *view) withoutRepeatedEmptiness(
	id, effect inventory.EntityID, link LinkType, modes []modeHealth,
) []modeHealth {
	if link != LinkCalls || id.Kind != kube.KindService ||
		v.initContainer(effect) || !v.backendsNotReady(id) {
		return modes
	}
	var out []modeHealth
	for _, mh := range modes {
		if !mh.mode.Within(detection.ModeNoEndpoints) {
			out = append(out, mh)
		}
	}
	return out
}

// notReadyBackendRow says that the not-ready pods of a workload explain
// the Service they back when it has no ready endpoint and nothing about
// the pods has a finding yet: the Service only repeats their state.
var notReadyBackendRow = Row{
	Name: "backends-not-ready",
	Cause: Side{Kind: kube.KindPod,
		Modes: []detection.Mode{ModeBackendsNotReady}},
	Link: LinkBacks,
	Effect: Side{Kind: kube.KindService,
		Modes: []detection.Mode{detection.ModeNoEndpoints}},
	Prior: 0.5,
}

// servedBackendModes is the pseudo mode of a pod of a workload that
// backs the effect Service and is not ready, while nothing about the pod
// has a finding: a failing pod is the backends-failing row's.
func (v *view) servedBackendModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkBacks || id.Kind != kube.KindPod ||
		effect.Kind != kube.KindService ||
		rootcause.TopOwner(v.s.Model, id) == id || !v.podNotReady(id) ||
		v.unitFailing(id) {
		return nil
	}
	return pseudoFailing(ModeBackendsNotReady)
}

// callsEmptyService reports a pod whose configuration names a Service
// of the cluster that exists and has no ready endpoint.
func (v *view) callsEmptyService(pod inventory.Entity) bool {
	for _, call := range kube.ServiceCalls(pod) {
		v.gate(pod.ID, call.Service)
		if v.s.Model.Exists(call.Service) &&
			v.noReadyEndpoints(call.Service) {
			return true
		}
	}
	return false
}

// initContainer reports an init container.
func (v *view) initContainer(id inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(id)
	return ok && id.Kind == kube.KindContainer && attrBool(e, kube.AttrInit)
}
