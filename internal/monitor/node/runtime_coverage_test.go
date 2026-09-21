package node

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestNodeRuntimeProcessesPressureRecoveryDeletionAndOvercommit(
	t *testing.T,
) {
	sink := &nodeSinkRecorder{}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, nil, time.Now,
	)
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	runtime.now = func() time.Time { return now }
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue,
			Reason: "memory", Message: "low memory",
		}}}}
	if err := runtime.ProcessNodeObject(node, false); err != nil {
		t.Fatal(err)
	}
	if len(sink.findings) != 1 || sink.findings[0].Reason !=
		string(corev1.NodeMemoryPressure) {
		t.Fatalf("pressure findings = %+v", sink.findings)
	}
	node.Status.Conditions[0].Status = corev1.ConditionFalse
	if err := runtime.ProcessNodeObject(node, false); err != nil {
		t.Fatal(err)
	}
	runtime.ProcessNodeResourceOvercommit(
		"NodeOvercommit", "worker", "overcommit", "",
	)
	if len(sink.findings) != 2 ||
		sink.findings[1].Severity != model.SeverityWarning {
		t.Fatalf("overcommit findings = %+v", sink.findings)
	}
	if err := runtime.ProcessNodeObject(node, true); err != nil {
		t.Fatal(err)
	}
}

func TestNodeRuntimeSourceConfigurationIsOneTime(t *testing.T) {
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, &nodeSinkRecorder{}, nil, time.Now,
	)
	if err := runtime.ConfigureSources(Sources{}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfigureSources(Sources{}); err == nil {
		t.Fatal("second source configuration succeeded")
	}
	runtime.beginProcessing()
	if err := runtime.ConfigureSources(Sources{}); err == nil {
		t.Fatal("source configuration after processing succeeded")
	}
}
