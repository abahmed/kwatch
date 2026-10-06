package detectors

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Never-ready thresholds. neverReadyGrace is longer than any ordinary
// start: a pod that has run this long without passing readiness is
// not starting, its probe fails. neverReadyNoteWindow is how far back
// a probe-failure event is still quoted; the kubelet repeats the event
// rarely once a probe has failed for hours.
const (
	neverReadyGrace      = 10 * time.Minute
	neverReadyNoteWindow = 24 * time.Hour
)

// neverReady reports a Deployment or StatefulSet whose pods run, have
// not restarted and still are not ready. Nothing crashes, so no
// container finding fires and the workload's unavailability (a symptom
// that never leads) has no cause to attach to; this finding is the
// cause. It is a warning when no replica is ready and an
// informational digest finding when some are.
func neverReady(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	if e.ID.Kind == kube.KindDaemonSet || ctx.Model == nil ||
		!ctx.Synced(kube.KindPod) {
		return detection.Finding{}, false
	}
	desired, ok := desiredReplicas(e)
	ready, _ := number(e, kube.AttrReadyReplicas)
	if !ok || desired == 0 || ready >= desired {
		return detection.Finding{}, false
	}
	pods, ok := runningUnreadyPods(ctx, e.ID)
	if !ok {
		return detection.Finding{}, false
	}
	since := unreadySince(pods)
	wait := max(neverReadyGrace, longestStartupBudget(ctx, pods)) +
		workloadReplacementGrace(ctx, e.ID)
	if !sustained(ctx, "never-ready", since, wait) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.WorkloadNeverReady, Severity: neverReadySeverity(
			ready), Since: since,
		Summary: "Has pods that run without restarting but are not " +
			"ready; " + strconv.Itoa(int(ready)) + " of " +
			strconv.Itoa(int(desired)) + " replicas are ready, and the " +
			"others have not passed readiness for " +
			format.Duration(ctx.Now.Sub(since)),
		Evidence: append(neverReadyEvidence(ctx, pods, ready, desired),
			neverHealthyEvidence(ctx, e)...),
		// Replicas that still serve make this a risk to name in the
		// digest; the unready pod's own findings are the failure.
		Advisory: ready > 0,
	}, true
}

// neverReadySeverity is a warning when nothing serves, and only digest
// news when some replicas still do.
func neverReadySeverity(ready float64) detection.Severity {
	if ready == 0 {
		return detection.Warning
	}
	return detection.Info
}

// runningUnreadyPods lists the workload's pods that are not ready. It
// reports false when there is none, or when any of them is not simply
// running: a pending, terminating, crashing or restarted pod is a
// different problem with its own findings.
func runningUnreadyPods(
	ctx detection.Context, workload inventory.EntityID,
) ([]inventory.Entity, bool) {
	var out []inventory.Entity
	for _, id := range runningPodsOf(ctx.Model, workload) {
		pod, ok := ctx.Model.Entity(id)
		if !ok || flag(pod, kube.AttrReady) {
			continue
		}
		if text(pod, kube.AttrPhase) != "Running" ||
			flag(pod, kube.AttrDeleting) || podFailingToRun(ctx, id) ||
			onUnreadyNode(ctx, id) {
			return nil, false
		}
		out = append(out, pod)
	}
	return out, len(out) > 0
}

// onUnreadyNode reports a pod on a node that is not Ready: the node's
// own findings explain the pod, which is unready because of it.
func onUnreadyNode(ctx detection.Context, pod inventory.EntityID) bool {
	for _, id := range ctx.Model.Related(pod, inventory.RunsOn,
		inventory.Outgoing) {
		node, ok := ctx.Model.Entity(id)
		if !ok {
			continue
		}
		if status, _, _ := condition(node, "Ready"); status != "True" {
			return true
		}
	}
	return false
}

// unreadySince is when the newest of the pods became unready, as the
// kubelet recorded it, so a restart of kwatch does not restart the wait.
func unreadySince(pods []inventory.Entity) time.Time {
	var since time.Time
	for _, pod := range pods {
		since = latest(since, timestamp(pod, kube.AttrReadySince))
	}
	return since
}

func longestStartupBudget(
	ctx detection.Context, pods []inventory.Entity,
) time.Duration {
	var longest time.Duration
	for _, pod := range pods {
		longest = max(longest, startupBudget(ctx, pod.ID))
	}
	return longest
}

func neverReadyEvidence(
	ctx detection.Context, pods []inventory.Entity, ready, desired float64,
) []detection.Evidence {
	names := make([]string, 0, len(pods))
	for _, pod := range pods {
		names = append(names, pod.ID.Name)
	}
	sort.Strings(names)
	out := []detection.Evidence{
		{Label: "ready", Value: strconv.Itoa(int(ready)) + "/" +
			strconv.Itoa(int(desired))},
		{Label: "pods not ready", Value: nameList(names)},
	}
	if message, ok := readinessMessage(ctx, pods); ok {
		out = append(out, detection.Evidence{
			Label: "readiness probe", Value: message})
	}
	return out
}

// readinessMessage is the newest readiness probe failure the kubelet
// reported for the pods or their containers, quoted as it was written.
func readinessMessage(
	ctx detection.Context, pods []inventory.Entity,
) (string, bool) {
	var newest inventory.Note
	found := false
	since := ctx.Now.Add(-neverReadyNoteWindow)
	for _, pod := range pods {
		for _, id := range podAndContainers(ctx, pod.ID) {
			for _, note := range ctx.Model.Notes(id, since) {
				if isReadinessFailure(note) &&
					(!found || note.At.After(newest.At)) {
					newest, found = note, true
				}
			}
		}
	}
	return newest.Message, found
}

func podAndContainers(
	ctx detection.Context, pod inventory.EntityID,
) []inventory.EntityID {
	ids := []inventory.EntityID{pod}
	return append(ids, ctx.Model.Related(pod, inventory.PartOf,
		inventory.Incoming)...)
}

func isReadinessFailure(note inventory.Note) bool {
	return note.Warning && note.Reason == unhealthyEvent &&
		strings.HasPrefix(note.Message, "Readiness probe")
}
