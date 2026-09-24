package cluster

import (
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/model"
)

var quotedObjectPattern = regexp.MustCompile(
	`(?i)(service|secret|configmap|persistentvolumeclaim|pvc)\s+` +
		`[\"']?([a-z0-9]([-a-z0-9.]*[a-z0-9])?)[\"']?`,
)

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func eventFacts(event *corev1.Event) model.Facts {
	if event == nil {
		return model.Facts{}
	}
	if facts := hpaEventFacts(event.Reason, event.Message); !facts.IsZero() {
		return facts
	}
	message := strings.ToLower(event.Message)
	facts := model.Facts{}
	switch event.Reason {
	case "FailedMount", "FailedAttachVolume", "FailedBinding",
		"FailedProvision", "ProvisioningFailed", "VolumeResizeFailed":
		facts.FailureDomain = "storage"
		facts.FailureCode = classifyStorageEvent(message)
	case "FailedScheduling":
		facts.FailureDomain = "scheduling"
		facts.FailureCode = classifySchedulingEvent(message)
	case "FailedCallingWebhook", "FailedAdmissionWebhook":
		facts.FailureDomain = "admission"
		facts.FailureCode = classifyAdmissionEvent(message)
	case "FailedUpdateEndpointSlices", "NetworkNotReady":
		facts.FailureDomain = "network"
		facts.FailureCode = classifyNetworkEvent(message)
	case "FailedDiscoveryCheck":
		facts.FailureDomain = "api_discovery"
		facts.FailureCode = "discovery_failed"
	case "FailedCreate", "FailedDaemonPod":
		facts.FailureDomain = "workload"
		facts.FailureCode = classifyWorkloadEvent(message)
	case "BackoffLimitExceeded", "DeadlineExceeded":
		facts.FailureDomain = "workload"
		facts.FailureCode = "workload_deadline_exceeded"
	case "FailedScale", "FailedRescale":
		facts.FailureDomain = "scaling"
		facts.FailureCode = "scale_operation_failed"
	case "FailedValidation":
		facts.FailureDomain = "configuration"
		facts.FailureCode = "validation_failed"
	case "NodeNotReady", "KubeletNotReady":
		facts.FailureDomain = "node"
		facts.FailureCode = "node_not_ready"
	case "Unhealthy":
		facts.FailureDomain = "health_check"
		facts.FailureCode = classifyHealthEvent(message)
	}
	facts.Dependency = referencedObject(event.Namespace, message)
	return facts
}

func autoscalerEventFacts(message string) model.Facts {
	facts := model.Facts{FailureDomain: "scaling"}
	switch {
	case containsAny(message, "max node group size", "maximum node group"):
		facts.FailureCode = "node_group_at_max"
	case containsAny(message, "insufficient", "no expansion options"):
		facts.FailureCode = "no_scale_up_option"
	case containsAny(message, "backoff", "back-off"):
		facts.FailureCode = "scale_up_backoff"
	default:
		facts.FailureCode = "scale_up_failed"
	}
	return facts
}

func classifyWorkloadEvent(message string) string {
	switch {
	case containsAny(message, "forbidden", "permission denied"):
		return "access_denied"
	case containsAny(message, "quota", "exceeded quota"):
		return "quota_exceeded"
	case containsAny(message, "not found", "does not exist"):
		return "dependency_missing"
	default:
		return "resource_create_failed"
	}
}

func classifyHealthEvent(message string) string {
	switch {
	case strings.Contains(message, "readiness probe"):
		return "readiness_probe_failed"
	case strings.Contains(message, "liveness probe"):
		return "liveness_probe_failed"
	case strings.Contains(message, "startup probe"):
		return "startup_probe_failed"
	default:
		return "health_check_failed"
	}
}

func classifyStorageEvent(message string) string {
	switch {
	case containsAny(message, "multi-attach", "already attached"):
		return "multi_attach"
	case containsAny(message, "not found", "does not exist"):
		return "dependency_missing"
	case containsAny(message, "permission denied", "unauthorized"):
		return "access_denied"
	case containsAny(message, "timed out", "deadline exceeded"):
		return "timeout"
	default:
		return "storage_operation_failed"
	}
}

func classifySchedulingEvent(message string) string {
	switch {
	case strings.Contains(message, "insufficient cpu"):
		return "insufficient_cpu"
	case strings.Contains(message, "insufficient memory"):
		return "insufficient_memory"
	case strings.Contains(message, "untolerated taint"):
		return "untolerated_taint"
	case containsAny(message, "affinity", "selector"):
		return "placement_constraints"
	case containsAny(message, "unbound immediate persistentvolumeclaims",
		"persistentvolumeclaim"):
		return "volume_unbound"
	default:
		return "no_eligible_node"
	}
}

func classifyAdmissionEvent(message string) string {
	switch {
	case strings.Contains(message, "no endpoints available"):
		return "webhook_no_endpoints"
	case containsAny(message, "certificate", "x509", "tls"):
		return "webhook_tls"
	case containsAny(message, "timeout", "deadline exceeded"):
		return "webhook_timeout"
	default:
		return "webhook_call_failed"
	}
}

func classifyNetworkEvent(message string) string {
	switch {
	case containsAny(message, "not ready", "unavailable"):
		return "network_unavailable"
	case containsAny(message, "timeout", "deadline exceeded"):
		return "timeout"
	default:
		return "network_update_failed"
	}
}

func referencedObject(namespace, message string) model.ObjectRef {
	match := quotedObjectPattern.FindStringSubmatch(message)
	if len(match) < 3 {
		return model.ObjectRef{}
	}
	kind := strings.ToLower(match[1])
	if kind == "persistentvolumeclaim" {
		kind = "pvc"
	}
	return model.ObjectRef{Kind: kind, Namespace: namespace, Name: match[2]}
}
