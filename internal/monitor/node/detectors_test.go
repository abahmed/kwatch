package node

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/constant"
)

func TestConditionReasonMapsNotReady(t *testing.T) {
	condition := corev1.NodeCondition{
		Type:   corev1.NodeReady,
		Status: corev1.ConditionFalse,
	}

	if got := ConditionReason(condition); got != constant.ReasonNodeNotReady {
		t.Fatalf("unexpected reason %q", got)
	}
}

func TestDetectDeletionIssueRequiresGracePeriod(t *testing.T) {
	deletedAt := metav1.NewTime(time.Date(
		2026, 1, 1, 12, 0, 0, 0, time.UTC,
	))
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "worker-1",
			DeletionTimestamp: &deletedAt,
			Finalizers:        []string{"example.com/cleanup"},
		},
	}

	if got := DetectDeletionIssue(
		node,
		deletedAt.Add(11*time.Minute),
	); got == nil {
		t.Fatal("expected stuck Node finding")
	}
}
