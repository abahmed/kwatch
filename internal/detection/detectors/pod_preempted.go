package detectors

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// preemptedMarker ends the scheduler's DisruptionTarget message
// ("batch/importer-x: preempting to accommodate a higher priority pod").
const preemptedMarker = ": preempting to accommodate"

// preemptedByEvent starts the message of the scheduler's Preempted
// event: "Preempted by batch/importer-x on node n1".
const preemptedByEvent = "Preempted by "

// preemptedFindings reports a pod the scheduler preempted while its
// workload is still short of the replicas it wants: the capacity the
// preemption took away has not come back. Once every replica is ready
// again, or the preemption is older than preemptionWindow, there is
// nothing happening and nothing is reported. The preemptor is read from
// the scheduler's own words, never guessed.
func preemptedFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if ctx.Model == nil || !preemptedAt(ctx, e) || !lostCapacity(ctx, e) {
		return nil
	}
	_, _, since := condition(e, "DisruptionTarget")
	var evidence []detection.Evidence
	name := preemptorName(ctx, e)
	if name != "" {
		evidence = preemptorEvidence(ctx, e, name)
	}
	if !since.IsZero() {
		evidence = append(evidence, detection.Evidence{
			Label: detection.EvidencePreemptedAt,
			Value: since.UTC().Format(time.RFC3339)})
	}
	return []detection.Finding{{
		Reason: reasons.PodPreempted, Severity: detection.Warning,
		Since: since,
		Summary: "Pod was preempted by the scheduler for a pod of " +
			"higher priority",
		Evidence: evidence,
	}}
}

// lostCapacity reports whether the pod's workload counts replicas and
// has fewer ready than it wants. A bare pod or a Job's pod is replaced by
// nothing that counts, so its preemption is not judged here.
func lostCapacity(ctx detection.Context, e inventory.Entity) bool {
	top := inventory.TopOwner(ctx.Model, e.ID)
	owner, ok := ctx.Model.Entity(top)
	if top == e.ID || !ok {
		return false
	}
	want, known := number(owner, kube.AttrReplicas)
	if !known {
		return false
	}
	ready, _ := number(owner, kube.AttrReadyReplicas)
	return ready < want
}

// preemptorName is the "namespace/name" of the pod that preempted e: the
// scheduler's condition message, else its Preempted event.
func preemptorName(ctx detection.Context, e inventory.Entity) string {
	message := conditionMessage(e, "DisruptionTarget")
	if before, _, ok := strings.Cut(message, preemptedMarker); ok {
		return podName(before)
	}
	for _, note := range ctx.Model.Notes(e.ID, ctx.Now.Add(-EventWindow)) {
		if note.Reason != kube.PreemptedReason {
			continue
		}
		if after, ok := strings.CutPrefix(note.Message,
			preemptedByEvent); ok {
			if name := podName(strings.Fields(after + " ")[0]); name != "" {
				return name
			}
		}
	}
	return ""
}

// podName accepts "namespace/name" and nothing else: the event of newer
// releases names the preemptor by UID, which says nothing here.
func podName(s string) string {
	s = strings.TrimSpace(s)
	ns, name, ok := strings.Cut(s, "/")
	if !ok || ns == "" || name == "" ||
		strings.ContainsAny(s[len(ns)+1:], "/: ") {
		return ""
	}
	return s
}

// preemptorEvidence names the preemptor, its workload and the two
// priorities, as far as the model knows them.
func preemptorEvidence(
	ctx detection.Context, victim inventory.Entity, name string,
) []detection.Evidence {
	out := []detection.Evidence{{
		Label: detection.EvidencePreemptor, Value: name}}
	ns, short, _ := strings.Cut(name, "/")
	id := inventory.CoreID(kube.KindPod, ns, short)
	pod, ok := ctx.Model.Entity(id)
	if !ok {
		return out
	}
	if top := inventory.TopOwner(ctx.Model, id); top != id {
		out = append(out, detection.Evidence{
			Label: detection.EvidencePreemptorOwner,
			Value: top.Namespace + "/" + top.Name})
	}
	high, okHigh := number(pod, kube.AttrPriority)
	low, okLow := number(victim, kube.AttrPriority)
	if okHigh && okLow {
		out = append(out, detection.Evidence{
			Label: detection.EvidencePriorities,
			Value: strconv.Itoa(int(high)) + " > " + strconv.Itoa(int(low))})
	}
	return out
}
