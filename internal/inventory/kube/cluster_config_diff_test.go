package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func validatingHook(
	policy admissionv1.FailurePolicyType, ca string,
) *admissionv1.ValidatingWebhookConfiguration {
	timeout := int32(10)
	return &admissionv1.ValidatingWebhookConfiguration{
		Webhooks: []admissionv1.ValidatingWebhook{{
			Name:           "check.example.com",
			FailurePolicy:  &policy,
			TimeoutSeconds: &timeout,
			ClientConfig: admissionv1.WebhookClientConfig{
				CABundle: []byte(ca)},
			Rules: []admissionv1.RuleWithOperations{{
				Operations: []admissionv1.OperationType{"CREATE"},
				Rule: admissionv1.Rule{
					APIGroups: []string{""}, APIVersions: []string{"v1"},
					Resources: []string{"pods"},
				},
			}},
		}},
	}
}

func TestWebhookDiffReportsPolicyAndRuleChanges(t *testing.T) {
	before := validatingHook(admissionv1.Fail, "ca-one")
	after := validatingHook(admissionv1.Ignore, "ca-two")
	after.Webhooks[0].Rules[0].Resources = []string{"pods", "deployments"}
	after.Webhooks[0].NamespaceSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{"env": "prod"}}

	got := fieldsByPath(kube.WebhookSchema{}.Diff(before, after))

	prefix := "webhooks.check.example.com."
	assert.Equal(t, "Fail", got[prefix+"failurePolicy"].Before)
	assert.Equal(t, "Ignore", got[prefix+"failurePolicy"].After)
	assert.Equal(t, "env=prod", got[prefix+"namespaceSelector"].After)
	assert.Contains(t, got[prefix+"rules"].After, "pods+deployments")
	ca := got[prefix+"caBundle"]
	assert.NotEmpty(t, ca.After)
	assert.NotContains(t, ca.After, "ca-two")
	assert.NotContains(t, got, prefix+"timeoutSeconds")
}

func TestWebhookDiffIsEmptyWhenNothingChanged(t *testing.T) {
	a := validatingHook(admissionv1.Fail, "ca")
	b := validatingHook(admissionv1.Fail, "ca")

	assert.Empty(t, kube.WebhookSchema{}.Diff(a, b))
	assert.Empty(t, kube.WebhookSchema{}.Diff(a, "not a webhook"))
}

func TestWebhookDiffReportsAddedAndRemovedHooks(t *testing.T) {
	before := validatingHook(admissionv1.Fail, "ca")
	after := validatingHook(admissionv1.Fail, "ca")
	after.Webhooks[0].Name = "other.example.com"

	got := fieldsByPath(kube.WebhookSchema{}.Diff(before, after))

	assert.Equal(t, "added", got["webhooks.other.example.com"].After)
	assert.Equal(t, "removed", got["webhooks.check.example.com"].After)
}

func TestServiceAccountDiffReportsPullSecrets(t *testing.T) {
	before := &corev1.ServiceAccount{ImagePullSecrets: []corev1.
		LocalObjectReference{{Name: "regcred"}}}
	after := &corev1.ServiceAccount{}

	got := fieldsByPath(kube.ServiceAccountSchema{}.Diff(before, after))

	require.Contains(t, got, "imagePullSecrets")
	assert.Equal(t, "regcred", got["imagePullSecrets"].Before)
	assert.Empty(t, kube.ServiceAccountSchema{}.Diff(before, before))
}

func TestStorageClassDiffReportsProvisioningChanges(t *testing.T) {
	retain := corev1.PersistentVolumeReclaimRetain
	wait := storagev1.VolumeBindingWaitForFirstConsumer
	allow := true
	before := &storagev1.StorageClass{Provisioner: "a",
		Parameters: map[string]string{"type": "gp2"}}
	after := &storagev1.StorageClass{Provisioner: "b",
		ReclaimPolicy: &retain, VolumeBindingMode: &wait,
		AllowVolumeExpansion: &allow,
		Parameters:           map[string]string{"type": "gp3"}}

	got := fieldsByPath(kube.StorageClassSchema{}.Diff(before, after))

	assert.Equal(t, "b", got["provisioner"].After)
	assert.Equal(t, "Delete", got["reclaimPolicy"].Before)
	assert.Equal(t, "Retain", got["reclaimPolicy"].After)
	assert.Equal(t, "WaitForFirstConsumer", got["volumeBindingMode"].After)
	assert.Equal(t, "true", got["allowVolumeExpansion"].After)
	assert.Equal(t, "type=gp3", got["parameters"].After)
	assert.Empty(t, kube.StorageClassSchema{}.Diff(before, before))
}

func TestNamespaceDiffReportsOnlyPodSecurityLabels(t *testing.T) {
	namespace := func(level, team string) *corev1.Namespace {
		return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"pod-security.kubernetes.io/enforce": level,
				"team":                               team,
			}}}
	}
	before, after := namespace("baseline", "a"), namespace("restricted", "b")

	got := kube.NamespaceSchema{}.Diff(before, after)

	require.Len(t, got, 1)
	assert.Equal(t, "labels.pod-security", got[0].Path)
	assert.Equal(t, "enforce=baseline", got[0].Before)
	assert.Equal(t, "enforce=restricted", got[0].After)
}

func TestLimitRangeDiffReportsLimits(t *testing.T) {
	limit := func(max string) *corev1.LimitRange {
		return &corev1.LimitRange{Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{{
				Type: corev1.LimitTypeContainer,
				Max: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse(max)},
			}}}}
	}

	got := kube.LimitRangeSchema{}.Diff(limit("1Gi"), limit("64Mi"))

	require.Len(t, got, 1)
	assert.Equal(t, "spec.limits", got[0].Path)
	assert.Contains(t, got[0].Before, "max memory=1Gi")
	assert.Contains(t, got[0].After, "max memory=64Mi")
	assert.Empty(t, kube.LimitRangeSchema{}.Diff(limit("1Gi"), limit("1Gi")))
}
