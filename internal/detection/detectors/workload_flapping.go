package detectors

import (
	"sort"
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Readiness flapping: pods that keep switching between ready and not
// ready. Each switch changes the endpoints of the Service in front of
// them, so clients see connections fail and requests move between pods.
// Nothing crashes, so no other finding says it.
const (
	// flapWindow is how far back a switch still counts.
	flapWindow = 10 * time.Minute
	// flapMinFlips is how many switches, across the workload's pods,
	// inside flapWindow make flapping.
	flapMinFlips = 6
)

// readinessFlapping reports a Deployment, StatefulSet or DaemonSet whose
// pods switched Ready at least flapMinFlips times in flapWindow without
// restarting. A pod's first Ready, pods that restart, pods in a booting
// node pool and young pods of a rollout are the ordinary churn of
// starting, and are not counted.
func readinessFlapping(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	if ctx.Model == nil || !ctx.Synced(kube.KindPod) {
		return detection.Finding{}, false
	}
	var pods []inventory.Entity
	var since time.Time
	downs, total := 0, 0
	for _, id := range runningPodsOf(ctx.Model, e.ID) {
		pod, ok := ctx.Model.Entity(id)
		if !ok || flapExcluded(ctx, pod) {
			continue
		}
		times, down := recentFlips(ctx.Now, pod)
		if len(times) == 0 {
			continue
		}
		pods = append(pods, pod)
		downs, total = downs+down, total+len(times)
		since = earliest(since, times[0])
	}
	if total < flapMinFlips {
		return detection.Finding{}, false
	}
	ctx.RecheckAfter(since.Add(flapWindow).Sub(ctx.Now) + time.Nanosecond)
	finding := detection.Finding{
		Reason: reasons.ReadinessFlapping, Severity: detection.Warning,
		Since: since,
		Summary: "Has pods that went ready to unready " +
			strconv.Itoa(downs) + " times in the last " +
			strconv.Itoa(int(flapWindow/time.Minute)) +
			" min; the endpoints of its " +
			"Service keep changing",
		Evidence: flapEvidence(ctx, e, pods, total),
	}
	flapJudgement(ctx, e, pods).apply(&finding)
	return finding, true
}

// flapJudgement sets the readiness failures of the last hour against
// what the workload usually has: a workload that never flaps is
// unusual when it does.
func flapJudgement(
	ctx detection.Context, e inventory.Entity, pods []inventory.Entity,
) judgement {
	b := baselinesOf(ctx)
	if b == nil {
		return judgement{}
	}
	failures := 0
	for _, pod := range pods {
		for _, id := range podAndContainers(ctx, pod.ID) {
			for _, note := range ctx.Model.Notes(id,
				ctx.Now.Add(-time.Hour)) {
				if isReadinessFailure(note) {
					failures += max(note.Count, 1)
				}
			}
		}
	}
	cmp := b.Compare(e.ID, inventory.WarningMetric(unhealthyEvent),
		float64(failures))
	return judge(cmp, "readiness probe failures "+
		perHour(float64(failures))+" vs a usual "+perHour(cmp.Typical))
}

// earliest is the older of two times, ignoring a zero one.
func earliest(a, b time.Time) time.Time {
	if a.IsZero() || b.Before(a) {
		return b
	}
	return a
}

// flapExcluded reports a pod whose Ready changes say nothing about
// flapping: it is going away, boots with its node pool, starts inside a
// rollout, or restarted inside the window.
func flapExcluded(ctx detection.Context, pod inventory.Entity) bool {
	return flag(pod, kube.AttrDeleting) || bootGraceFor(ctx, pod) > 0 ||
		podStartingInRollout(ctx, pod) || restartedWithin(ctx, pod, flapWindow)
}

// restartedWithin reports a pod with a container whose restart count
// changed inside the window, or that kwatch first saw inside it.
func restartedWithin(
	ctx detection.Context, pod inventory.Entity, window time.Duration,
) bool {
	for _, id := range ctx.Model.Related(pod.ID, inventory.PartOf,
		inventory.Incoming) {
		container, ok := ctx.Model.Entity(id)
		if !ok {
			continue
		}
		if _, known := number(container, kube.AttrRestarts); known &&
			ctx.Now.Sub(valueSince(container, kube.AttrRestarts)) < window {
			return true
		}
	}
	return false
}

// recentFlips are the times inside flapWindow the pod's Ready changed,
// oldest first, and how many of them were to not ready. The first
// change of a pod that was not ready when first seen is its first Ready,
// part of starting, and is left out unless older ones were forgotten.
func recentFlips(now time.Time, pod inventory.Entity) ([]time.Time, int) {
	attribute, ok := pod.Attribute(kube.AttrReady)
	if !ok || len(attribute.Flips) == 0 {
		return nil, 0
	}
	ready, _ := attribute.Value.AsBool()
	flips := attribute.Flips
	// The last flip led to the current value, the one before it away
	// from it, and so on: an even position from the end led to ready
	// when the pod is ready now.
	toReady := func(i int) bool { return (len(flips)-1-i)%2 == 0 == ready }
	first := 0
	if len(flips) < inventory.MaxAttributeFlips && toReady(0) {
		first = 1
	}
	var times []time.Time
	downs := 0
	for i := first; i < len(flips); i++ {
		if now.Sub(flips[i]) >= flapWindow {
			continue
		}
		times = append(times, flips[i])
		if !toReady(i) {
			downs++
		}
	}
	return times, downs
}

// flapEvidence names the pods, the Services whose endpoints change and
// the newest readiness failure the kubelet reported, quoted as written.
func flapEvidence(
	ctx detection.Context, e inventory.Entity, pods []inventory.Entity,
	flips int,
) []detection.Evidence {
	names := make([]string, 0, len(pods))
	for _, pod := range pods {
		names = append(names, pod.ID.Name)
	}
	sort.Strings(names)
	out := []detection.Evidence{
		{Label: "ready changes", Value: strconv.Itoa(flips)},
		{Label: "pods", Value: nameList(names)},
	}
	if services := kube.ServicesSelecting(ctx.Model, e); len(services) > 0 {
		out = append(out, detection.Evidence{Label: "service",
			Value: services[0].Name})
	}
	if message, ok := readinessMessage(ctx, pods); ok {
		out = append(out, detection.Evidence{
			Label: "readiness probe", Value: message})
	}
	return out
}
