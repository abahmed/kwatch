package kube_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestMaintenanceAnnotationValueIsBoundedAndRedacted(t *testing.T) {
	cfg := kube.MaintenanceAnnotations{On: "m"}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pod := maintenancePod(map[string]string{
		"m": "password=hunter2 " + strings.Repeat("x", 5000),
	})
	attrs := podAttributes(kube.NewTranslator(kube.PodSchema{}).
		WithMaintenance(cfg).Added(pod, true, at))
	text := attrs[kube.AttrMaintenance].AsText()
	assert.NotContains(t, text, "hunter2")
	assert.LessOrEqual(t, len(text), 300)
}

func TestConditionKeyMatchesTheStoredName(t *testing.T) {
	long := strings.Repeat("T", 100)
	p := pod("p")
	p.Status.Conditions = append(p.Status.Conditions,
		corev1.PodCondition{Type: corev1.PodConditionType(long),
			Status: corev1.ConditionTrue},
		corev1.PodCondition{Type: "example.com/gate",
			Status: corev1.ConditionFalse, Reason: "Waiting"})
	desc, ok := kube.PodSchema{}.Describe(p)
	assert.True(t, ok)

	assert.Contains(t, desc.Attributes, kube.ConditionKey(long))
	for name := range desc.Attributes {
		assert.NotContains(t, name, "example.com",
			"a dotted type would make condition.<type>.reason ambiguous")
	}
}
