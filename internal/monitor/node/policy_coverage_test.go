package node

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/constant"
)

func TestNodePolicyCoversConditionsBootstrapAndDeletion(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	if IsNew(nil, now) || DetectDeletionIssue(nil, now) != nil {
		t.Fatal("nil node produced a finding")
	}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "worker", CreationTimestamp: metav1.NewTime(now.Add(-time.Minute)),
	}}
	if !IsNew(node, now) {
		t.Fatal("new node was not within bootstrap grace")
	}
	for _, test := range []struct {
		condition corev1.NodeCondition
		want      string
	}{
		{corev1.NodeCondition{Type: corev1.NodeReady,
			Status: corev1.ConditionFalse}, constant.ReasonNodeNotReady},
		{corev1.NodeCondition{Type: corev1.NodeMemoryPressure,
			Status: corev1.ConditionTrue}, string(corev1.NodeMemoryPressure)},
		{corev1.NodeCondition{Type: corev1.NodeDiskPressure,
			Status: corev1.ConditionTrue}, string(corev1.NodeDiskPressure)},
		{corev1.NodeCondition{Type: corev1.NodeReady,
			Status: corev1.ConditionTrue}, ""},
	} {
		if got := ConditionReason(test.condition); got != test.want {
			t.Fatalf("condition reason = %q, want %q", got, test.want)
		}
	}
	deleting := metav1.NewTime(now.Add(-11 * time.Minute))
	node.DeletionTimestamp = &deleting
	node.Finalizers = []string{"cleanup.example"}
	if got := DetectDeletionIssue(node, now); got == nil {
		t.Fatal("stuck node deletion was missed")
	}
	if got := DetectDeletionIssue(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{
			DeletionTimestamp: &deleting,
		}}, now,
	); got != nil {
		t.Fatal("node without finalizers produced a finding")
	}
}
