package cluster

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/constant"
)

func TestEventFactsClassifiesFailureDomains(t *testing.T) {
	tests := []struct {
		name       string
		reason     string
		message    string
		domain     string
		code       string
		dependency string
	}{
		{
			name:    "storage missing",
			reason:  "FailedMount",
			message: `service "database" not found`,
			domain:  "storage", code: "dependency_missing",
			dependency: "service/demo/database",
		},
		{
			name: "storage binding", reason: "FailedBinding",
			message: "volume binding failed",
			domain:  "storage", code: "storage_operation_failed",
		},
		{
			name: "storage resize", reason: "VolumeResizeFailed",
			message: "resize timed out",
			domain:  "storage", code: "timeout",
		},
		{
			name:    "scheduling cpu",
			reason:  "FailedScheduling",
			message: "Insufficient cpu",
			domain:  "scheduling", code: "insufficient_cpu",
		},
		{
			name:    "admission tls",
			reason:  "FailedAdmissionWebhook",
			message: "x509 certificate has expired",
			domain:  "admission", code: "webhook_tls",
		},
		{
			name:    "network timeout",
			reason:  "NetworkNotReady",
			message: "request timeout",
			domain:  "network", code: "timeout",
		},
		{
			name: "network update", reason: "FailedUpdateEndpointSlices",
			message: "endpoint update failed",
			domain:  "network", code: "network_update_failed",
		},
		{
			name:    "discovery",
			reason:  "FailedDiscoveryCheck",
			message: "discovery failed",
			domain:  "api_discovery", code: "discovery_failed",
		},
		{
			name:    "workload quota",
			reason:  "FailedCreate",
			message: "exceeded quota",
			domain:  "workload", code: "quota_exceeded",
		},
		{
			name: "workload daemon", reason: "FailedDaemonPod",
			message: "pod does not exist",
			domain:  "workload", code: "dependency_missing",
		},
		{
			name:    "deadline",
			reason:  "DeadlineExceeded",
			message: "job deadline exceeded",
			domain:  "workload", code: "workload_deadline_exceeded",
		},
		{
			name:    "scaling",
			reason:  "FailedScale",
			message: "scale failed",
			domain:  "scaling", code: "scale_operation_failed",
		},
		{
			name: "rescaling", reason: "FailedRescale",
			message: "rescale failed",
			domain:  "scaling", code: "scale_operation_failed",
		},
		{
			name:    "validation",
			reason:  "FailedValidation",
			message: "invalid object",
			domain:  "configuration", code: "validation_failed",
		},
		{
			name:    "node",
			reason:  "NodeNotReady",
			message: "node stopped responding",
			domain:  "node", code: "node_not_ready",
		},
		{
			name: "kubelet", reason: "KubeletNotReady",
			message: "kubelet stopped responding",
			domain:  "node", code: "node_not_ready",
		},
		{
			name:    "readiness",
			reason:  "Unhealthy",
			message: "readiness probe failed",
			domain:  "health_check", code: "readiness_probe_failed",
		},
		{
			name: "startup", reason: "Unhealthy",
			message: "startup probe failed",
			domain:  "health_check", code: "startup_probe_failed",
		},
		{
			name: "hpa api", reason: constant.ReasonFailedGetMetrics,
			message: "metrics API is unavailable for resource cpu",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts := eventFacts(&corev1.Event{
				Reason: test.reason, Message: test.message,
				ObjectMeta: metav1.ObjectMeta{Namespace: "demo"},
				InvolvedObject: corev1.ObjectReference{
					Namespace: "demo",
				},
			})
			if facts.FailureDomain != test.domain {
				t.Fatalf("domain = %q, want %q", facts.FailureDomain,
					test.domain)
			}
			if facts.FailureCode != test.code {
				t.Fatalf("code = %q, want %q", facts.FailureCode, test.code)
			}
			if test.dependency != "" && facts.Dependency.Key() != test.dependency {
				t.Fatalf("dependency = %q, want %q", facts.Dependency.Key(),
					test.dependency)
			}
		})
	}
}

func TestEventFactsClassifiersCoverFallbacks(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			"workload access", classifyWorkloadEvent("forbidden"),
			"access_denied",
		},
		{
			"workload missing", classifyWorkloadEvent("does not exist"),
			"dependency_missing",
		},
		{
			"health liveness", classifyHealthEvent("liveness probe failed"),
			"liveness_probe_failed",
		},
		{
			"health startup", classifyHealthEvent("startup probe failed"),
			"startup_probe_failed",
		},
		{
			"health fallback", classifyHealthEvent("container failed"),
			"health_check_failed",
		},
		{
			"storage attach", classifyStorageEvent("already attached"),
			"multi_attach",
		},
		{
			"storage permission", classifyStorageEvent("unauthorized"),
			"access_denied",
		},
		{"storage timeout", classifyStorageEvent("timed out"), "timeout"},
		{
			"storage fallback", classifyStorageEvent("broken"),
			"storage_operation_failed",
		},
		{
			"scheduling memory", classifySchedulingEvent("insufficient memory"),
			"insufficient_memory",
		},
		{
			"scheduling taint", classifySchedulingEvent("untolerated taint"),
			"untolerated_taint",
		},
		{
			"scheduling affinity", classifySchedulingEvent("affinity mismatch"),
			"placement_constraints",
		},
		{
			"scheduling volume",
			classifySchedulingEvent("persistentvolumeclaim pending"),
			"volume_unbound",
		},
		{
			"scheduling fallback", classifySchedulingEvent("no match"),
			"no_eligible_node",
		},
		{
			"admission endpoint",
			classifyAdmissionEvent("no endpoints available"),
			"webhook_no_endpoints",
		},
		{
			"admission timeout", classifyAdmissionEvent("deadline exceeded"),
			"webhook_timeout",
		},
		{
			"admission fallback", classifyAdmissionEvent("connection reset"),
			"webhook_call_failed",
		},
		{
			"network unavailable",
			classifyNetworkEvent("service unavailable"),
			"network_unavailable",
		},
		{
			"network fallback", classifyNetworkEvent("update failed"),
			"network_update_failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("got %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestHPAEventFactsClassifiesMetricFailures(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want string
	}{
		{
			"missing request",
			"missing request for cpu in container api of pod web",
			"missing_request",
		},
		{
			"api unavailable", "metrics API is unavailable for resource cpu",
			"api_unavailable",
		},
		{
			"pod metrics", "no metrics returned for ready pods",
			"missing_pod_metrics",
		},
		{"invalid metric", "failed to parse invalid metric", "invalid_metric"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts := hpaEventFacts(constant.ReasonFailedGetMetrics, test.msg)
			if facts.MetricFailure != test.want {
				t.Fatalf("failure = %q, want %q", facts.MetricFailure,
					test.want)
			}
		})
	}
	if got := hpaEventFacts("Other", "metrics API is unavailable"); !got.IsZero() {
		t.Fatalf("unexpected facts for unrelated reason: %#v", got)
	}
}

func TestAutoscalerEventFactsClassifiesMessages(t *testing.T) {
	tests := []struct {
		message string
		want    string
	}{
		{"maximum node group size reached", "node_group_at_max"},
		{"insufficient capacity", "no_scale_up_option"},
		{"scale up backoff", "scale_up_backoff"},
		{"provider failed", "scale_up_failed"},
	}
	for _, test := range tests {
		facts := autoscalerEventFacts(test.message)
		if facts.FailureCode != test.want {
			t.Errorf("message %q: code = %q, want %q", test.message,
				facts.FailureCode, test.want)
		}
	}
}

func TestEventFactsHandlesNilAndObjectReferences(t *testing.T) {
	if facts := eventFacts(nil); !facts.IsZero() {
		t.Fatalf("nil event facts = %#v", facts)
	}
	for _, test := range []struct {
		message string
		kind    string
	}{
		{`secret 'credentials' changed`, "secret"},
		{`configmap "settings" changed`, "configmap"},
		{`persistentvolumeclaim "data" pending`, "pvc"},
	} {
		ref := referencedObject("demo", test.message)
		if ref.Kind != test.kind || ref.Name == "" || ref.Namespace != "demo" {
			t.Errorf("reference for %q = %#v", test.message, ref)
		}
	}
}
