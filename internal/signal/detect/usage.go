package detect

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// Usage thresholds, in percent of capacity.
const (
	usageWarning  = 85.0
	usageCritical = 95.0
	// psiThreshold is the share of the last minute tasks stalled on a
	// resource; above it workloads visibly slow down.
	psiThreshold = 20.0
	// fillWarning and fillCritical bound how soon a growing volume fills.
	fillWarning  = 24 * time.Hour
	fillCritical = 6 * time.Hour
)

// NodeUsage detects nodes running out of disk or inodes and nodes whose
// tasks stall on memory, CPU or IO pressure.
type NodeUsage struct{}

// Name implements signal.Detector.
func (NodeUsage) Name() string { return "node-usage" }

// Kinds implements signal.Detector.
func (NodeUsage) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindNode}
}

// Detect implements signal.Detector.
func (NodeUsage) Detect(_ signal.Context, e knowledge.Entity) []signal.Signal {
	var out []signal.Signal
	if s, ok := threshold(e, kube.AttrFSUsedPct,
		constant.ReasonNodeFilesystemHigh,
		constant.ReasonNodeFilesystemCritical, "disk"); ok {
		out = append(out, s)
	}
	if s, ok := threshold(e, kube.AttrInodesUsedPct,
		constant.ReasonNodeInodesHigh, constant.ReasonNodeInodesCritical,
		"inodes"); ok {
		out = append(out, s)
	}
	for _, p := range []struct{ attr, resource string }{
		{kube.AttrMemoryPSI, "memory"}, {kube.AttrCPUPSI, "CPU"},
		{kube.AttrIOPSI, "disk IO"},
	} {
		stall, ok := number(e, p.attr)
		if !ok || stall < psiThreshold {
			continue
		}
		out = append(out, signal.Signal{
			Reason: constant.ReasonNodePSIHigh, Severity: signal.Warning,
			Since: valueSince(e, p.attr),
			Summary: "Node workloads stall on " + p.resource + " " +
				strconv.Itoa(int(stall)) + "% of the time",
		})
		break
	}
	return out
}

// VolumeUsage detects claims that are nearly full or filling up fast.
type VolumeUsage struct{}

// Name implements signal.Detector.
func (VolumeUsage) Name() string { return "volume-usage" }

// Kinds implements signal.Detector.
func (VolumeUsage) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindPVC}
}

// Detect implements signal.Detector.
func (VolumeUsage) Detect(
	_ signal.Context, e knowledge.Entity,
) []signal.Signal {
	var out []signal.Signal
	if s, ok := threshold(e, kube.AttrVolumeUsedPct,
		constant.ReasonVolumeUsageHigh, constant.ReasonVolumeUsageHigh,
		"storage"); ok {
		out = append(out, s)
	}
	seconds, ok := number(e, kube.AttrVolumeFillETA)
	if !ok {
		return out
	}
	eta := time.Duration(seconds) * time.Second
	if eta > fillWarning {
		return out
	}
	severity := signal.Warning
	if eta <= fillCritical {
		severity = signal.Critical
	}
	used, _ := number(e, kube.AttrVolumeUsedPct)
	return append(out, signal.Signal{
		Reason: constant.ReasonVolumeFillingUp, Severity: severity,
		Since: valueSince(e, kube.AttrVolumeFillETA),
		Summary: "Volume is " + strconv.Itoa(int(used)) +
			"% full and will be full in about " + format.Duration(eta),
	})
}

// threshold reports usage above the warning or critical level.
func threshold(
	e knowledge.Entity, attr, warnReason, critReason, what string,
) (signal.Signal, bool) {
	pct, ok := number(e, attr)
	if !ok || pct < usageWarning {
		return signal.Signal{}, false
	}
	reason, severity := warnReason, signal.Warning
	if pct >= usageCritical {
		reason, severity = critReason, signal.Critical
	}
	return signal.Signal{
		Reason: reason, Severity: severity, Since: valueSince(e, attr),
		Summary: upper(what) + " is " + strconv.Itoa(int(pct)) + "% used",
	}, true
}

func upper(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
