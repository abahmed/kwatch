package detectors

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// daemonSetMisscheduled reports a DaemonSet whose status says some nodes
// run its pod although it is not meant to run there, once that has held
// for the workload grace. The DaemonSet controller removes such pods at
// once, so a count that stays is one it could not remove. It is a digest
// line: nothing fails because of it.
func daemonSetMisscheduled(
	ctx detection.Context, e inventory.Entity, grace time.Duration,
) (detection.Finding, bool) {
	count, ok := number(e, kube.AttrMisscheduled)
	if e.ID.Kind != kube.KindDaemonSet || !ok || count <= 0 {
		return detection.Finding{}, false
	}
	since := ctx.Onset("misscheduled", valueSince(e, kube.AttrMisscheduled))
	if !sustained(ctx, "misscheduled", since, grace) {
		return detection.Finding{}, false
	}
	nodes := strconv.Itoa(int(count))
	return detection.Finding{
		Reason: reasons.DaemonSetMisscheduled, Severity: detection.Info,
		Since:   since,
		Summary: misscheduledSummary(count, nodes),
		Evidence: []detection.Evidence{
			{Label: "misscheduled nodes", Value: nodes}},
	}, true
}

// misscheduledSummary names the nodes in the grammar of their count.
func misscheduledSummary(count float64, nodes string) string {
	subject := nodes + " nodes run"
	if count == 1 {
		subject = nodes + " node runs"
	}
	return subject + " its pod although it is not meant to run there"
}
