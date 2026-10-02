package compose

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// goldenCase is one message whose note is kept in testdata.
type goldenCase struct {
	name  string
	write func() notification.Message
}

var goldenStart = time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)

func at(minutes, seconds int) time.Time {
	return goldenStart.Add(time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second)
}

func members(
	findings ...detection.Finding,
) map[detection.Key]detection.Finding {
	out := make(map[detection.Key]detection.Finding, len(findings))
	for _, s := range findings {
		out[s.Key()] = s
	}
	return out
}

func goldenCases() []goldenCase {
	cases := append(clusterCases(), nodeResolveCases()...)
	cases = append(cases, callCases()...)
	return append(cases, []goldenCase{
		{"bad_rollout", func() notification.Message {
			return Writer{}.Write(announce(badRollout()), at(5, 0))
		}},
		{"node_memory_hog", writeNodeMemoryHog},
		{"unknown_cause_unverified_secrets", writeUnknownUnverified},
		{"unknown_cause_unclear", writeUnknownUnclear},
		{"update_spreading", writeSpreading},
		{"cause_revised", writeCauseRevised},
		{"resolved_by_rollback", writeResolvedByRollback},
		{"resolved_without_fix", writeResolvedWithoutFix},
		{"flapping", writeFlapping},
		{"flapping_update", writeFlappingUpdate},
		{"likely_config_change", writeLikelyConfigChange},
		{"startup_summary", writeStartupSummary},
	}...)
}

func announce(p incident.Incident) incident.Decision {
	return incident.Decision{Action: incident.Announce, Incident: p,
		Reason: "settled"}
}

var (
	payments   = inventory.CoreID(kube.KindDeployment, "shop", "payments")
	paymentsCt = inventory.CoreID(kube.KindContainer, "shop",
		"payments-7d9f/app")
)

func badRollout() incident.Incident {
	crash := detection.Finding{
		Entity: paymentsCt, Reason: reasons.CrashLoopBackOff,
		Severity: detection.Critical, Since: at(2, 20),
		Summary: "Container is crash looping",
		Evidence: []detection.Evidence{{Label: "error",
			Value: "panic: STRIPE_KEY is not set"}},
	}
	return incident.Incident{
		ID: "inc-1", Root: payments, Tier: incident.Page,
		State: incident.Open, Opened: at(2, 20), Announced: at(5, 0),
		Revision: 1, Members: members(crash),
		Impact: []inventory.EntityID{payments,
			inventory.CoreID(kube.KindService, "shop", "payments"),
			inventory.CoreID(kube.KindIngress, "shop", "storefront"),
			inventory.CoreID(kube.KindDeployment, "shop", "checkout"),
		},
		Cause: &rootcause.CauseRecord{
			Root: payments, Score: 0.95, RollbackRevision: "13",
			Chain: []inventory.EntityID{payments, paymentsCt},
			Change: &inventory.Change{Entity: payments, At: at(2, 0),
				Actor: "alice", Revision: "14",
				Fields: []inventory.FieldChange{{
					Path:   "spec.template.spec.containers[0].image",
					Before: "payments:2.2", After: "payments:2.3"}}},
			Summary: "the rollout that changed image payments:2.2 → " +
				"payments:2.3",
			Proof: []rootcause.Proof{
				{Text: "only pods of the new revision fail", Weight: 0.3,
					Supports: true},
				{Text: "the failure started right after the rollout",
					Weight: 0.25, Supports: true},
			},
		},
	}
}

func writeNodeMemoryHog() notification.Message {
	node := inventory.CoreID(kube.KindNode, "", "n1")
	etl := inventory.CoreID(kube.KindContainer, "batch", "etl-7/etl")
	p := incident.Incident{
		ID: "inc-2", Root: node, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		Members: members(
			detection.Finding{Entity: node, Reason: reasons.MemoryPressure,
				Severity: detection.Critical, Since: at(0, 0),
				Summary: "Node is low on memory; pods may be evicted",
				Evidence: []detection.Evidence{{Label: "used",
					Value: "96%"}}},
			detection.Finding{Entity: etl, Reason: "MemoryHog",
				Severity: detection.Warning, Since: at(0, 0),
				Summary: "Container uses far more memory than it requests",
				Evidence: []detection.Evidence{{Label: "used",
					Value: "11534Mi"}}},
		),
		Impact: []inventory.EntityID{node,
			inventory.CoreID(kube.KindDeployment, "shop", "api"),
			inventory.CoreID(kube.KindDeployment, "shop", "web")},
	}
	return Writer{}.Write(announce(p), at(6, 0))
}

func writeUnknownUnverified() notification.Message {
	invoices := inventory.CoreID(kube.KindDeployment, "billing", "invoices")
	ct := inventory.CoreID(kube.KindContainer, "billing",
		"invoices-5c8/app")
	p := incident.Incident{
		ID: "inc-3", Root: invoices, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		Unverified: []string{"secrets in billing"},
		Members: members(detection.Finding{Entity: ct,
			Reason: reasons.CrashLoopBackOff, Severity: detection.Critical,
			Since: at(0, 0), Summary: "Container is crash looping",
			Evidence: []detection.Evidence{{Label: "error",
				Value: "invalid credentials for database invoices"}}}),
	}
	d := announce(p)
	d.Output = []string{"starting invoices", "auth failed"}
	return Writer{}.Write(d, at(4, 0))
}

// writeUnknownUnclear is a rollout stuck on its own condition, with a
// quota that was weighed as its cause and dropped: nothing is blamed,
// and the note says the cause is not clear.
func writeUnknownUnclear() notification.Message {
	reports := inventory.CoreID(kube.KindDeployment, "shop", "reports")
	ct := inventory.CoreID(kube.KindContainer, "shop", "reports-8b1/app")
	p := incident.Incident{
		ID: "inc-5", Root: reports, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		CauseUnclear: true,
		Members: members(
			detection.Finding{Entity: reports,
				Reason:   reasons.ProgressDeadlineExceeded,
				Severity: detection.Critical, Since: at(0, 0),
				Summary: "Rollout is stuck: new pods did not become " +
					"available within the progress deadline"},
			detection.Finding{Entity: ct, Reason: reasons.CrashLoopBackOff,
				Severity: detection.Critical, Since: at(0, 30),
				Summary: "Container is crash looping",
				Evidence: []detection.Evidence{{Label: "error",
					Value: "dial tcp 10.0.0.9:5432: connection refused"}}},
		),
	}
	return Writer{}.Write(announce(p), at(6, 0))
}

func writeSpreading() notification.Message {
	p := badRollout()
	cart := inventory.CoreID(kube.KindService, "shop", "cart")
	backends := detection.Finding{Entity: cart, Reason: "NoEndpoints",
		Severity: detection.Critical, Since: at(7, 0), Symptom: true,
		Summary: "Service has no ready backends; traffic to it fails"}
	p.Members[backends.Key()] = backends
	p.Revision = 2
	p.Timeline = []incident.Event{
		{At: at(2, 20), Text: "Container is crash looping " +
			"(container shop/payments-7d9f/app)"},
		{At: at(7, 0), Text: backends.Summary + " (service shop/cart)"},
	}
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: "material change"}
	return Writer{}.Write(d, at(7, 0))
}

func writeCauseRevised() notification.Message {
	node := inventory.CoreID(kube.KindNode, "", "n2")
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	pod := inventory.CoreID(kube.KindPod, "shop", "api-6f5-x2")
	pressure := detection.Finding{Entity: node,
		Reason: reasons.MemoryPressure, Severity: detection.Critical,
		Since: at(1, 0), Summary: "Node is low on memory; pods may be evicted"}
	p := incident.Incident{
		ID: "inc-4", Root: node, Tier: incident.Page, State: incident.Open,
		Opened: at(0, 0), Revision: 2,
		Members: members(detection.Finding{Entity: pod,
			Reason: "Evicted", Severity: detection.Critical, Since: at(2, 0),
			Summary: "Pod was evicted"}),
		Cause: &rootcause.CauseRecord{Root: node, Score: 0.8,
			RootFindings: []detection.Finding{pressure},
			Chain:        []inventory.EntityID{node, api, pod},
			Summary:      "node n2: Node is low on memory",
			Proof: []rootcause.Proof{
				{Text: "3 of 4 pods on the node are failing", Weight: 0.2,
					Supports: true},
				{Text: "the node problem started before the pod failed",
					Weight: 0.2, Supports: true},
			}},
	}
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: incident.ReasonCauseRevised}
	return Writer{}.Write(d, at(8, 0))
}

func writeResolvedByRollback() notification.Message {
	p := badRollout()
	p.State, p.Resolved, p.Revision = incident.Resolved, at(9, 30), 3
	d := incident.Decision{Action: incident.Resolve, Incident: p,
		Reason: "healthy for 2m"}
	return Writer{}.WriteResolvedBy(d, at(11, 30), inventory.Change{
		Entity: payments, At: at(9, 0), Actor: "alice", Revision: "13"})
}

func writeResolvedWithoutFix() notification.Message {
	p := badRollout()
	p.Tier = incident.Notify
	p.State, p.Resolved, p.Revision = incident.Resolved, at(40, 0), 2
	d := incident.Decision{Action: incident.Resolve, Incident: p}
	return Writer{}.Write(d, at(42, 0))
}

func flappingSearch() incident.Incident {
	search := inventory.CoreID(kube.KindDeployment, "shop", "search")
	ct := inventory.CoreID(kube.KindContainer, "shop", "search-9b/app")
	return incident.Incident{
		ID: "inc-5", Root: search, Tier: incident.Notify,
		State: incident.Flapping, Opened: at(0, 0), Revision: 1,
		Cycles: []time.Time{at(10, 0), at(25, 0), at(40, 0)},
		Members: members(detection.Finding{Entity: ct,
			Reason: reasons.OOMKilled, Severity: detection.Critical,
			Since:   at(40, 0),
			Summary: "Container is killed for exceeding its memory limit"}),
	}
}

func writeFlapping() notification.Message {
	return Writer{}.Write(announce(flappingSearch()), at(41, 0))
}

func writeFlappingUpdate() notification.Message {
	d := incident.Decision{Action: incident.Update,
		Incident: flappingSearch(), Reason: "flapping"}
	return Writer{}.Write(d, at(41, 0))
}

func writeLikelyConfigChange() notification.Message {
	orders := inventory.CoreID(kube.KindDeployment, "shop", "orders")
	cfg := inventory.CoreID(kube.KindConfigMap, "shop", "orders-config")
	ct := inventory.CoreID(kube.KindContainer, "shop", "orders-4d/app")
	p := incident.Incident{
		ID: "inc-6", Root: cfg, Tier: incident.Notify, State: incident.Open,
		Opened: at(3, 0), Revision: 1,
		Members: members(detection.Finding{Entity: ct,
			Reason: reasons.CrashLoopBackOff, Severity: detection.Critical,
			Since: at(3, 0), Summary: "Container is crash looping"}),
		Cause: &rootcause.CauseRecord{Root: cfg, Score: 0.6,
			Chain: []inventory.EntityID{cfg, orders, ct},
			Change: &inventory.Change{Entity: cfg, At: at(1, 30),
				Actor: "bob", Fields: []inventory.FieldChange{{
					Path: "data.DB_HOST", Before: "db-1", After: "db-2"}}},
			Summary: "configmap orders-config changed",
			Proof: []rootcause.Proof{{Text: "it changed 1m30s before " +
				"the failure", Weight: 0.15, Supports: true}}},
	}
	return Writer{}.Write(announce(p), at(6, 0))
}

func writeStartupSummary() notification.Message {
	hog := writeNodeMemoryHogIncident()
	return Writer{}.StartupSummary([]incident.Decision{
		announce(hog), announce(badRollout()),
	}, at(10, 0))
}

func writeNodeMemoryHogIncident() incident.Incident {
	node := inventory.CoreID(kube.KindNode, "", "n1")
	return incident.Incident{
		ID: "inc-7", Root: node, Tier: incident.Notify, State: incident.Open,
		Opened: at(0, 0), Revision: 1,
		Members: members(detection.Finding{Entity: node,
			Reason: reasons.MemoryPressure, Severity: detection.Critical,
			Since:   at(0, 0),
			Summary: "Node is low on memory; pods may be evicted"}),
	}
}
