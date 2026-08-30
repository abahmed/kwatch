package statuswatch

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestFailureSignalUsesConfiguredConditionRules(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "db"},
		"status": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{"type": "Healthy", "status": "False", "reason": "ReplicaLag"},
		}},
	}}
	rules := map[string]map[string]bool{"Healthy": {"False": true}}
	if signal := failureSignal(obj, "customresource", "db", rules); signal == nil || signal.Hint != "Healthy=False: ReplicaLag" {
		t.Fatalf("unexpected signal: %+v", signal)
	}
	if signal := failureSignal(obj, "customresource", "db", defaultConditionRules()); signal != nil {
		t.Fatalf("default rules should ignore custom Healthy condition: %+v", signal)
	}
}
