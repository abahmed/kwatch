package statuswatch

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/constant"
)

func TestAdmissionBindingTracksMissingPolicyAndRecovers(t *testing.T) {
	sink := &statusSink{}
	monitor := &Monitor{
		incidentSink:      sink,
		admissionPolicies: make(map[string]struct{}),
		admissionBindings: make(map[string]*unstructured.Unstructured),
	}
	binding := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "binding"},
		"spec":     map[string]interface{}{"policyName": "missing"},
	}}
	monitor.processAdmissionBinding(binding)
	if len(sink.processed) != 1 || sink.processed[0].Reason !=
		constant.ReasonAdmissionBindingInvalid {
		t.Fatalf("binding failure = %+v", sink.processed)
	}
	policy := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "missing"},
	}}
	monitor.processAdmissionPolicy(policy)
	if len(sink.resolved) != 2 || sink.resolved[1].reason !=
		constant.ReasonAdmissionBindingInvalid {
		t.Fatalf("binding recovery = %+v", sink.resolved)
	}
	monitor.deleteAdmissionPolicy(policy)
	if len(sink.resolved) != 3 || len(sink.processed) != 2 {
		t.Fatalf(
			"policy deletion did not resolve policy and binding: %+v",
			sink.resolved,
		)
	}
}

func TestProcessAPIServiceAndCustomResourceResolveHealthyObjects(t *testing.T) {
	sink := &statusSink{}
	monitor := &Monitor{
		incidentSink:     sink,
		conditionRules:   defaultConditionRules(),
		namespaceAllowed: func(string) bool { return true },
	}
	apiService := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "v1.apps"},
		"status": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{"type": "Available", "status": "False"},
		}},
	}}
	monitor.processAPIService(apiService)
	if len(sink.processed) != 1 || sink.processed[0].Reason !=
		constant.ReasonAPIServiceFailure {
		t.Fatalf("APIService failure = %+v", sink.processed)
	}
	healthy := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "db", "namespace": "apps"},
	}}
	monitor.processCR(healthy)
	if len(sink.resolved) != 1 || sink.resolved[0].reason !=
		constant.ReasonCustomResourceFailure {
		t.Fatalf("custom resource recovery = %+v", sink.resolved)
	}
}

func TestAdmissionPolicyIgnoresUnexpectedEventTypes(t *testing.T) {
	sink := &statusSink{}
	monitor := &Monitor{
		incidentSink:      sink,
		admissionPolicies: make(map[string]struct{}),
		admissionBindings: make(map[string]*unstructured.Unstructured),
	}
	monitor.processAdmissionPolicy("not an object")
	monitor.processAdmissionBinding("not an object")
	monitor.deleteAdmissionPolicy("not an object")
	monitor.deleteAdmissionBinding("not an object")
	if len(sink.processed) != 0 || len(sink.resolved) != 0 {
		t.Fatal("unexpected event type changed monitor state")
	}
}
