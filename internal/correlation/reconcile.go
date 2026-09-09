package correlation

import (
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

// Container-level problems had no recovery branch. Every other kind of
// incident is closed by its detector noticing the condition is gone -- a
// Deployment's detector re-runs on each informer resync and calls
// Resolve once replicas are available -- but a crashing container had
// nothing equivalent. Its incident could only die of silence: no report for a
// whole correlation window, closed with "the underlying problem was not
// observed to recover". For the single most common alert kwatch sends, the
// resolve was always late and always hedged.
//
// This is the missing half. When a container is running and ready again, the
// incidents about it are resolved the same way every other observed recovery
// is -- through markResolved, so the resolve hold-down still applies and a
// container that crashes again during the hold-down cancels it instead of
// producing a premature "✅ resolved".
//
// resolvableContainerReasons is deliberately not every container reason.
// Excluded are the historical aggregates -- HighRestartCount, OOMRepeating,
// CrashLoopHighFreq -- whose claim is "this has been bad for a long time".
// A container that has been ready for five minutes does not falsify that, so
// those keep closing on silence.
var resolvableContainerReasons = map[string]bool{
	constant.ReasonCrashLoopBackOff:     true,
	constant.ReasonBackOff:              true,
	constant.ReasonError:                true,
	constant.ReasonOOMKilled:            true,
	constant.ReasonCreateContainerError: true,
	constant.ReasonContainerCannotRun:   true,
	constant.ReasonInitContainerError:   true,
	constant.ReasonStartupProbeFailed:   true,
	constant.ReasonLivenessProbeFailed:  true,
	constant.ReasonReadinessProbeFailed: true,
	constant.ReasonProbeError:           true,
	constant.ReasonPostStartHookError:   true,
	constant.ReasonPreStopHookError:     true,
	constant.ReasonImagePullBackOff:     true,
	constant.ReasonErrImagePull:         true,
	constant.ReasonImageInspectError:    true,
	constant.ReasonInvalidImageName:     true,
	constant.ReasonRegistryUnavailable:  true,
	constant.ReasonCreateConfigError:    true,
}

// resolvablePodReasons are the pod-level reasons a Pod becoming ready
// falsifies. They had no recovery path at all: with the Pod still present the
// stale sweep held them open for four correlation windows and then closed
// them as "not observed to recover", so an unschedulable Pod that was
// scheduled forty minutes ago still showed as failing.
//
// PodFailed is deliberately absent. A Pod that reached the Failed phase does
// not come back; a replacement is a different Pod, and closing the incident
// because the replacement is healthy would hide the failure that happened.
var resolvablePodReasons = map[string]bool{
	constant.ReasonUnschedulable:             true,
	constant.ReasonPodPending:                true,
	constant.ReasonContainersNotReady:        true,
	constant.ReasonSchedulingGated:           true,
	constant.ReasonNodeAffinity:              true,
	constant.ReasonPodStuckTerminating:       true,
	constant.ReasonProjectedSecretMissing:    true,
	constant.ReasonProjectedConfigMapMissing: true,
	constant.ReasonServiceAccountMissing:     true,
}

// ResolveHealthyPodContainers resolves the container incidents for one Pod
// whose containers are running and ready again. healthy maps container name
// to whether it has recovered; an empty map does nothing.
//
// It looks the incidents up rather than rebuilding their keys. Key rebuilding
// is what a caller in the handler package would have to do, and it cannot: an
// ownerless Pod's incident is keyed by a UID or lineage hash the handler does
// not know, so every such resolve would silently miss.
func (e *Engine) ResolveHealthyPodContainers(
	namespace, podName string,
	healthy map[string]bool,
) {
	if len(healthy) == 0 || podName == "" {
		return
	}
	keys := e.recoveredContainerIncidents(namespace, podName, healthy)
	// markResolved takes the lock and applies the hold-down, owner-health and
	// grouping rules that every other observed recovery goes through.
	for _, key := range keys {
		e.markResolved(key)
	}
}

// recoveredContainerIncidents collects the incident keys that this Pod's
// recovery accounts for.
func (e *Engine) recoveredContainerIncidents(
	namespace, podName string,
	healthy map[string]bool,
) []model.IncidentKey {
	e.mu.Lock()
	defer e.mu.Unlock()
	// The namespace index keeps this proportional to the incidents in one
	// namespace, not to every incident in the cluster: this runs for every
	// healthy Pod on every resync, and in a namespace with nothing wrong it
	// costs a single map lookup.
	var keys []model.IncidentKey
	for key, inc := range e.namespaceIndex[namespace] {
		if !e.podRecoveryResolves(inc, podName, healthy) {
			continue
		}
		keys = append(keys, key)
	}
	return keys
}

// podRecoveryResolves reports whether one Pod becoming healthy accounts for
// the whole incident. Caller must hold e.mu.
func (e *Engine) podRecoveryResolves(
	inc *model.Incident,
	podName string,
	healthy map[string]bool,
) bool {
	if inc == nil || inc.Resource != "pod" {
		return false
	}
	if inc.State == model.StateResolved ||
		inc.State == model.StatePendingResolve {
		return false
	}
	switch {
	case resolvableContainerReasons[inc.Reason]:
		// A container-scoped incident is only answered by that container.
		// The empty name is the Pod itself, which the caller reports as
		// recovered only when every container is ready.
		if !healthy[inc.ContainerName] {
			return false
		}
	case resolvablePodReasons[inc.Reason]:
		// A pod-level reason is about the Pod, not one container, so it is
		// answered only by the Pod as a whole being ready -- which is what
		// the caller reports under the empty container name.
		if !healthy[""] || !isPodScoped(inc.ContainerName) {
			return false
		}
	default:
		return false
	}
	return e.incidentCoveredByPod(inc, podName)
}

// isPodScoped reports whether an incident is about the Pod itself rather
// than one of its containers. "." is the pipeline's spelling for "the pod".
func isPodScoped(container string) bool {
	return container == "" || container == "."
}

// incidentCoveredByPod reports whether podName is the only Pod the incident
// still concerns.
//
// An incident aggregates every Pod that failed the same way, so one Pod
// recovering says nothing about the others: resolving on it would close an
// incident while four replicas are still crashing. The other Pods count only
// if they are gone -- the scale-down and replacement cases -- which is what
// the presence check can answer. Anything it cannot answer is treated as
// still there, so the incident stays open and closes on silence as before.
// Caller must hold e.mu.
func (e *Engine) incidentCoveredByPod(
	inc *model.Incident,
	podName string,
) bool {
	if len(inc.Resources) == 0 {
		return inc.Ref() == model.ObjectRef{
			Kind: "pod", Namespace: inc.Namespace, Name: podName,
		}
	}
	if !inc.Resources[podName] {
		return false
	}
	for other := range inc.Resources {
		if other == podName {
			continue
		}
		if e.config.SubjectPresent == nil {
			return false // cannot tell; leave the incident open
		}
		exists, known := e.config.SubjectPresent("pod", inc.Namespace, other)
		if exists || !known {
			return false
		}
	}
	return true
}
