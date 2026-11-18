package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// helmCases are messages about failed or stuck Helm releases.
func helmCases() []goldenCase {
	return []goldenCase{
		{"helm_release_failed", writeHelmFailed},
	}
}

// writeHelmFailed is a release whose upgrade to revision 12 failed,
// with revision 11 deployed before it.
func writeHelmFailed() notification.Message {
	release := kube.HelmReleaseID("shop", "payments")
	p := incident.Incident{
		ID: "inc-helm", Root: release, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		Members: members(detection.Finding{Entity: release,
			Reason: reasons.HelmReleaseFailed, Severity: detection.Warning,
			Since: at(0, 0),
			Summary: "Helm release has failed to upgrade or roll " +
				"back at revision 12",
			Evidence: []detection.Evidence{
				{Label: detection.EvidenceHelmRelease,
					Value: "shop/payments"},
				{Label: detection.EvidenceHelmRevision, Value: "12"},
				{Label: detection.EvidenceHelmStatus, Value: "failed"},
				{Label: detection.EvidenceHelmGoodRevision, Value: "11"},
			}}),
	}
	return Writer{}.Write(announce(p), at(20, 0))
}
