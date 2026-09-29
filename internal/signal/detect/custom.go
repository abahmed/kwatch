package detect

import (
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
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
	failedWhenTrue = []string{"Degraded", "Failed", "Stalled", "Dangling"}
)

// Custom detects custom resources and APIServices whose status conditions
// report a failure.
type Custom struct{}

// Name implements signal.Detector.
func (Custom) Name() string { return "custom-resource" }

// Kinds implements signal.Detector.
func (Custom) Kinds() []knowledge.Kind {
	return []knowledge.Kind{signal.AnyKind}
}

// Detect implements signal.Detector.
func (Custom) Detect(ctx signal.Context, e knowledge.Entity) []signal.Signal {
	if !flag(e, kube.AttrCustom) {
		return nil
	}
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
	if ready, known := e.Attribute(kube.AttrReady); known {
		if value, _ := ready.Value.AsBool(); !value &&
			text(e, kube.AttrMessage) != "" {
			failing = append(failing, "not ready")
		}
	}
	if len(failing) == 0 || !sustained(ctx, since, DefaultCustomFailing) {
		return nil
	}
	sort.Strings(failing)
	return []signal.Signal{{
		Reason: customReason(e.ID.Kind), Severity: signal.Warning,
		Since:   since,
		Summary: "Reports " + strings.Join(failing, ", "),
		Evidence: []signal.Evidence{{
			Label: "message", Value: text(e, kube.AttrMessage),
		}},
	}}
}

func customReason(kind knowledge.Kind) string {
	switch kind {
	case "apiservice":
		return constant.ReasonAPIServiceFailure
	case "volumesnapshot":
		return constant.ReasonVolumeSnapshotFailure
	case "certificatesigningrequest":
		return constant.ReasonCertificateSigningRequestFailure
	case "flowschema", "prioritylevelconfiguration":
		return constant.ReasonAPIPriorityAndFairnessFailure
	case "validatingadmissionpolicy":
		return constant.ReasonAdmissionPolicyInvalid
	case "validatingadmissionpolicybinding":
		return constant.ReasonAdmissionBindingInvalid
	case "mutatingadmissionpolicy":
		return constant.ReasonMutatingAdmissionPolicyInvalid
	case "resourceclaim":
		return constant.ReasonResourceClaimFailure
	default:
		return constant.ReasonCustomResourceFailure
	}
}
