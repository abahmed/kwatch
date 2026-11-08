package compose

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// burstCases are workloads Kubernetes sees as healthy whose log says
// otherwise.
func burstCases() []goldenCase {
	return []goldenCase{
		{"log_error_burst", func() notification.Message {
			return Writer{}.Write(announce(burstIncident(
				312, 2, 3600)), at(5, 0))
		}},
		{"log_error_burst_no_baseline", func() notification.Message {
			return Writer{}.Write(announce(burstIncident(
				312, 0, 720)), at(5, 0))
		}},
	}
}

func burstIncident(count, before, span int) incident.Incident {
	orders := inventory.CoreID(kube.KindDeployment, "shop", "orders")
	finding := detection.Finding{
		Entity: orders, Reason: reasons.LogErrorBurst,
		Mode: detection.ModeErrorBurst, Severity: detection.Warning,
		Since: at(0, 0), Summary: "340 errors in 5 min",
		Evidence: []detection.Evidence{
			{Label: detection.EvidenceLogErrors, Value: "340"},
			{Label: detection.EvidenceLogWindow, Value: "300"},
			{Label: detection.EvidenceLogBaselineSpan,
				Value: strconv.Itoa(span)},
			{Label: detection.EvidenceLogLine,
				Value: "pq: connection refused"},
			{Label: detection.EvidenceLogLineCount,
				Value: strconv.Itoa(count)},
			{Label: detection.EvidenceLogLineBefore,
				Value: strconv.Itoa(before)},
		},
	}
	return incident.Incident{
		ID: "inc-12", Root: orders, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		Members: members(finding),
	}
}
