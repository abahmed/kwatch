package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// LinkPreemptedBy links a preempted pod to the workload of the pod the
// scheduler put in its place.
const LinkPreemptedBy LinkType = "preempted-by"

// ModePreempting is the pseudo mode of a workload whose pods preempted
// others. The workload itself is healthy; it is the cause because its
// priority took the capacity.
const ModePreempting detection.Mode = "Preempting"

// RowPreemptor names the row that blames a workload for the pods it
// preempted. A message about it speaks of the victims, not of the
// preemptor.
const RowPreemptor = "preemptor"

// preemptionRows blame the workload that preempted pods. Many victims of
// one preemptor become one incident rooted at that workload.
var preemptionRows = []Row{
	{
		Name: RowPreemptor,
		Cause: Side{Kind: AnyKind,
			Modes: []detection.Mode{ModePreempting}},
		Link: LinkPreemptedBy,
		Effect: podSide(detection.ModePreempted, detection.ModePending,
			detection.ModeUnschedulable),
		Prior: 0.8,
	},
}

// preemptionHops lead from a preempted pod to the workload of its
// preemptor, when the scheduler's message named a pod the model holds. A
// replacement that cannot be scheduled leads to the same workload, so
// the victims and their stuck replacements are one incident.
func (v *view) preemptionHops(pod inventory.EntityID) []hop {
	if v.hasMode(pod, []detection.Mode{detection.ModePreempted}) {
		return v.preemptorHop(pod, pod)
	}
	if !v.hasMode(pod, []detection.Mode{
		detection.ModePending, detection.ModeUnschedulable}) {
		return nil
	}
	for _, sibling := range v.workloadPods(pod) {
		if hops := v.preemptorHop(pod, sibling); len(hops) > 0 {
			return hops
		}
	}
	return nil
}

// preemptorHop is the hop from pod to the workload that preempted
// victim, if victim was preempted and that workload is not pod's own.
func (v *view) preemptorHop(pod, victim inventory.EntityID) []hop {
	for _, f := range v.s.Findings[victim] {
		if f.Mode != detection.ModePreempted {
			continue
		}
		name := evidenceValue(f, detection.EvidencePreemptor)
		ns, short, ok := strings.Cut(name, "/")
		preemptor := inventory.CoreID(kube.KindPod, ns, short)
		if !ok || !v.s.Model.Exists(preemptor) {
			continue
		}
		top := rootcause.TopOwner(v.s.Model, preemptor)
		if top != rootcause.TopOwner(v.s.Model, pod) {
			return []hop{{link: LinkPreemptedBy, to: top}}
		}
	}
	return nil
}

// preemptionModes is ModePreempting for the end of a preempted-by hop.
func preemptionModes(link LinkType) []modeHealth {
	if link != LinkPreemptedBy {
		return nil
	}
	return []modeHealth{{mode: ModePreempting,
		health: detection.Failing, pseudo: true}}
}

func evidenceValue(f detection.Finding, label string) string {
	for _, e := range f.Evidence {
		if e.Label == label {
			return e.Value
		}
	}
	return ""
}
