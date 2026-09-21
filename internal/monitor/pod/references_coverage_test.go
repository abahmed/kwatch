package pod

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
)

func TestDetectReferenceIssuesFindsRequiredReferences(t *testing.T) {
	optional := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "api"},
		Spec: corev1.PodSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: "pull"}},
			Volumes: []corev1.Volume{
				{VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
					SecretName: "volume-secret",
				}}},
				{VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
					Name: "volume-config",
				}}},
				{VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{{
						Secret:    &corev1.SecretProjection{Name: "projected-secret"},
						ConfigMap: &corev1.ConfigMapProjection{Name: "projected-config"},
					}},
				}}},
				{VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
					SecretName: "optional-secret", Optional: &optional,
				}}},
			},
			Containers: []corev1.Container{{
				Name: "api",
				EnvFrom: []corev1.EnvFromSource{{
					SecretRef:    &corev1.SecretEnvSource{Name: "env-secret"},
					ConfigMapRef: &corev1.ConfigMapEnvSource{Name: "env-config"},
				}},
				Env: []corev1.EnvVar{
					{Name: "SECRET", ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{Name: "key-secret"},
					}},
					{Name: "CONFIG", ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{Name: "key-config"},
					}},
				},
			}},
		},
	}
	secretIndex := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	configIndex := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	serviceIndex := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	issues := DetectReferenceIssues(pod, ReferenceListers{
		Secret:         corev1listers.NewSecretLister(secretIndex),
		ConfigMap:      corev1listers.NewConfigMapLister(configIndex),
		ServiceAccount: corev1listers.NewServiceAccountLister(serviceIndex),
	})
	if len(issues) != 10 {
		t.Fatalf("reference issues = %d, want 10: %#v", len(issues), issues)
	}
	seen := map[string]bool{}
	for _, issue := range issues {
		seen[issue.Reason] = true
	}
	for _, reason := range []string{
		constant.ReasonProjectedSecretMissing,
		constant.ReasonProjectedConfigMapMissing,
		constant.ReasonServiceAccountMissing,
	} {
		if !seen[reason] {
			t.Fatalf("missing reference reason %q in %#v", reason, seen)
		}
	}
}

func TestDeletionAndDescriptorHelpers(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	if DetectReferenceIssues(nil, ReferenceListers{}) != nil {
		t.Fatal("nil pod returned references")
	}
	if DetectDeletionIssue(&corev1.Pod{}, now) != nil {
		t.Fatal("pod without finalizers produced deletion issue")
	}
	deleting := metav1.NewTime(now.Add(-11 * time.Minute))
	if got := DetectDeletionIssue(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api", DeletionTimestamp: &deleting,
			Finalizers: []string{"cleanup.example"},
		},
	}, now); got == nil || got.Reason != constant.ReasonPodStuckTerminating {
		t.Fatalf("deletion issue = %#v", got)
	}
	if Descriptor().Name == "" {
		t.Fatal("pod descriptor has no name")
	}
}
