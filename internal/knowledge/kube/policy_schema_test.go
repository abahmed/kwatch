package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func text(d kube.Description, name string) string {
	return d.Attributes[name].AsText()
}

func TestPDBSchemaRecordsDisruptionBudget(t *testing.T) {
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "x"},
			},
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: 0, CurrentHealthy: 1, DesiredHealthy: 2,
		},
	}
	d, ok := kube.PDBSchema{}.Describe(pdb)
	assert.True(t, ok)
	assert.Equal(t, kube.KindPDB, d.ID.Kind)
	n, _ := d.Attributes[kube.AttrDesiredHealthy].AsNumber()
	assert.Equal(t, 2.0, n)
	assert.Contains(t, text(d, kube.AttrSelector), "app")
	assert.Nil(t, kube.PDBSchema{}.Diff(pdb, pdb))
}

func TestQuotaSchemaReportsExhaustedResources(t *testing.T) {
	q := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "ns"},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				"pods": mustQuantity("5"), "cpu": mustQuantity("4"),
			},
			Used: corev1.ResourceList{
				"pods": mustQuantity("5"), "cpu": mustQuantity("1"),
			},
		},
	}
	d, ok := kube.QuotaSchema{}.Describe(q)
	assert.True(t, ok)
	assert.Equal(t, "pods", text(d, kube.AttrExhausted))
	assert.Equal(t, []knowledge.EntityID{
		knowledge.NewEntityID(kube.KindNamespace, "", "ns"),
	}, d.Relations[knowledge.Constrains])
}

func TestQuotaSchemaDiffDetectsHardLimitChange(t *testing.T) {
	mk := func(cpu string) *corev1.ResourceQuota {
		return &corev1.ResourceQuota{Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{"cpu": mustQuantity(cpu)},
		}}
	}
	s := kube.QuotaSchema{}
	assert.Nil(t, s.Diff(mk("1"), mk("1")))
	got := s.Diff(mk("1"), mk("2"))
	assert.Len(t, got, 1)
	assert.Equal(t, "spec.hard", got[0].Path)
}

func TestNetworkPolicySchemaDetectsEgressDenyAndChanges(t *testing.T) {
	np := func(egress bool) *networkingv1.NetworkPolicy {
		p := &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "n", Namespace: "ns"},
		}
		if egress {
			p.Spec.PolicyTypes = []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			}
		}
		return p
	}
	s := kube.NetworkPolicySchema{}
	d, ok := s.Describe(np(true))
	assert.True(t, ok)
	deny, _ := d.Attributes[kube.AttrDeniesEgress].AsBool()
	assert.True(t, deny)
	d, _ = s.Describe(np(false))
	deny, _ = d.Attributes[kube.AttrDeniesEgress].AsBool()
	assert.False(t, deny)
	assert.NotEmpty(t, text(d, kube.AttrPolicyDigest))
	assert.Nil(t, s.Diff(np(true), np(true)))
	assert.Len(t, s.Diff(np(true), np(false)), 1)
}

func TestVolumeAttachmentSchemaRecordsAttachError(t *testing.T) {
	pv := "pv1"
	va := &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "va"},
		Spec: storagev1.VolumeAttachmentSpec{
			NodeName: "n1",
			Source: storagev1.VolumeAttachmentSource{
				PersistentVolumeName: &pv,
			},
		},
		Status: storagev1.VolumeAttachmentStatus{
			AttachError: &storagev1.VolumeError{Message: "timeout"},
		},
	}
	s := kube.VolumeAttachmentSchema{}
	d, ok := s.Describe(va)
	assert.True(t, ok)
	assert.Equal(t, "timeout", text(d, kube.AttrAttachError))
	assert.Len(t, d.Relations[knowledge.RunsOn], 1)
	assert.Len(t, d.Relations[knowledge.References], 1)
	assert.Nil(t, s.Diff(va, va))
}

func TestWebhookSchemaLinksServicesAndFailurePolicy(t *testing.T) {
	ignore := admissionv1.Ignore
	client := admissionv1.WebhookClientConfig{
		Service: &admissionv1.ServiceReference{
			Namespace: "sys", Name: "hook",
		},
	}
	mutating := &admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "m"},
		Webhooks: []admissionv1.MutatingWebhook{
			{ClientConfig: client, FailurePolicy: &ignore},
			{ClientConfig: admissionv1.WebhookClientConfig{}},
		},
	}
	validating := &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "v"},
		Webhooks:   []admissionv1.ValidatingWebhook{{ClientConfig: client}},
	}
	m := kube.WebhookSchema{Mutating: true}
	d, ok := m.Describe(mutating)
	assert.True(t, ok)
	assert.Equal(t, "Ignore,Fail", text(d, kube.AttrFailurePolicy))
	assert.Len(t, d.Relations[knowledge.Serves], 1)
	_, ok = m.Describe(validating)
	assert.False(t, ok)

	v := kube.WebhookSchema{}
	d, ok = v.Describe(validating)
	assert.True(t, ok)
	assert.Equal(t, kube.KindValidatingHook, d.ID.Kind)
	_, ok = v.Describe(mutating)
	assert.False(t, ok)
	assert.Nil(t, v.Diff(validating, validating))
}

func TestLimitRangeSchemaFlagsContradictoryLimits(t *testing.T) {
	lr := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: "lr", Namespace: "ns"},
		Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
			Min: corev1.ResourceList{"cpu": mustQuantity("2")},
			Max: corev1.ResourceList{"cpu": mustQuantity("1")},
			DefaultRequest: corev1.ResourceList{
				"memory": mustQuantity("2Gi"),
			},
			Default: corev1.ResourceList{"memory": mustQuantity("1Gi")},
		}}},
	}
	s := kube.LimitRangeSchema{}
	d, ok := s.Describe(lr)
	assert.True(t, ok)
	got := text(d, kube.AttrLimitsInvalid)
	assert.Contains(t, got, "cpu min 2 exceeds max 1")
	assert.Contains(t, got, "memory default request")
	assert.Nil(t, s.Diff(lr, lr))
	d, _ = s.Describe(&corev1.LimitRange{})
	assert.Empty(t, text(d, kube.AttrLimitsInvalid))
}
