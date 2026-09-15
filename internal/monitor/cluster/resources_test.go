package cluster

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/constant"
)

func TestDetectResourceQuotaIssueFindsExhaustedHardLimit(t *testing.T) {
	quota := &corev1.ResourceQuota{
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("2"),
			},
			Used: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("2"),
			},
		},
	}

	if got := DetectResourceQuotaIssue(quota); got == nil {
		t.Fatal("expected exhausted quota finding")
	}
}

func TestDetectPodSecurityLabelIssueRejectsInvalidLevel(t *testing.T) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			"pod-security.kubernetes.io/enforce": "invalid",
		}},
	}

	got := DetectPodSecurityLabelIssue(namespace)
	if got == nil || got.Reason != constant.ReasonPodSecurityPolicyInvalid {
		t.Fatal("expected invalid Pod Security label finding")
	}
}
