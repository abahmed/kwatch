package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestPodSchemaRecordsReadinessGates(t *testing.T) {
	p := pod("gated")
	p.Spec.ReadinessGates = []corev1.PodReadinessGate{
		{ConditionType: "target-health.example/lb"},
		{ConditionType: "mesh-ready"},
	}
	p.Status.Conditions = append(p.Status.Conditions, corev1.PodCondition{
		Type: "mesh-ready", Status: corev1.ConditionFalse,
		Reason: "Syncing",
	})

	desc, ok := kube.PodSchema{}.Describe(p)

	require.True(t, ok)
	assert.Equal(t, "target-health.example/lb,mesh-ready",
		desc.Attributes[kube.AttrReadinessGates].AsText())
	assert.Equal(t, "False",
		desc.Attributes[kube.ConditionKey("mesh-ready")].AsText())
}

func TestPodSchemaOmitsReadinessGatesWhenNoneDeclared(t *testing.T) {
	desc, ok := kube.PodSchema{}.Describe(pod("plain"))

	require.True(t, ok)
	_, has := desc.Attributes[kube.AttrReadinessGates]
	assert.False(t, has)
}
