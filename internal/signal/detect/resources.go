package detect

import (
	"fmt"
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// Resource thresholds.
const (
	limitWarning       = 90.0
	limitCritical      = 95.0
	throttleWarning    = 50.0
	throttleCritical   = 75.0
	cpuSustained       = 10 * time.Minute
	errorRateWarning   = 1.0
	errorRateCritical  = 10.0
	overcommitWarning  = 2.0
	overcommitCritical = 4.0
)

// ContainerResources detects containers close to their memory or CPU
// limit, and containers the kernel throttles heavily.
type ContainerResources struct{}

// Name implements signal.Detector.
func (ContainerResources) Name() string { return "container-resources" }

// Kinds implements signal.Detector.
func (ContainerResources) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindContainer}
}

// Detect implements signal.Detector.
func (ContainerResources) Detect(
	ctx signal.Context, e knowledge.Entity,
) []signal.Signal {
	var out []signal.Signal
	if pct, ok := ratio(e, kube.AttrMemoryWorking, kube.AttrMemoryLimit); ok &&
		pct >= limitWarning {
		out = append(out, levelSignal(pct, constant.ReasonContainerMemoryHigh,
			valueSince(e, kube.AttrMemoryWorking),
			"Memory use is "+percentText(pct)+" of the limit; the "+
				"container is close to being OOM-killed"))
	}
	if pct, ok := ratio(e, kube.AttrCPUUsageMilli, kube.AttrCPULimit); ok &&
		pct >= limitWarning {
		since := valueSince(e, kube.AttrCPULimit)
		if sustained(ctx, since, cpuSustained) {
			out = append(out, signal.Signal{
				Reason:   constant.ReasonContainerCPUHigh,
				Severity: signal.Warning, Since: since,
				Summary: "CPU use is " + percentText(pct) + " of the limit",
			})
		}
	}
	if pct, ok := number(e, kube.AttrThrottledPct); ok &&
		pct >= throttleWarning {
		since := valueSince(e, kube.AttrThrottledPct)
		if sustained(ctx, since, cpuSustained) {
			s := levelSignal(pct, constant.ReasonContainerCPUThrottled, since,
				"CPU is throttled "+percentText(pct)+" of the time; "+
					"requests slow down")
			s.Severity = signal.Warning
			if pct >= throttleCritical {
				s.Severity = signal.Critical
			}
			out = append(out, s)
		}
	}
	return out
}

// PodStorage detects pods close to their ephemeral-storage limit, which
// gets them evicted.
type PodStorage struct{}

// Name implements signal.Detector.
func (PodStorage) Name() string { return "pod-storage" }

// Kinds implements signal.Detector.
func (PodStorage) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindPod}
}

// Detect implements signal.Detector.
func (PodStorage) Detect(
	ctx signal.Context, e knowledge.Entity,
) []signal.Signal {
	used, ok := number(e, kube.AttrEphemeralUsed)
	if !ok {
		return nil
	}
	limit := 0.0
	for _, id := range ctx.Model.Related(e.ID, knowledge.PartOf,
		knowledge.Incoming) {
		if c, ok := ctx.Model.Entity(id); ok {
			value, _ := number(c, kube.AttrEphemeralLimit)
			limit += value
		}
	}
	if limit == 0 {
		return nil
	}
	pct := 100 * used / limit
	if pct < limitWarning {
		return nil
	}
	return []signal.Signal{levelSignal(pct,
		constant.ReasonContainerEphemeralStorageHigh,
		valueSince(e, kube.AttrEphemeralUsed),
		"Local storage use is "+percentText(pct)+" of the limit; the pod "+
			"will be evicted when it is exceeded")}
}

// NodeHealth detects node network and container-runtime error rates and
// nodes whose pods' limits far exceed what the node can provide.
type NodeHealth struct{}

// Name implements signal.Detector.
func (NodeHealth) Name() string { return "node-health" }

// Kinds implements signal.Detector.
func (NodeHealth) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindNode}
}

// Detect implements signal.Detector.
func (NodeHealth) Detect(
	ctx signal.Context, e knowledge.Entity,
) []signal.Signal {
	var out []signal.Signal
	for _, r := range []struct{ attr, reason, what string }{
		{kube.AttrNetErrorRate, constant.ReasonNodeNetworkErrors,
			"network errors"},
		{kube.AttrRuntimeErrRate, constant.ReasonNodeRuntimeErrors,
			"container runtime errors"},
	} {
		rate, ok := number(e, r.attr)
		if !ok || rate < errorRateWarning {
			continue
		}
		severity := signal.Warning
		if rate >= errorRateCritical {
			severity = signal.Critical
		}
		out = append(out, signal.Signal{
			Reason: r.reason, Severity: severity,
			Since: valueSince(e, r.attr),
			Summary: fmt.Sprintf("Node reports %.1f %s per second", rate,
				r.what),
		})
	}
	if s, ok := overcommit(ctx, e); ok {
		out = append(out, s)
	}
	return out
}

// overcommit compares the pods' limits with what the node can provide.
// Limits beyond capacity mean pods are OOM-killed or throttled under load.
func overcommit(
	ctx signal.Context, node knowledge.Entity,
) (signal.Signal, bool) {
	cpuAlloc, _ := number(node, kube.AttrCPUAllocatable)
	memAlloc, _ := number(node, kube.AttrMemoryAllocatable)
	if cpuAlloc <= 0 || memAlloc <= 0 {
		return signal.Signal{}, false
	}
	var cpu, mem float64
	for _, pod := range ctx.Model.Related(node.ID, knowledge.RunsOn,
		knowledge.Incoming) {
		for _, id := range ctx.Model.Related(pod, knowledge.PartOf,
			knowledge.Incoming) {
			if c, ok := ctx.Model.Entity(id); ok {
				limit, _ := number(c, kube.AttrCPULimit)
				cpu += limit
				memory, _ := number(c, kube.AttrMemoryLimit)
				mem += memory
			}
		}
	}
	ratioCPU, ratioMem := cpu/cpuAlloc, mem/memAlloc
	worst := max(ratioCPU, ratioMem)
	if worst < overcommitWarning {
		return signal.Signal{}, false
	}
	reason, severity := constant.ReasonNodeResourceHigh, signal.Warning
	if worst >= overcommitCritical {
		reason, severity = constant.ReasonNodeResourceCritical,
			signal.Critical
	}
	return signal.Signal{
		Reason: reason, Severity: severity,
		Since: valueSince(node, kube.AttrMemoryAllocatable),
		Summary: fmt.Sprintf("Pod limits exceed node capacity: CPU %.1f×, "+
			"memory %.1f×", ratioCPU, ratioMem),
	}, true
}

func ratio(e knowledge.Entity, usedAttr, limitAttr string) (float64, bool) {
	used, ok1 := number(e, usedAttr)
	limit, ok2 := number(e, limitAttr)
	if !ok1 || !ok2 || limit <= 0 {
		return 0, false
	}
	return 100 * used / limit, true
}

func levelSignal(
	pct float64, reason string, since time.Time, summary string,
) signal.Signal {
	severity := signal.Warning
	if pct >= limitCritical {
		severity = signal.Critical
	}
	return signal.Signal{
		Reason: reason, Severity: severity, Since: since, Summary: summary,
	}
}

func percentText(pct float64) string {
	return strconv.Itoa(int(pct)) + "%"
}
