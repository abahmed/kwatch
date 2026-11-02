package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Readiness is how a workload is doing, read from the model.
type Readiness struct {
	Desired, Ready float64
	// Restarts is the sum of the restart counts of its containers.
	Restarts float64
	// Failing is true when a pod of the workload is not ready and one of
	// its containers has restarted.
	Failing bool
}

// Short reports a workload with fewer ready replicas than desired.
func (r Readiness) Short() bool {
	return r.Desired > 0 && r.Ready < r.Desired
}

// IsApplicationKind reports the kinds that run an application's own
// code: a pod, a container, or a workload that owns pods. Proof about
// one of their runs speaks of the application, not of the
// infrastructure under it.
func IsApplicationKind(kind inventory.Kind) bool {
	switch kind {
	case kube.KindPod, kube.KindContainer, kube.KindReplicaSet,
		kube.KindJob, kube.KindCronJob:
		return true
	}
	return IsWorkload(kind)
}

// IsWorkload reports the kinds that have desired and ready replicas.
func IsWorkload(kind inventory.Kind) bool {
	return kind == kube.KindDeployment || kind == kube.KindStatefulSet ||
		kind == kube.KindDaemonSet
}

// ReplicasOf reads the desired and ready replica counts of a Deployment,
// StatefulSet or DaemonSet. It reports false for anything else or when
// the object is not in the model.
func ReplicasOf(
	model inventory.Reader, id inventory.EntityID,
) (desired, ready float64, ok bool) {
	if !IsWorkload(id.Kind) {
		return 0, 0, false
	}
	e, found := model.Entity(id)
	if !found {
		return 0, 0, false
	}
	desiredAttr := kube.AttrReplicas
	if id.Kind == kube.KindDaemonSet {
		desiredAttr = kube.AttrDesiredReplicas
	}
	return attrNumber(e, desiredAttr),
		attrNumber(e, kube.AttrReadyReplicas), true
}

// ReadinessOf adds the state of the workload's pods to its replica
// counts. It reports false when ReplicasOf does.
func ReadinessOf(
	model inventory.Reader, id inventory.EntityID,
) (Readiness, bool) {
	desired, ready, ok := ReplicasOf(model, id)
	if !ok {
		return Readiness{}, false
	}
	r := Readiness{Desired: desired, Ready: ready}
	for _, pod := range rootcause.OwnedPods(model, id) {
		restarts := podRestarts(model, pod)
		r.Restarts += restarts
		if restarts > 0 && !podReady(model, pod) {
			r.Failing = true
		}
	}
	return r, true
}

// podRestarts sums the restart counts of a pod's containers.
func podRestarts(model inventory.Reader, pod inventory.EntityID) float64 {
	total := 0.0
	for _, c := range model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		if e, ok := model.Entity(c); ok && c.Kind == kube.KindContainer {
			total += attrNumber(e, kube.AttrRestarts)
		}
	}
	return total
}

func podReady(model inventory.Reader, pod inventory.EntityID) bool {
	e, ok := model.Entity(pod)
	if !ok {
		return false
	}
	a, ok := e.Attribute(kube.AttrReady)
	if !ok {
		return false
	}
	ready, _ := a.Value.AsBool()
	return ready
}

// workloadFor names the workload that owns a root: an autoscaler's
// target, a pod's controller, or the root itself.
func workloadFor(
	model inventory.Reader, root inventory.EntityID,
) inventory.EntityID {
	if root.Kind == kube.KindHPA {
		for _, target := range model.Related(root, inventory.Scales,
			inventory.Outgoing) {
			return target
		}
		return root
	}
	return defaultRoot(model, root)
}

// stillBroken reports that the workload behind p's root is below its
// desired replicas while its pods keep failing. Such an incident may not
// resolve on a quiet timer: the failure only moved out of its members.
// The hold is bounded: StillBrokenMax after the incident lost its last
// member it ends anyway, so one pod no detector flags cannot keep an
// incident open for ever.
func (m *Manager) stillBroken(p *Incident, now time.Time) bool {
	return now.Before(brokenHoldEnd(p)) &&
		(m.workloadBroken(p) || m.crashing(p, now))
}

// workloadBroken reports a workload below its desired replicas whose
// pods keep failing, whatever the hold says.
func (m *Manager) workloadBroken(p *Incident) bool {
	if m.model == nil {
		return false
	}
	// A Service is as broken as the workloads behind it.
	for _, w := range workloadsBehind(m.model, workloadFor(m.model, p.Root)) {
		if r, ok := ReadinessOf(m.model, w); ok && r.Short() && r.Failing {
			return true
		}
	}
	return false
}

// StillCrashingNote ends the reason of a resolve at the StillBrokenMax
// cap that held only because a container kept crashing, the workload
// being at its replicas. Without it the cap held on a workload short of
// replicas. Messages word their lead from it.
const StillCrashingNote = "; container still crashing"

// resolveReason is why a recovering or flapping incident ends now. It
// says what held, unless the workload is still broken: the cap ended the
// hold and not a recovery, so the reason must not claim health. It
// carries which condition held at the cap.
func (m *Manager) resolveReason(
	p *Incident, now time.Time, healthy Reason,
) Reason {
	stopped := StoppedTrackingPrefix + format.Duration(StillBrokenMax) +
		"; coverage check continues"
	switch {
	case m.workloadBroken(p):
		return Reason(stopped)
	case m.crashing(p, now):
		return Reason(stopped + StillCrashingNote)
	}
	return healthy
}

// brokenHoldEnd is when the extra hold of stillBroken ends.
func brokenHoldEnd(p *Incident) time.Time {
	return p.RecoveringSince.Add(StillBrokenMax)
}

// holdEnd is when a recovering incident, whose hold ends at due, may
// resolve at the latest: due, or the end of the extra hold when a broken
// workload keeps it past due. The timer of such an incident is armed for
// that end.
func (m *Manager) holdEnd(p *Incident, due, now time.Time) time.Time {
	if !now.Before(due) && m.stillBroken(p, now) {
		return brokenHoldEnd(p)
	}
	return due
}
