package detectors

import (
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultCustomFailing is how long a custom resource may report a failed
// condition; operators often pass through failed states while
// reconciling.
const DefaultCustomFailing = 2 * time.Minute

// failedWhenFalse and failedWhenTrue are condition types with a common
// meaning across operators and the Gateway API.
var (
	failedWhenFalse = []string{
		"Ready", "Available", "Accepted", "Programmed", "ResolvedRefs",
		"Synced", "Healthy",
	}
	failedWhenTrue = []string{
		"Conflicted", "Degraded", "Failed", "Stalled", "Dangling",
		"OverlappingTLSConfig",
	}
)

// Custom detects custom resources and APIServices whose status conditions
// report a failure.
type Custom struct{}

// Name implements detection.Detector.
func (Custom) Name() string { return "custom-resource" }

// Kinds implements detection.Detector.
func (Custom) Kinds() []inventory.Kind {
	return []inventory.Kind{detection.AnyKind}
}

// Detect implements detection.Detector.
func (Custom) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if !flag(e, kube.AttrCustom) {
		return nil
	}
	out := kindFindings(ctx, e)
	failing, since := failingConditions(e)
	listeners := listenerEvidence(e)
	if len(failing) == 0 && len(listeners) > 0 {
		failing = []string{"failing listeners"}
		since = valueSince(e, kube.AttrListenerProblems)
	}
	if len(failing) == 0 ||
		!sustained(ctx, "custom-failing", since, DefaultCustomFailing) {
		return out
	}
	sort.Strings(failing)
	return append(out, detection.Finding{
		Reason: customReason(e.ID.Kind), Severity: detection.Warning,
		Since:   since,
		Summary: "Reports " + strings.Join(failing, ", "),
		Evidence: append([]detection.Evidence{{
			Label: "message", Value: text(e, kube.AttrMessage),
		}}, listeners...),
	})
}

// failingConditions lists the failing condition labels and the earliest
// time any of them started.
func failingConditions(e inventory.Entity) ([]string, time.Time) {
	var failing []string
	var since time.Time
	check := func(conditionType, badStatus string) {
		status, reason, at := condition(e, conditionType)
		if status != badStatus {
			return
		}
		label := conditionType + "=" + status
		if reason != "" {
			label += " (" + reason + ")"
		}
		failing = append(failing, label)
		if since.IsZero() || at.Before(since) {
			since = at
		}
	}
	for _, c := range failedWhenFalse {
		check(c, "False")
	}
	for _, c := range failedWhenTrue {
		check(c, "True")
	}
	if notReadyWithMessage(e) {
		failing = append(failing, "not ready")
		if at := valueSince(e, kube.AttrReady); since.IsZero() ||
			at.Before(since) {
			since = at
		}
	}
	return failing, since
}

func notReadyWithMessage(e inventory.Entity) bool {
	ready, known := e.Attribute(kube.AttrReady)
	if !known {
		return false
	}
	value, _ := ready.Value.AsBool()
	return !value && text(e, kube.AttrMessage) != ""
}

func customReason(kind inventory.Kind) string {
	switch kind {
	case "apiservice":
		return reasons.APIServiceFailure
	case "volumesnapshot":
		return reasons.VolumeSnapshotFailure
	case "certificatesigningrequest":
		return reasons.CertificateSigningRequestFailure
	case "flowschema", "prioritylevelconfiguration":
		return reasons.APIPriorityAndFairnessFailure
	case "validatingadmissionpolicy":
		return reasons.AdmissionPolicyInvalid
	case "validatingadmissionpolicybinding",
		"mutatingadmissionpolicybinding":
		return reasons.AdmissionBindingInvalid
	case "mutatingadmissionpolicy":
		return reasons.MutatingAdmissionPolicyInvalid
	case "resourceclaim":
		return reasons.ResourceClaimFailure
	default:
		return reasons.CustomResourceFailure
	}
}
