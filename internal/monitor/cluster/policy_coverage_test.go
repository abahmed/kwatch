package cluster

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/constant"
)

func TestClusterResourcePoliciesCoverLimitQuotaNamespaceAndSecurity(
	t *testing.T,
) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	if DetectLimitRangeIssue(nil) != nil || DetectResourceQuotaIssue(nil) != nil {
		t.Fatal("nil resource produced a finding")
	}
	limit := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: "limits", Namespace: "apps"},
		Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
			Type: corev1.LimitTypeContainer,
			Min: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("2"),
			},
			Max: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("1"),
			},
		}}},
	}
	if DetectLimitRangeIssue(limit) == nil {
		t.Fatal("contradictory limit range was missed")
	}
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "quota", Namespace: "apps"},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("2"),
			},
			Used: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("2"),
			},
		},
	}
	if DetectResourceQuotaIssue(quota) == nil {
		t.Fatal("exhausted resource quota was missed")
	}
	deleting := metav1.NewTime(now.Add(-11 * time.Minute))
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "apps", DeletionTimestamp: &deleting,
		},
		Status: corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating},
	}
	if DetectNamespaceIssue(namespace, now, 0) == nil {
		t.Fatal("stuck namespace was missed")
	}
	namespace.Labels = map[string]string{
		"pod-security.kubernetes.io/enforce": "invalid",
	}
	if DetectPodSecurityLabelIssue(namespace) == nil {
		t.Fatal("invalid pod security level was missed")
	}
	namespace.Labels = map[string]string{
		"pod-security.kubernetes.io/enforce":         "baseline",
		"pod-security.kubernetes.io/enforce-version": "v2",
	}
	if DetectPodSecurityLabelIssue(namespace) == nil {
		t.Fatal("invalid pod security version was missed")
	}
	if reason := constant.ReasonNamespaceStuck; reason == "" {
		t.Fatal("namespace reason is empty")
	}
}
