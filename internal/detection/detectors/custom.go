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
	failing := failingConditions(e)
	since := failingSince(failing)
	labels := failingLabels(failing)
	listeners := listenerEvidence(e)
	if len(labels) == 0 && len(listeners) > 0 {
		labels = []string{"failing listeners"}
		since = valueSince(e, kube.AttrListenerProblems)
	}
	if len(labels) == 0 ||
		!sustained(ctx, "custom-failing", since, failingGrace(failing)) {
		return out
	}
	sort.Strings(labels)
	return append(out, detection.Finding{
		Reason: customReason(e.ID.Kind), Severity: detection.Warning,
		Since:    since,
		Summary:  "Reports " + strings.Join(labels, ", "),
		Evidence: append(messageEvidence(e, failing), listeners...),
	})
}

// failingCondition is a condition of a custom resource in its failing
// state.
type failingCondition struct {
	label   string
	message string
	since   time.Time
	// unknown marks a controller that cannot tell yet, not one that
	// reports a failure.
	unknown bool
}

// failingConditions lists the conditions that are in their bad status
// by the shared condition table, and a resource that says it is not
// ready with a message.
func failingConditions(e inventory.Entity) []failingCondition {
	var failing []failingCondition
	for _, rule := range kube.ConditionRules {
		status, reason, at := condition(e, rule.Type)
		if status != rule.Bad {
			continue
		}
		label := rule.Type + "=" + status
		if reason != "" {
			label += " (" + reason + ")"
		}
		failing = append(failing, failingCondition{
			label: label, since: at, unknown: rule.Bad == "Unknown",
			message: text(e, kube.ConditionKey(rule.Type)+
				kube.AttrConditionMessage),
		})
	}
	if notReadyWithMessage(e) {
		failing = append(failing, failingCondition{
			label: "not ready", since: valueSince(e, kube.AttrReady),
			message: text(e, kube.AttrMessage),
		})
	}
	return failing
}

// failingGrace is how long the failing conditions must last. A resource
// whose only trouble is a controller that cannot tell yet (Ready=Unknown)
// gets the reconcile window, like a controller that has not observed its
// spec; any other failure uses the shorter one.
func failingGrace(failing []failingCondition) time.Duration {
	for _, c := range failing {
		if !c.unknown {
			return DefaultCustomFailing
		}
	}
	if len(failing) == 0 {
		return DefaultCustomFailing
	}
	return DefaultGenerationGrace
}

// failingSince is when the first of the conditions began, or zero.
func failingSince(failing []failingCondition) time.Time {
	var since time.Time
	for _, c := range failing {
		if since.IsZero() || c.since.Before(since) {
			since = c.since
		}
	}
	return since
}

func failingLabels(failing []failingCondition) []string {
	labels := make([]string, 0, len(failing))
	for _, c := range failing {
		labels = append(labels, c.label)
	}
	return labels
}

// messageEvidence quotes what the controller says is wrong: the message
// of the earliest failing condition that has one, else the resource's
// own status error. It is quoted even when empty so the evidence keeps
// its shape.
func messageEvidence(
	e inventory.Entity, failing []failingCondition,
) []detection.Evidence {
	message := text(e, kube.AttrMessage)
	var since time.Time
	for _, c := range failing {
		if c.message != "" && (since.IsZero() || c.since.Before(since)) {
			message, since = c.message, c.since
		}
	}
	return []detection.Evidence{{Label: "message", Value: message}}
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
