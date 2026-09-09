package statuswatch

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func admissionWarningText(warnings []interface{}) string {
	for _, raw := range warnings {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		warning, ok := item["warning"].(string)
		if ok && warning != "" {
			return warning
		}
	}
	return "invalid CEL expression"
}

// failureSignal reports the first condition on an object that the rules
// consider a failure.
//
// The owner is derived from the object rather than passed in: every caller
// computed the same "namespace/name", and a caller that computed it
// differently would key its incidents where nothing else could resolve them.
func failureSignal(
	u *unstructured.Unstructured,
	resource string,
	rules map[string]map[string]bool,
) *model.Observation {
	conditions, found, _ := unstructured.NestedSlice(
		u.Object,
		"status",
		"conditions",
	)
	if !found {
		return nil
	}
	for _, raw := range conditions {
		condition, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := condition["type"].(string)
		status, _ := condition["status"].(string)
		reason, _ := condition["reason"].(string)
		message, _ := condition["message"].(string)
		if statuses := rules[typ]; statuses == nil || !statuses[status] {
			continue
		}
		if reason == "" {
			reason = "condition reported " + status
		}
		hint := typ + "=" + status + ": " + reason
		if message != "" {
			hint += " — " + message
		}
		return observeObject(u, resource).WithHint(hint)
	}
	return nil
}

func reasonFor(resource string) string {
	if resource == "apiservice" {
		return constant.ReasonAPIServiceFailure
	}
	switch resource {
	case "mutatingadmissionpolicy", "mutatingadmissionpolicybinding":
		return constant.ReasonMutatingAdmissionPolicyInvalid
	case "certificatesigningrequest":
		return constant.ReasonCertificateSigningRequestFailure
	case "flowschema", "prioritylevelconfiguration":
		return constant.ReasonAPIPriorityAndFairnessFailure
	case "endpoints":
		return constant.ReasonServiceNoEndpoints
	case "resourceclaim":
		return constant.ReasonResourceClaimFailure
	}
	return constant.ReasonCustomResourceFailure
}

// observeObject is the observation every status watch produces: the watched
// object as its own subject and owner, with the reason its kind maps to.
func observeObject(
	u *unstructured.Unstructured, resource string,
) *model.Observation {
	return observe.ObjectNamed(
		resource, u.GetNamespace(), u.GetName(), reasonFor(resource),
	).WithLabels(u.GetLabels())
}

func nestedStringValues(value interface{}, path []string) []string {
	if len(path) == 0 {
		if name, ok := value.(string); ok && name != "" {
			return []string{name}
		}
		return nil
	}
	if items, ok := value.([]interface{}); ok {
		var values []string
		for _, item := range items {
			values = append(values, nestedStringValues(item, path)...)
		}
		return values
	}
	object, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	return nestedStringValues(object[path[0]], path[1:])
}
