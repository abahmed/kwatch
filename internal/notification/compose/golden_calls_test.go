package compose

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// callCases are storms that only error text ties together: one
// external endpoint every workload calls, or one shared error.
func callCases() []goldenCase {
	return []goldenCase{
		{"external_endpoint_storm", writeEndpointStorm},
		{"shared_signature_storm", writeSignatureStorm},
	}
}

// crashStorm is an incident of five crashing workloads in shop whose
// containers all fail with errors.
func crashStorm(root inventory.EntityID, rule string, mode detection.Mode,
	errorOf func(i int) string,
) incident.Incident {
	var findings []detection.Finding
	var impact []inventory.EntityID
	for i, name := range []string{"api", "cart", "search", "auth", "web"} {
		impact = append(impact,
			inventory.CoreID(kube.KindDeployment, "shop", name))
		findings = append(findings, detection.Finding{
			Entity: inventory.CoreID(kube.KindContainer, "shop",
				fmt.Sprintf("%s-6d4f-%d/app", name, i)),
			Reason: reasons.CrashLoopBackOff, Severity: detection.Critical,
			Since: at(1, i), Summary: "Container is crash looping",
			Evidence: []detection.Evidence{{Label: "error",
				Value: errorOf(i)}},
		})
	}
	return incident.Incident{
		ID: "inc-9", Root: root, Tier: incident.Page, State: incident.Open,
		Opened: at(1, 0), Revision: 1, Members: members(findings...),
		Impact: impact,
		Cause: &rootcause.CauseRecord{Rule: rule, Mode: mode, Root: root,
			Score: 0.95, Chain: []inventory.EntityID{root, impact[0]},
			Summary: string(root.Kind) + " " + root.Name + " (" +
				string(mode) + ") explains 5 failures",
			Proof: []rootcause.Proof{
				{Code: rootcause.ProofErrorsName, Count: 5, Total: 5,
					Text:   "the errors of 5 of 5 failures name it",
					Weight: 0.15, Supports: true},
				{Code: rootcause.ProofWorkloadsMeet, Count: 5,
					Text:   "failures of 5 workloads meet here",
					Weight: 0.15, Supports: true},
			}},
	}
}

func writeEndpointStorm() notification.Message {
	root := inventory.CoreID(explain.KindExternalEndpoint, "",
		"db.example.com:5432")
	p := crashStorm(root, "external-endpoint-failing",
		explain.ModeEndpointFailing+".Refused", func(int) string {
			return "dial tcp db.example.com:5432: connect: " +
				"connection refused"
		})
	return Writer{}.Write(announce(p), at(4, 0))
}

func writeSignatureStorm() notification.Message {
	root := inventory.CoreID(explain.KindFailureSignature, "",
		"CrashLoop panic: license check failed for tenant #")
	p := crashStorm(root, "shared-failure-signature",
		explain.ModeSharedSignature, func(i int) string {
			return fmt.Sprintf("panic: license check failed for tenant %d",
				100+i)
		})
	p.Tier = incident.Notify
	p.Cause.Score = 0.7
	return Writer{}.Write(announce(p), at(4, 0))
}
