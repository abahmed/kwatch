package statuswatch

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

type statusSink struct {
	processed []*model.Observation
	resolved  []struct {
		owner  model.ObjectRef
		reason string
	}
}

func (s *statusSink) Process(
	obs *model.Observation,
) (*model.Incident, model.IncidentAction) {
	s.processed = append(s.processed, obs)
	return nil, model.ActionCreate
}

func (s *statusSink) Resolve(owner model.ObjectRef, reason string) {
	s.resolved = append(s.resolved, struct {
		owner  model.ObjectRef
		reason string
	}{owner: owner, reason: reason})
}

func (s *statusSink) ResolveObserved(*model.Observation) {}

func TestAdmissionWarningTextUsesFirstValidWarning(t *testing.T) {
	warnings := []interface{}{
		map[string]interface{}{"warning": "expression is invalid"},
	}
	if got := admissionWarningText(warnings); got != "expression is invalid" {
		t.Fatalf("warning text = %q", got)
	}
	if got := admissionWarningText([]interface{}{map[string]interface{}{}}); got !=
		"invalid CEL expression" {
		t.Fatalf("fallback warning = %q", got)
	}
}

func TestFailureSignalIncludesReasonAndMessage(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "db", "namespace": "apps",
		},
		"status": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{
				"type": "Ready", "status": "False",
				"reason": "Failed", "message": "replicas unavailable",
			},
		}},
	}}
	signal := failureSignal(obj, "customresource", defaultConditionRules())
	if signal == nil ||
		signal.Hint != "Ready=False: Failed — replicas unavailable" {
		t.Fatalf("unexpected failure signal: %+v", signal)
	}
	if signal.Owner.Name != "db" || signal.Owner.Namespace != "apps" {
		t.Fatalf("unexpected owner: %+v", signal.Owner)
	}
}

func TestReasonForCoversBuiltInResources(t *testing.T) {
	tests := map[string]string{
		"apiservice":                constant.ReasonAPIServiceFailure,
		"mutatingadmissionpolicy":   constant.ReasonMutatingAdmissionPolicyInvalid,
		"certificatesigningrequest": constant.ReasonCertificateSigningRequestFailure,
		"flowschema":                constant.ReasonAPIPriorityAndFairnessFailure,
		"endpoints":                 constant.ReasonServiceNoEndpoints,
		"resourceclaim":             constant.ReasonResourceClaimFailure,
		"unknown":                   constant.ReasonCustomResourceFailure,
	}
	for resource, want := range tests {
		if got := reasonFor(resource); got != want {
			t.Errorf("reasonFor(%q) = %q, want %q", resource, got, want)
		}
	}
}

func TestNestedStringValuesTraversesObjectsAndArrays(t *testing.T) {
	value := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"name": "api"},
			map[string]interface{}{"name": "web"},
		},
	}
	got := nestedStringValues(value, []string{"items", "name"})
	if len(got) != 2 || got[0] != "api" || got[1] != "web" {
		t.Fatalf("nested values = %v", got)
	}
	if got := nestedStringValues(42, []string{}); got != nil {
		t.Fatalf("non-string leaf = %v", got)
	}
}

func TestAdmissionPolicySignalReportsWarningsAndConditions(t *testing.T) {
	withWarning := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "policy"},
		"status": map[string]interface{}{
			"typeChecking": map[string]interface{}{
				"expressionWarnings": []interface{}{
					map[string]interface{}{"warning": "bad expression"},
				},
			},
		},
	}}
	if got := admissionPolicySignal(withWarning); got == nil ||
		got.Hint != "type checking reported 1 expression warning(s): bad expression" {
		t.Fatalf("warning signal = %+v", got)
	}
	withCondition := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "policy"},
		"status": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{"type": "Valid", "status": "False"},
		}},
	}}
	if got := admissionPolicySignal(withCondition); got == nil ||
		got.Reason != constant.ReasonAdmissionPolicyInvalid {
		t.Fatalf("condition signal = %+v", got)
	}
}

func TestCertificateSignalReportsApprovedRequestWithoutCertificate(
	t *testing.T,
) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	created := now.Add(-11 * time.Minute)
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "request", "creationTimestamp": created.Format(time.RFC3339),
		},
		"status": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{"type": "Approved", "status": "True"},
		}},
	}}
	got := certificateSignal(obj, "certificaterequest", now)
	if got == nil || !strings.Contains(got.Hint, "approved") {
		t.Fatalf("certificate signal = %+v", got)
	}
}

func TestStatusJSONIsStableAndSafe(t *testing.T) {
	monitor := &Monitor{generation: 3}
	data, err := monitor.StatusJSON()
	if err != nil {
		t.Fatal(err)
	}
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatal(err)
	}
	if status.State != "stopped" || status.Generation != 3 {
		t.Fatalf("unexpected status: %+v", status)
	}
}
