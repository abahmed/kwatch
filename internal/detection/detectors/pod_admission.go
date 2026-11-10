package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// admissionFinding reports a pod the kubelet refused to run. Its Mode
// names the rejection, such as "Admission.Rejected.OutOfcpu".
// It is Critical while news and ages into history like an eviction, so a
// kwatch restart does not raise old rejected pods again.
func admissionFinding(
	ctx detection.Context, e inventory.Entity, since time.Time,
) (detection.Finding, bool) {
	reason := text(e, kube.AttrReason)
	if !detection.AdmissionReason(reason) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason:   reasons.PodAdmissionRejected,
		Severity: admissionSeverity(ctx, since),
		Mode: detection.ModeFor(reasons.PodAdmissionRejected) +
			detection.Mode("."+reason),
		Since:   since,
		Summary: "Kubelet rejected the pod at admission (" + reason + ")",
		Evidence: []detection.Evidence{{
			Label: "message", Value: text(e, kube.AttrMessage),
		}},
	}, true
}

func admissionSeverity(
	ctx detection.Context, since time.Time,
) detection.Severity {
	if since.IsZero() || !ageOut(ctx, since) {
		return detection.Critical
	}
	return detection.Info
}
