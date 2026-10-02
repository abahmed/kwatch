package detectors

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
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

// Name implements detection.Detector.
func (NodeUsage) Name() string { return "node-usage" }

// Kinds implements detection.Detector.
func (NodeUsage) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindNode}
}

// Detect implements detection.Detector.
func (NodeUsage) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	if s, ok := threshold(ctx, e, kube.AttrFSUsedPct,
		reasons.NodeFilesystemHigh, "disk"); ok {
		out = append(out, s)
	}
	if s, ok := threshold(ctx, e, kube.AttrInodesUsedPct,
		reasons.NodeInodesHigh, "inodes"); ok {
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
		out = append(out, detection.Finding{
			Reason: reasons.NodePSIHigh, Severity: detection.Warning,
			Since:   valueSince(e, p.attr),
			Summary: "Node workloads stall on " + p.resource,
			Evidence: []detection.Evidence{{
				Label: "stalled",
				Value: strconv.Itoa(int(stall)) + "% of the last minute",
			}},
		})
		break
	}
	return out
}

// VolumeUsage detects claims that are nearly full or filling up fast.
type VolumeUsage struct{}

// Name implements detection.Detector.
func (VolumeUsage) Name() string { return "volume-usage" }

// Kinds implements detection.Detector.
func (VolumeUsage) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindPVC}
}

// Detect implements detection.Detector.
func (VolumeUsage) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	if s, ok := threshold(ctx, e, kube.AttrVolumeUsedPct,
		reasons.VolumeUsageHigh, "storage"); ok {
		out = append(out, s)
	}
	seconds, ok := number(e, kube.AttrVolumeFillETA)
	// A negative or very large estimate is not a fill time; converting it
	// to a Duration could overflow into a negative "full now".
	if !ok || seconds < 0 || seconds > fillWarning.Seconds() {
		return out
	}
	eta := time.Duration(seconds) * time.Second
	severity := detection.Warning
	if eta <= fillCritical {
		severity = detection.Critical
	}
	summary := "Volume will be full within a day"
	if severity == detection.Critical {
		summary = "Volume will be full within " +
			format.Duration(fillCritical)
	}
	used, _ := number(e, kube.AttrVolumeUsedPct)
	return append(out, detection.Finding{
		Reason: reasons.VolumeFillingUp, Severity: severity,
		Since:   valueSince(e, kube.AttrVolumeFillETA),
		Summary: summary,
		Evidence: []detection.Evidence{
			{Label: "used", Value: strconv.Itoa(int(used)) + "%"},
			{Label: "full in", Value: "about " + format.Duration(eta)},
		},
	})
}

// usageHysteresis is how far usage must fall below a level it crossed
// before the level counts as left, so a value hovering around 95% does
// not flip between warning and critical with every sample.
const usageHysteresis = 5.0

// threshold reports usage above the warning level as one finding whose
// severity rises to critical at the critical level. Both levels have
// hysteresis, and the finding dates from when usage first crossed the
// warning level, not from the latest sample.
func threshold(
	ctx detection.Context, e inventory.Entity, attr, reason, what string,
) (detection.Finding, bool) {
	pct, ok := number(e, attr)
	if !ok || !crossed(ctx, "high:"+attr, pct, usageWarning) {
		return detection.Finding{}, false
	}
	since := ctx.Onset("high:"+attr, valueSince(e, attr))
	severity, level := detection.Warning, usageWarning
	if crossed(ctx, "critical:"+attr, pct, usageCritical) {
		ctx.Onset("critical:"+attr, time.Time{})
		severity, level = detection.Critical, usageCritical
	}
	// The summary names the crossed level; the exact figure moves with
	// every sample and stays in the evidence.
	return detection.Finding{
		Reason: reason, Severity: severity, Since: since,
		Summary: upper(what) + " is over " + strconv.Itoa(int(level)) +
			"% used",
		Evidence: []detection.Evidence{{
			Label: "used", Value: strconv.Itoa(int(pct)) + "%",
		}},
	}, true
}

// crossed reports whether pct is at or over level, or was over it at
// the previous evaluation and has not yet fallen usageHysteresis below.
func crossed(
	ctx detection.Context, key string, pct, level float64,
) bool {
	if pct >= level {
		return true
	}
	return ctx.Ongoing(key) && pct > level-usageHysteresis
}

func upper(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
