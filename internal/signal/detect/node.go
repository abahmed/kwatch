package detect

import (
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// DefaultNodeNotReady is how long a node may be NotReady before it is a
// signal; brief kubelet restarts recover faster than this.
const DefaultNodeNotReady = 90 * time.Second

// nodePressure maps pressure conditions to their signal reasons.
var nodePressure = []struct {
	condition string
	reason    string
	summary   string
}{
	{"MemoryPressure", constant.ReasonMemoryPressure,
		"Node is low on memory; pods may be evicted"},
	{"DiskPressure", constant.ReasonDiskPressure,
		"Node is low on disk space; pods may be evicted"},
	{"PIDPressure", constant.ReasonPIDPressure,
		"Node is running out of process IDs"},
}

// Node detects NotReady or unreachable nodes, resource pressure and
// network unavailability.
type Node struct {
	notReady time.Duration
}

// NewNode builds the node detector.
func NewNode(notReady time.Duration) Node {
	if notReady <= 0 {
		notReady = DefaultNodeNotReady
	}
	return Node{notReady: notReady}
}

// Name implements signal.Detector.
func (Node) Name() string { return "node" }

// Kinds implements signal.Detector.
func (Node) Kinds() []knowledge.Kind { return []knowledge.Kind{kube.KindNode} }

// Detect implements signal.Detector.
func (d Node) Detect(ctx signal.Context, e knowledge.Entity) []signal.Signal {
	var out []signal.Signal
	if s, ok := d.readiness(ctx, e); ok {
		out = append(out, s)
	}
	for _, p := range nodePressure {
		status, _, since := condition(e, p.condition)
		if status == "True" {
			out = append(out, signal.Signal{
				Reason: p.reason, Severity: signal.Critical,
				Since: since, Summary: p.summary,
			})
		}
	}
	if status, reason, since := condition(e, "NetworkUnavailable"); status ==
		"True" {
		out = append(out, signal.Signal{
			Reason:   constant.ReasonNetworkUnavailable,
			Severity: signal.Critical, Since: since,
			Summary: "Node network is not configured (" + reason + ")",
		})
	}
	return out
}

func (d Node) readiness(
	ctx signal.Context, e knowledge.Entity,
) (signal.Signal, bool) {
	status, reason, since := condition(e, "Ready")
	if status == "" || status == "True" {
		return signal.Signal{}, false
	}
	if !sustained(ctx, since, d.notReady) {
		return signal.Signal{}, false
	}
	summary := "Node is NotReady for " + format.Duration(ctx.Now.Sub(since))
	if status == "Unknown" {
		summary = "Node stopped reporting (kubelet unreachable) for " +
			format.Duration(ctx.Now.Sub(since))
	}
	return signal.Signal{
		Reason: constant.ReasonNodeNotReady, Severity: signal.Critical,
		Since: since, Summary: summary,
		Evidence: []signal.Evidence{
			{Label: "reason", Value: reason},
			{Label: "message", Value: conditionMessage(e, "Ready")},
		},
	}, true
}
