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

// Usage thresholds, in percent of capacity.
const (
	usageWarning  = 85.0
	usageCritical = 95.0
	// psiThreshold is the share of the last minute tasks stalled on a
	// resource; above it workloads visibly slow down.
	psiThreshold = 20.0
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
	out := kubeletFindings(ctx, e)
	if s, ok := threshold(ctx, e, kube.AttrFSUsedPct,
		reasons.NodeFilesystemHigh, "disk"); ok {
		out = append(out, s)
	}
	if s, ok := threshold(ctx, e, kube.AttrInodesUsedPct,
		reasons.NodeInodesHigh, "inodes"); ok {
		out = append(out, s)
	}
	if !nodeIsYoung(ctx, e) {
		out = append(out, stallFindings(e)...)
	}
	return out
}

// stallFindings reports the first resource the node's tasks stall on.
func stallFindings(e inventory.Entity) []detection.Finding {
	for _, p := range []struct{ attr, resource string }{
		{kube.AttrMemoryPSI, "memory"}, {kube.AttrCPUPSI, "CPU"},
		{kube.AttrIOPSI, "disk IO"},
	} {
		stall, ok := number(e, p.attr)
		if !ok || stall < psiThreshold {
			continue
		}
		return []detection.Finding{{
			Reason: reasons.NodePSIHigh, Severity: detection.Warning,
			Since: valueSince(e, p.attr),
			Summary: "Node is under " + p.resource + " pressure: " +
				"workloads stall on " + p.resource,
			Evidence: []detection.Evidence{{
				Label: "stalled",
				Value: strconv.Itoa(int(stall)) + "% of the last minute",
			}},
		}}
	}
	return nil
}

// VolumeUsage detects claims that are nearly full.
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
	if s, ok := volumeInodes(ctx, e); ok {
		out = append(out, s)
	}
	for i := range out {
		out[i].Evidence = append(out[i].Evidence, volumeDetail(ctx, e)...)
	}
	return out
}

// volumeInodes reports a claim that has used most of its inodes: the
// bytes may be plentiful, but every new file fails with "No space left
// on device". It is the same finding as bytes running out, from
// another count, so it uses the same levels.
func volumeInodes(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	f, ok := threshold(ctx, e, kube.AttrVolumeInodesPct,
		reasons.VolumeInodesHigh, "volume inodes")
	if !ok {
		return f, false
	}
	level := usageWarning
	if f.Severity == detection.Critical {
		level = usageCritical
	}
	f.Summary = "Volume has used over " + strconv.Itoa(int(level)) +
		"% of its inodes"
	// The share is of inodes, not bytes: a message about bytes must not
	// read it as its own.
	f.Evidence = []detection.Evidence{{
		Label: detection.EvidenceVolumeInodes,
		Value: f.Evidence[0].Value,
	}}
	return f, true
}

// usageHysteresis is how far usage must fall below a level it crossed
// before the level counts as left, so a value hovering around 95% does
// not flip between warning and critical with every sample.
const usageHysteresis = 5.0

// usageHoldMax is how long a usage finding is held without a reading.
const usageHoldMax = 30 * time.Minute

// threshold reports usage above the warning level as one finding whose
// severity rises to critical at the critical level. Both levels have
// hysteresis, and the finding dates from when usage first crossed the
// warning level, not from the latest sample.
func threshold(
	ctx detection.Context, e inventory.Entity, attr, reason, what string,
) (detection.Finding, bool) {
	pct, ok := number(e, attr)
	if !ok {
		return heldUsage(ctx, e, attr, reason, what)
	}
	if !crossed(ctx, "high:"+attr, pct, usageWarning) {
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

// heldUsage keeps a usage finding alive while its reading is gone only
// because the kubelet cannot be read. The model drops usage figures after
// a few failed polls; the disk did not empty meanwhile, so a finding
// that held at the previous evaluation holds now, with the severity it
// had, until the kubelet answers again or usageHoldMax has passed. The
// hold ends because an old reading says less and less about the disk: a
// node that stays unreachable is a finding of its own.
func heldUsage(
	ctx detection.Context, e inventory.Entity, attr, reason, what string,
) (detection.Finding, bool) {
	if !ctx.Ongoing("high:"+attr) || !kubeletBlind(ctx, e) {
		return detection.Finding{}, false
	}
	staleSince := ctx.Onset("held:"+attr, ctx.Now)
	if ctx.Now.Sub(staleSince) > usageHoldMax {
		return detection.Finding{}, false
	}
	since := ctx.Onset("high:"+attr, ctx.Now)
	severity := detection.Warning
	if ctx.Ongoing("critical:" + attr) {
		ctx.Onset("critical:"+attr, ctx.Now)
		severity = detection.Critical
	}
	return detection.Finding{
		Reason: reason, Severity: severity, Since: since,
		Summary: upper(what) + " was over " + strconv.Itoa(
			int(usageWarning)) + "% used and the kubelet cannot be " +
			"read, so it is assumed to be still full",
		Evidence: []detection.Evidence{{
			Label: "reading stale since",
			Value: staleSince.UTC().Format("2006-01-02 15:04 MST"),
		}},
	}, true
}

// kubeletBlind reports whether the kubelet that would report the usage
// of e failed its last reads: the node itself, or the nodes of the pods
// that mount a claim.
func kubeletBlind(ctx detection.Context, e inventory.Entity) bool {
	nodes := []inventory.EntityID{e.ID}
	if e.ID.Kind == kube.KindPVC {
		nodes = nil
		for _, pod := range ctx.Model.Related(e.ID, inventory.Mounts,
			inventory.Incoming) {
			nodes = append(nodes, ctx.Model.Related(pod, inventory.RunsOn,
				inventory.Outgoing)...)
		}
	}
	for _, id := range nodes {
		node, ok := ctx.Model.Entity(id)
		if failures, _ := number(node, kube.AttrKubeletFailures); ok &&
			failures > 0 {
			return true
		}
	}
	return false
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
