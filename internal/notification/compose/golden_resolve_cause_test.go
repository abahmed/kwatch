package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// resolveCauseCases are resolves of an incident whose stated cause was
// looked at again when it resolved.
func resolveCauseCases() []goldenCase {
	return []goldenCase{
		{"resolved_cause_still_present", func() notification.Message {
			return resolveWith(incident.CauseStill)
		}},
		{"resolved_cause_recovered", func() notification.Message {
			return resolveWith(incident.CauseRecovered)
		}},
	}
}

func resolveWith(state incident.CauseState) notification.Message {
	root := inventory.CoreID(explain.KindExternalEndpoint, "",
		"db.example.com:5432")
	ct := inventory.CoreID(kube.KindContainer, "shop", "payments-6d4f-0/app")
	p := incident.Incident{
		ID: "inc-14", Root: payments, Tier: incident.Notify,
		State: incident.Resolved, Opened: at(0, 0), Resolved: at(38, 0),
		Revision: 2,
		Members: members(detection.Finding{Entity: ct,
			Reason: reasons.CrashLoopBackOff, Severity: detection.Critical,
			Since: at(0, 0), Summary: "Container is crash looping"}),
		Cause: &rootcause.CauseRecord{
			Rule: "external-endpoint-failing", Root: root,
			Mode:  explain.ModeEndpointFailing + ".Refused",
			Score: 0.9, Chain: []inventory.EntityID{root, payments},
		},
	}
	d := incident.Decision{Action: incident.Resolve, Incident: p,
		Facts: incident.Facts{Cause: state}}
	return Writer{}.Write(d, at(40, 0))
}
