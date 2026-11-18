package detectors

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Memory history shapes.
const (
	// flatShare is how close to its peak a run must have started, and
	// flatDrawdownPct how little it may have fallen, to have stayed
	// near one level: a limit that is simply too low.
	flatShare       = 0.85
	flatDrawdownPct = 15.0
	// atLimitShare is how close to the limit a peak must be to be at
	// the limit: the kernel caps use there, so the real need is higher.
	atLimitShare = 0.95
	// sameEndShare is how far apart, as a share of the highest, the
	// peaks of ended runs may be and still be "each near" one value.
	sameEndShare = 0.1
	// memoryWindowMax is the longest usage history kwatch keeps.
	memoryWindowMax = 24 * time.Hour
)

// withMemoryContext adds, to each OOM kill, the facts around it that
// kwatch already holds: which limit was reached, the shape and length of
// the memory history, the node's memory for a node kill, and the bounds
// a raised limit must fit for a kill by the container's own limit.
func withMemoryContext(
	ctx detection.Context, e inventory.Entity, found []detection.Finding,
) []detection.Finding {
	for i := range found {
		if found[i].Reason != reasons.OOMKilled {
			continue
		}
		f := &found[i]
		f.Evidence = append(f.Evidence, historyContext(ctx, e)...)
		switch {
		case !evidenceLabelMissing(*f, detection.EvidenceKilledByNode):
			f.Evidence = append(f.Evidence, nodeKillContext(ctx, e)...)
		case f.Mode == detection.ModeOOMKilledLimit:
			f.Evidence = append(f.Evidence, detection.Evidence{
				Label: detection.EvidenceOOMKind,
				Value: "its own memory limit"})
			f.Evidence = append(f.Evidence, limitBounds(ctx, e)...)
		}
	}
	return found
}

// historyContext states the request, how much history there is and its
// shape. A container never read says so instead.
func historyContext(
	ctx detection.Context, e inventory.Entity,
) []detection.Evidence {
	var out []detection.Evidence
	add := func(label, value string) {
		out = append(out, detection.Evidence{Label: label, Value: value})
	}
	if req, ok := number(e, kube.AttrMemoryReq); ok && req > 0 {
		add(detection.EvidenceMemoryRequest, quantity(req))
	}
	peak, ok := number(e, kube.AttrMemoryPeak24h)
	if !ok || peak <= 0 {
		add(detection.EvidenceMemoryHistory, "none before the kill")
		return out
	}
	if since := timestamp(e, kube.AttrMemoryHistorySince); !since.IsZero() {
		add(detection.EvidenceMemoryWindow, windowText(ctx.Now.Sub(since)))
	}
	if flat, ok := flatRun(e); ok {
		add(detection.EvidenceMemoryFlat, flat)
	}
	limit, _ := number(e, kube.AttrMemoryLimit)
	if hours, ok := number(e, kube.AttrMemoryPeakHours); ok && hours > 0 &&
		limit > 0 && peak >= atLimitShare*limit {
		add(detection.EvidenceMemoryNearLimit, strconv.Itoa(int(hours)))
	}
	if runs, ok := runsText(e); ok {
		add(detection.EvidenceMemoryRuns, runs)
	}
	return out
}

// windowText is the history's length in whole hours, at least one and
// at most a day: "5h".
func windowText(d time.Duration) string {
	d = min(max(d.Round(time.Hour), time.Hour), memoryWindowMax)
	return format.Duration(d)
}

// flatRun describes a run that stayed near one level until the kill:
// "near 500Mi for 5h".
func flatRun(e inventory.Entity) (string, bool) {
	run := prevRunOf(e)
	gap := killTime(e).Sub(run.ended)
	if run.ended.IsZero() || gap > runMatchGap || gap < -runMatchGap ||
		run.peak <= 0 || run.length < riseMinRun ||
		run.start < flatShare*run.peak || run.drawdown > flatDrawdownPct {
		return "", false
	}
	return "near " + quantity(run.peak) + " for " +
		format.Duration(run.length.Round(time.Minute)), true
}

// runsText says where the last runs ended, when more than one did.
func runsText(e inventory.Entity) (string, bool) {
	count, _ := number(e, kube.AttrMemoryRunsEnded)
	low, _ := number(e, kube.AttrMemoryRunsLow)
	high, _ := number(e, kube.AttrMemoryRunsHigh)
	if count < 2 || high <= 0 {
		return "", false
	}
	n := strconv.Itoa(int(count)) + " runs, "
	if high-low <= sameEndShare*high {
		return n + "each near " + quantity(high), true
	}
	return n + "between " + quantity(low) + " and " + quantity(high), true
}
