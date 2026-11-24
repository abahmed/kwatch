package explain

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A shared dependency removed on purpose: a job deletes a cache's
// Service and pods, and every workload that uses it stops being ready
// within minutes. Each caller says so in its own words ("UnableToConnect
// on cache.data:6379", "getaddrinfo ENOTFOUND cache.data", "lookup
// cache.data.svc.cluster.local: no such host"), so no shared error line
// ties them, but each names the Service that is gone. When enough
// workloads begin to fail close to the deletion, the deleted Service is
// the one cause of all of them: one incident, rooted at it, instead of
// one per caller.

// ModeRemoved is a Service the failing pods name that was deleted
// shortly before they began to fail: a finer ModeMissing.
const ModeRemoved = ModeMissing + ".Removed"

// RemovedDependencyWindow is how close to the deletion of a Service a
// caller's failure must begin for the deletion to explain it, on either
// side: a job that deletes the pods first and the Service a moment
// later breaks the callers before the Service is gone.
const RemovedDependencyWindow = 5 * time.Minute

// RemovedDependencyMinWorkloads is the fewest workloads that must fail
// on a removed Service for the removal to be their shared cause.
const RemovedDependencyMinWorkloads = 3

// removedServiceRows say that a Service deleted just before the pods
// that name it began to fail explains them all. It is above the missing
// Service rows: the deletion and its time are known, not only the name.
var removedServiceRows = []Row{
	{
		Name: RuleDependencyRemoved,
		Cause: Side{Kind: kube.KindService,
			Modes: []detection.Mode{ModeRemoved}},
		Link:         LinkCalls,
		Effect:       Side{Kind: kube.KindPod, Modes: missingCallFailures},
		Prior:        0.85,
		MinWorkloads: RemovedDependencyMinWorkloads,
	},
}

// removedModes adds ModeRemoved to modes, the missing Service's modes
// for effect, when the Service was deleted within
// RemovedDependencyWindow of when effect began to fail.
func (v *view) removedModes(
	id, effect inventory.EntityID, modes []modeHealth,
) []modeHealth {
	if len(modes) == 0 {
		return modes
	}
	deleted, ok := v.deletedAt(id)
	onset := v.groupOnset(effect)
	if !ok || !onset.ok {
		return modes
	}
	gap := onset.t.Sub(deleted)
	if gap < -RemovedDependencyWindow || gap > RemovedDependencyWindow {
		return modes
	}
	return append(pseudoFailing(ModeRemoved), modes...)
}

// deletedAt is when id was last deleted, as its change history shows.
func (v *view) deletedAt(id inventory.EntityID) (time.Time, bool) {
	var at time.Time
	for _, change := range v.changesOf(id) {
		if change.Deleted && change.At.After(at) {
			at = change.At
		}
	}
	return at, !at.IsZero()
}

// namedMissing are the deleted Services the failing pod's own failed
// calls name by their cluster name ("cache.data:6379", "cache.data.svc
// .cluster.local"), each with the line, quoted as written. A bare name
// ("cache:6379") is too short to be told from a host of anything else,
// and a Service that was never seen is left to the lookup rows: a call
// that got an answer ("connection reset") does not show it missing.
func (v *view) namedMissing(pod inventory.EntityID) []missingRef {
	var out []missingRef
	for _, line := range v.callerLines(pod) {
		call, ok := endpointIn(line)
		host := endpointHost(call.endpoint)
		if !ok || !strings.Contains(host, ".") ||
			temporaryLookup.MatchString(strings.ToLower(line)) {
			continue
		}
		service, named := clusterServiceID(host, pod)
		if !named || !v.namesMissingService(service, pod) {
			continue
		}
		if _, deleted := v.deletedAt(service); deleted &&
			!v.dnsUnwell(pod) {
			out = append(out, missingRef{service: service, line: line})
		}
	}
	return out
}

// callerLines are the error texts of a failing pod (errorLines) and the
// failure lines its containers' own logs hold (ownWordAttrs): a probe
// log line is kept only while the probe fails, so it outlives the
// events that quoted it but never the failure.
func (v *view) callerLines(pod inventory.EntityID) []string {
	out := v.errorLines(pod)
	for _, id := range v.s.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		e, _ := v.s.Model.Entity(id)
		for _, attr := range ownWordAttrs(v.s.Model, id) {
			if line := attrText(e, attr); line != "" {
				out = append(out, line)
			}
		}
	}
	return out
}

// ranWithoutIt reports a pod that ran ready for at least
// RemovedDependencyWindow while service was already missing, before it
// turned unready: the name its configuration holds did not stop it
// then, so it does not explain the failure now. A Service deleted
// after the pod started may.
func (v *view) ranWithoutIt(pod, service inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(pod)
	if !ok || attrBool(e, kube.AttrReady) {
		return false
	}
	started, ok := attrTime(e, kube.AttrContainersStarted)
	unready, known := attrTime(e, kube.AttrReadySince)
	if !ok || !known || unready.Sub(started) < RemovedDependencyWindow {
		return false
	}
	deleted, gone := v.deletedAt(service)
	return !gone || deleted.Before(started)
}
