package detectors

import (
	"fmt"
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Resource thresholds.
const (
	limitWarning       = 90.0
	limitCritical      = 95.0
	throttleWarning    = 50.0
	cpuSustained       = 10 * time.Minute
	memorySustained    = 2 * time.Minute
	errorRateSustained = 3 * time.Minute
	errorRateWarning   = 1.0
	errorRateCritical  = 10.0
	overcommitWarning  = 2.0
	overcommitCritical = 4.0
)

// ContainerResources detects containers close to their memory or CPU
// limit, and containers the kernel throttles heavily.
type ContainerResources struct{}

// Name implements detection.Detector.
func (ContainerResources) Name() string { return "container-resources" }

// Kinds implements detection.Detector.
func (ContainerResources) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindContainer}
}

// Detect implements detection.Detector.
func (ContainerResources) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	if attr, what := memoryReading(e); attr != "" {
		pct, _ := ratio(e, attr, kube.AttrMemoryLimit)
		if pct >= limitWarning {
			// Memory moves with every sample and hovers around the
			// line, so the finding waits for a level that lasts.
			since := ctx.Onset("memory-high", valueSince(e, attr))
			if sustained(ctx, "memory-high", since, memorySustained) {
				s := levelFinding(pct,
					reasons.ContainerMemoryHigh, since, what+" is "+
						percentText(pct)+" of the limit; the container "+
						"is close to being OOM-killed")
				// Close to the limit is only usual, not safe, below the
				// critical level: that stays an interruption.
				used, _ := number(e, attr)
				memoryJudgement(ctx, e, used).
					keepUsual(pct < limitCritical).apply(&s)
				out = append(out, s)
			}
		}
	}
	if pct, ok := ratio(e, kube.AttrCPUUsageMilli, kube.AttrCPULimit); ok &&
		pct >= limitWarning {
		// Usage changes with every sample, so its own Since restarts
		// on each one; the wait runs from when usage first went high.
		since := ctx.Onset("cpu-high", valueSince(e, kube.AttrCPUUsageMilli))
		if sustained(ctx, "cpu-high", since, cpuSustained) {
			out = append(out, detection.Finding{
				Reason:   reasons.ContainerCPUHigh,
				Severity: detection.Warning, Since: since,
				Summary: "CPU use is " + percentText(pct) + " of the limit",
			})
		}
	}
	if pct, ok := number(e, kube.AttrThrottledPct); ok &&
		pct >= throttleWarning {
		since := ctx.Onset("cpu-throttled",
			valueSince(e, kube.AttrThrottledPct))
		if sustained(ctx, "cpu-throttled", since, cpuSustained) {
			s := levelFinding(pct, reasons.ContainerCPUThrottled, since,
				"CPU is throttled "+percentText(pct)+" of the time; "+
					"requests slow down")
			// Throttling slows requests down; it never fails them, so
			// even heavy throttling is only a warning.
			s.Severity = detection.Warning
			out = append(out, s)
		}
	}
	return out
}

// memoryReading picks the memory figure to compare with the limit: the
// resident set (anonymous memory) when the kubelet reports it, because
// file cache is reclaimed before the kernel kills anything; otherwise the
// working set. It returns the attribute and how the text names it, or ""
// when the container has no figure or no limit.
func memoryReading(e inventory.Entity) (attr, what string) {
	attr, what = kube.AttrMemoryRSS, "Resident memory (RSS)"
	if _, ok := ratio(e, attr, kube.AttrMemoryLimit); ok {
		return attr, what
	}
	attr, what = kube.AttrMemoryWorking, "Working set memory"
	if _, ok := ratio(e, attr, kube.AttrMemoryLimit); ok {
		return attr, what
	}
	return "", ""
}

// PodStorage detects pods close to their ephemeral-storage limit, which
// gets them evicted.
type PodStorage struct{}

// Name implements detection.Detector.
func (PodStorage) Name() string { return "pod-storage" }

// Kinds implements detection.Detector.
func (PodStorage) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindPod}
}

// Detect implements detection.Detector.
func (PodStorage) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	used, ok := number(e, kube.AttrEphemeralUsed)
	if !ok {
		return nil
	}
	limit, complete := podEphemeralLimit(ctx, e.ID)
	if !complete || limit == 0 {
		return nil
	}
	pct := 100 * used / limit
	if pct < limitWarning {
		return nil
	}
	return []detection.Finding{levelFinding(pct,
		reasons.ContainerEphemeralStorageHigh,
		valueSince(e, kube.AttrEphemeralUsed),
		"Local storage use is "+percentText(pct)+" of the limit; the pod "+
			"will be evicted when it is exceeded")}
}

// podEphemeralLimit sums the running containers' ephemeral limits. It is
// complete only when every one of them has a limit: pod-wide usage says
// nothing about a limit that some container does not share.
func podEphemeralLimit(
	ctx detection.Context, pod inventory.EntityID,
) (float64, bool) {
	limit, complete := 0.0, true
	for _, c := range runningContainers(ctx, pod) {
		value, ok := number(c, kube.AttrEphemeralLimit)
		complete = complete && ok && value > 0
		limit += value
	}
	return limit, complete
}

// runningContainers returns a pod's app and sidecar containers. Regular
// init containers are excluded: they have finished and hold nothing.
func runningContainers(
	ctx detection.Context, pod inventory.EntityID,
) []inventory.Entity {
	var out []inventory.Entity
	for _, id := range ctx.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		if c, ok := ctx.Model.Entity(id); ok && !flag(c, kube.AttrInit) {
			out = append(out, c)
		}
	}
	return out
}

// NodeHealth detects node network and container-runtime error rates and
// nodes whose pods' limits far exceed what the node can provide.
type NodeHealth struct{}

// Name implements detection.Detector.
func (NodeHealth) Name() string { return "node-health" }

// Kinds implements detection.Detector.
func (NodeHealth) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindNode}
}

// Detect implements detection.Detector.
func (NodeHealth) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	for _, r := range []struct{ attr, reason, what string }{
		{kube.AttrNetErrorRate, reasons.NodeNetworkErrors,
			"network errors"},
		{kube.AttrRuntimeErrRate, reasons.NodeRuntimeErrors,
			"container runtime errors"},
	} {
		rate, ok := number(e, r.attr)
		if !ok || rate < errorRateWarning {
			continue
		}
		since := ctx.Onset("errors/"+r.attr, valueSince(e, r.attr))
		if !sustained(ctx, "errors/"+r.attr, since, errorRateSustained) {
			continue
		}
		severity := detection.Warning
		if rate >= errorRateCritical {
			severity = detection.Critical
		}
		out = append(out, detection.Finding{
			Reason: r.reason, Severity: severity,
			Since: since,
			Summary: fmt.Sprintf("Node reports %.1f %s per second", rate,
				r.what),
		})
	}
	// Pods are still landing on a fresh node: its commitment settles.
	if s, ok := overcommit(ctx, e); ok && !nodeIsYoung(ctx, e) {
		out = append(out, s)
	}
	return out
}

// overcommit compares the pods' limits with what the node can provide.
// Limits beyond capacity mean pods are OOM-killed or throttled under load.
// Finished pods and regular init containers reserve nothing, so they are
// left out; sidecars run beside the app containers and count.
func overcommit(
	ctx detection.Context, node inventory.Entity,
) (detection.Finding, bool) {
	cpuAlloc, _ := number(node, kube.AttrCPUAllocatable)
	memAlloc, _ := number(node, kube.AttrMemoryAllocatable)
	if cpuAlloc <= 0 || memAlloc <= 0 {
		return detection.Finding{}, false
	}
	var cpu, mem float64
	for _, pod := range ctx.Model.Related(node.ID, inventory.RunsOn,
		inventory.Incoming) {
		if p, ok := ctx.Model.Entity(pod); ok && podFinished(p) {
			continue
		}
		for _, c := range runningContainers(ctx, pod) {
			limit, _ := number(c, kube.AttrCPULimit)
			cpu += limit
			memory, _ := number(c, kube.AttrMemoryLimit)
			mem += memory
		}
	}
	ratioCPU, ratioMem := cpu/cpuAlloc, mem/memAlloc
	// Memory between overcommitWarning and overcommitCritical belongs to
	// NodeCommitment (NodeMemoryOvercommitted); counting it here too
	// would announce one condition twice.
	worst := ratioCPU
	if ratioMem >= overcommitCritical {
		worst = max(worst, ratioMem)
	}
	if worst < overcommitWarning {
		return detection.Finding{}, false
	}
	reason, severity := reasons.NodeResourceHigh, detection.Warning
	if worst >= overcommitCritical {
		reason, severity = reasons.NodeResourceCritical,
			detection.Critical
	}
	return detection.Finding{
		Reason: reason, Severity: severity,
		Since: valueSince(node, kube.AttrMemoryAllocatable),
		Summary: fmt.Sprintf("Pod limits exceed node capacity: CPU %.1f×, "+
			"memory %.1f×", ratioCPU, ratioMem),
	}, true
}

func ratio(e inventory.Entity, usedAttr, limitAttr string) (float64, bool) {
	used, ok1 := number(e, usedAttr)
	limit, ok2 := number(e, limitAttr)
	if !ok1 || !ok2 || limit <= 0 {
		return 0, false
	}
	return 100 * used / limit, true
}

func levelFinding(
	pct float64, reason string, since time.Time, summary string,
) detection.Finding {
	severity := detection.Warning
	if pct >= limitCritical {
		severity = detection.Critical
	}
	return detection.Finding{
		Reason: reason, Severity: severity, Since: since, Summary: summary,
	}
}

func percentText(pct float64) string {
	return strconv.Itoa(int(pct)) + "%"
}
