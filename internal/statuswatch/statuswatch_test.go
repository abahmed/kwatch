package statuswatch

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	kwcontext "github.com/abahmed/kwatch/internal/context"
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

func TestRebuildGraphTracksCustomResourceOwner(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	monitor := &Monitor{graph: graph}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "db", "namespace": "apps",
			"ownerReferences": []interface{}{map[string]interface{}{
				"kind": "Database", "name": "db-prod",
			}},
		},
	}}

	monitor.rebuildGraph(obj)

	if got := graph.DependenciesOf("customresource", "apps", "db"); len(got) != 1 || got[0] != "database/apps/db-prod" {
		t.Fatalf("unexpected custom resource dependencies: %v", got)
	}
}
