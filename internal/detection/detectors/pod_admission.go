package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// admissionFinding reports a pod the kubelet refused to run. Its Mode
// names the rejection, such as "Admission.Rejected.OutOfcpu".
func admissionFinding(e inventory.Entity) (detection.Finding, bool) {
	reason := text(e, kube.AttrReason)
	if !detection.AdmissionReason(reason) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.PodAdmissionRejected, Severity: detection.Critical,
		Mode: detection.ModeFor(reasons.PodAdmissionRejected) +
			detection.Mode("."+reason),
		Since:   valueSince(e, kube.AttrPhase),
		Summary: "Kubelet rejected the pod at admission (" + reason + ")",
		Evidence: []detection.Evidence{{
			Label: "message", Value: text(e, kube.AttrMessage),
		}},
	}, true
}
