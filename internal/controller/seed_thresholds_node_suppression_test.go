package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/config"
)

func TestSeedThresholdsNodeConditionSuppressed(t *testing.T) {
	index := &config.SuppressionIndex{
		NodeReasons: []string{"KubeletNotReady"},
	}
	thresholds := seedThresholds{nodeSuppression: index}

	suppressed := thresholds.nodeConditionSuppressed(corev1.NodeCondition{
		Reason: "KubeletNotReady",
	})
	if !suppressed {
		t.Fatalf("expected condition to be suppressed")
	}

	notSuppressed := thresholds.nodeConditionSuppressed(corev1.NodeCondition{
		Reason: "SomethingElse",
	})
	if notSuppressed {
		t.Fatalf("did not expect unrelated reason to be suppressed")
	}
}

func TestSeedThresholdsNodeConditionSuppressedNilIndex(t *testing.T) {
	thresholds := seedThresholds{}

	if thresholds.nodeConditionSuppressed(corev1.NodeCondition{
		Reason: "KubeletNotReady",
	}) {
		t.Fatalf("expected nil suppression index to never suppress")
	}
}
