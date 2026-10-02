package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// prodWriter is a writer for a cluster with a configured name.
var prodWriter = Writer{Cluster: "prod-eu-1"}

// clusterCases say the configured cluster name once, in the lead; the
// cases without a cluster name in goldenCases leave it out.
func clusterCases() []goldenCase {
	return []goldenCase{
		{"cluster_bad_rollout", func() notification.Message {
			return prodWriter.Write(announce(badRollout()), at(5, 0))
		}},
		{"cluster_node_lost", func() notification.Message {
			return prodWriter.Write(announce(lostNode()), at(3, 0))
		}},
		{"cluster_resolved", func() notification.Message {
			return prodWriter.Write(resolved(badRollout()), at(42, 0))
		}},
		{"cluster_startup_summary", func() notification.Message {
			return prodWriter.StartupSummary([]incident.Decision{
				announce(badRollout())}, at(10, 0))
		}},
	}
}

// nodeResolveCases end a lost node in each way a node can end.
func nodeResolveCases() []goldenCase {
	n1 := lostNode().Root
	return []goldenCase{
		{"resolved_node_deleted", func() notification.Message {
			return Writer{}.WriteResolvedBy(resolved(lostNode()), at(12, 0),
				inventory.Change{Entity: n1, At: at(9, 0), Deleted: true})
		}},
		{"resolved_node_replaced", func() notification.Message {
			p := lostNode()
			p.Fix = incident.FixNodeReplaced
			return Writer{}.Write(resolved(p), at(12, 0))
		}},
		{"resolved_node_recovered", func() notification.Message {
			return Writer{}.Write(resolved(lostNode()), at(12, 0))
		}},
	}
}

func lostNode() incident.Incident {
	node := inventory.CoreID(kube.KindNode, "", "n1")
	return incident.Incident{
		ID: "inc-8", Root: node, Tier: incident.Page, State: incident.Open,
		Opened: at(0, 0), Revision: 1,
		Members: members(detection.Finding{Entity: node,
			Reason: reasons.NodeNotReady, Severity: detection.Critical,
			Since:   at(0, 0),
			Summary: "Node stopped reporting (kubelet unreachable) for 2m"}),
	}
}

func resolved(p incident.Incident) incident.Decision {
	p.State, p.Resolved, p.Revision = incident.Resolved, at(10, 0), 2
	return incident.Decision{Action: incident.Resolve, Incident: p,
		Reason: "healthy for 3m0s"}
}
