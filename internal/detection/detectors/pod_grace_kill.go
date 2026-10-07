package detectors

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// graceSlack is how much earlier than the end of the grace period a
// container may have ended and still count as killed by its end: the
// kubelet kills at the deadline and clocks of nodes differ a little.
const graceSlack = 2 * time.Second

// graceKillFindings reports a pod that is being deleted whose container
// did not stop within the termination grace period: the kubelet killed
// it (exit 137, not an out-of-memory kill) at the end of the period.
// It happens at every rollout, scale-down and drain of such a pod, so it
// is only reported for an actual termination, never for a pod whose node
// stopped answering: those pods are killed by the node's loss.
func graceKillFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if ctx.Model == nil || !flag(e, kube.AttrDeleting) ||
		text(e, kube.AttrPhase) == "Unknown" || nodeLost(ctx, e) {
		return nil
	}
	deadline := timestamp(e, kube.AttrDeletionTime)
	grace, _ := number(e, kube.AttrTerminationGrace)
	if deadline.IsZero() || grace <= 0 {
		return nil
	}
	killed, last := killedAtDeadline(ctx, e, deadline)
	if len(killed) == 0 {
		return nil
	}
	evidence := []detection.Evidence{
		{Label: detection.EvidenceGracePeriod,
			Value: strconv.Itoa(int(grace)) + "s"},
		{Label: detection.EvidenceKilledContainers,
			Value: strings.Join(killed, ", ")},
	}
	if duringRollout(ctx, e) {
		evidence = append(evidence, detection.Evidence{
			Label: detection.EvidenceDuringRollout, Value: "true"})
	}
	if hook := stopHookMessage(ctx, e); hook != "" {
		evidence = append(evidence, detection.Evidence{
			Label: detection.EvidenceStopHook, Value: hook})
	}
	return []detection.Finding{{
		Reason: reasons.PodKilledAtGrace, Severity: detection.Warning,
		Since: last,
		Summary: "Pod did not stop within its " +
			strconv.Itoa(int(grace)) + "s grace period and was killed",
		Evidence: evidence,
	}}
}

// killedAtDeadline names the containers of the pod that ended with exit
// 137, not by running out of memory, at or after the grace deadline, and
// when the last of them ended.
func killedAtDeadline(
	ctx detection.Context, e inventory.Entity, deadline time.Time,
) ([]string, time.Time) {
	var names []string
	var last time.Time
	for _, id := range ctx.Model.Related(e.ID, inventory.PartOf,
		inventory.Incoming) {
		c, ok := ctx.Model.Entity(id)
		if !ok || flag(c, kube.AttrInit) ||
			text(c, kube.AttrState) != "terminated" ||
			text(c, kube.AttrStateReason) == reasons.OOMKilled {
			continue
		}
		code, _ := number(c, kube.AttrExitCode)
		finished := timestamp(c, kube.AttrFinished)
		if code != exitSIGKILL || finished.IsZero() ||
			finished.Before(deadline.Add(-graceSlack)) {
			continue
		}
		names = append(names, containerName(id))
		if finished.After(last) {
			last = finished
		}
	}
	sort.Strings(names)
	return names, last
}

// nodeLost reports a pod on a node that is not Ready: its containers
// are killed by the loss of the node, not by a termination.
func nodeLost(ctx detection.Context, e inventory.Entity) bool {
	for _, id := range ctx.Model.Related(e.ID, inventory.RunsOn,
		inventory.Outgoing) {
		node, ok := ctx.Model.Entity(id)
		if !ok {
			continue
		}
		if status, _, _ := condition(node, "Ready"); status != "" &&
			status != "True" {
			return true
		}
	}
	return false
}

// duringRollout reports a pod whose workload is replacing its pods:
// the new revision is not fully updated, or the controller has not seen
// the latest spec.
func duringRollout(ctx detection.Context, e inventory.Entity) bool {
	top := inventory.TopOwner(ctx.Model, e.ID)
	owner, ok := ctx.Model.Entity(top)
	if !ok || top == e.ID {
		return false
	}
	if replacedRevision(ctx, e, owner) {
		return true
	}
	want, known := number(owner, kube.AttrReplicas)
	updated, hasUpdated := number(owner, kube.AttrUpdatedReplicas)
	if known && hasUpdated && updated < want {
		return true
	}
	gen, okGen := number(owner, kube.AttrGeneration)
	seen, okSeen := number(owner, kube.AttrObservedGen)
	return okGen && okSeen && seen < gen
}

// replacedRevision reports a pod of an older pod template than its
// workload's current one: its ReplicaSet is being scaled down.
func replacedRevision(
	ctx detection.Context, e, workload inventory.Entity,
) bool {
	direct := ctx.Model.Related(e.ID, inventory.OwnedBy, inventory.Outgoing)
	if len(direct) == 0 || direct[0] == workload.ID {
		return false
	}
	set, ok := ctx.Model.Entity(direct[0])
	mine, current := text(set, kube.AttrTemplateHash),
		text(workload, kube.AttrTemplateHash)
	return ok && mine != "" && current != "" && mine != current
}

// stopHookMessage is the kubelet's message about a failed preStop hook
// or a failed kill of this pod, as written; "" when there is none.
func stopHookMessage(ctx detection.Context, e inventory.Entity) string {
	for _, note := range ctx.Model.Notes(e.ID, ctx.Now.Add(-EventWindow)) {
		if note.Reason == "FailedPreStopHook" ||
			note.Reason == "FailedKillPod" {
			return note.Message
		}
	}
	return ""
}

// containerName is the container's own name: what follows the pod name
// in its id ("api-0/app").
func containerName(id inventory.EntityID) string {
	if i := strings.LastIndex(id.Name, "/"); i >= 0 {
		return id.Name[i+1:]
	}
	return id.Name
}
