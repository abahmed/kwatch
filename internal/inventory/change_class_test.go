package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChangeClassifyByType(t *testing.T) {
	deployment := CoreID("deployment", "shop", "api")
	field := func(path string) []FieldChange {
		return []FieldChange{{Path: path, Before: "a", After: "b"}}
	}
	tests := []struct {
		name   string
		change Change
		want   ChangeClass
	}{
		{"rollout", Change{Entity: deployment,
			Fields: field("spec.template")}, ClassRollout},
		{"image wins over template", Change{Entity: deployment,
			Fields: append(field("template.references"),
				field("containers[api].image")...)}, ClassImage},
		{"scale", Change{Entity: deployment,
			Fields: field("spec.replicas")}, ClassScale},
		{"autoscaler bound", Change{
			Entity: CoreID("horizontalpodautoscaler", "shop", "api"),
			Fields: field("spec.maxReplicas")}, ClassScale},
		{"configmap data", Change{
			Entity: CoreID("configmap", "shop", "app-config"),
			Fields: field("data.url")}, ClassConfig},
		{"secret data", Change{Entity: CoreID("secret", "shop", "db"),
			Fields: field("data.password")}, ClassConfig},
		{"rbac", Change{Entity: CoreID("rolebinding", "shop", "read"),
			Fields: field("subjects")}, ClassRBAC},
		{"network policy", Change{
			Entity: CoreID("networkpolicy", "shop", "deny"),
			Fields: field("spec")}, ClassPolicy},
		{"quota", Change{Entity: CoreID("resourcequota", "shop", "q"),
			Fields: field("spec.hard")}, ClassPolicy},
		{"service selector", Change{
			Entity: CoreID("service", "shop", "api"),
			Fields: field("spec.selector")}, ClassLabels},
		{"pod labels", Change{Entity: CoreID("pod", "shop", "api-1"),
			Fields: field("metadata.labels")}, ClassLabels},
		{"taints", Change{Entity: CoreID("node", "", "n1"),
			Fields: field("spec.taints")}, ClassTaints},
		{"cordon", Change{Entity: CoreID("node", "", "n1"),
			Fields: field("spec.unschedulable")}, ClassTaints},
		{"custom resource spec", Change{
			Entity: NewEntityID("example.com", "widget", "shop", "w"),
			Fields: field("spec")}, ClassCRDSpec},
		{"node added", Change{Entity: CoreID("node", "", "n2"),
			Created: true}, ClassNodeAdded},
		{"node removed", Change{Entity: CoreID("node", "", "n2"),
			Deleted: true}, ClassNodeRemoved},
		{"created", Change{Entity: deployment, Created: true},
			ClassCreated},
		{"deleted", Change{Entity: deployment, Deleted: true},
			ClassDeleted},
		{"other", Change{Entity: deployment,
			Fields: field("spec.paused")}, ClassOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.change.Classify())
		})
	}
}

func TestChangeReceivedAtFallsBackToAt(t *testing.T) {
	change := Change{At: testTime}
	assert.Equal(t, testTime, change.ReceivedAt())
	change.Observed = testTime.Add(3)
	assert.Equal(t, testTime.Add(3), change.ReceivedAt())
}

func TestChangeClassifyDeletionTimestampIsNotConfig(t *testing.T) {
	pod := CoreID("pod", "shop", "api-1")
	change := Change{Entity: pod, Fields: []FieldChange{{
		Path: "metadata.deletionTimestamp", After: "set"}}}
	assert.Equal(t, ClassOther, change.Classify())
}
