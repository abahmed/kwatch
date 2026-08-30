package handler

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDetectLimitRangeIssueFindsInvalidDefault(t *testing.T) {
	limitRange := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: "defaults", Namespace: "apps"},
		Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
			Type:    corev1.LimitTypeContainer,
			Min:     corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
			Default: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")},
		}}},
	}

	sig := DetectLimitRangeIssue(limitRange)
	if sig == nil || sig.Reason != "LimitRangeInvalid" {
		t.Fatalf("expected invalid limit range signal, got %+v", sig)
	}
}

func TestDetectLimitRangeIssueAcceptsValidConstraints(t *testing.T) {
	limitRange := &corev1.LimitRange{
		Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
			Type:           corev1.LimitTypeContainer,
			Min:            corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("128Mi")},
			Max:            corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
			Default:        corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")},
			DefaultRequest: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("256Mi")},
		}}},
	}

	if sig := DetectLimitRangeIssue(limitRange); sig != nil {
		t.Fatalf("valid limit range produced signal: %+v", sig)
	}
}
