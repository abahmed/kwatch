package statuswatch

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
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

func failureSignal(
	u *unstructured.Unstructured,
	resource, owner string,
	rules map[string]map[string]bool,
) *event.Signal {
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
		return &event.Signal{
			Resource:  resource,
			Namespace: u.GetNamespace(),
			PodName:   u.GetName(),
			Owner:     owner,
			Reason:    reasonFor(resource),
			Labels:    u.GetLabels(),
			Hint:      hint,
		}
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

func resourceOwnerParts(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}

func resourceOwner(u *unstructured.Unstructured) string {
	return resourceOwnerParts(u.GetNamespace(), u.GetName())
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
