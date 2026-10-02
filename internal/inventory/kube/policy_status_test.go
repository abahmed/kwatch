package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestPDBSchemaRecordsSyncFailedCondition(t *testing.T) {
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns"},
		Status: policyv1.PodDisruptionBudgetStatus{
			Conditions: []metav1.Condition{{
				Type: "DisruptionAllowed", Status: metav1.ConditionFalse,
				Reason: "SyncFailed", Message: "found no controllers",
			}},
		},
	}

	d, _ := kube.PDBSchema{}.Describe(pdb)

	key := kube.ConditionKey("DisruptionAllowed")
	assert.Equal(t, "False", text(d, key))
	assert.Equal(t, "SyncFailed", text(d, key+kube.AttrConditionReason))
	assert.Equal(t, "found no controllers",
		text(d, key+kube.AttrConditionMessage))
}

func TestQuotaSchemaReportsResourcesNearLimit(t *testing.T) {
	q := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "ns"},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				"pods": mustQuantity("10"), "cpu": mustQuantity("4"),
				"memory": mustQuantity("1Gi"), "secrets": mustQuantity("0"),
			},
			Used: corev1.ResourceList{
				"pods": mustQuantity("9"), "cpu": mustQuantity("4"),
				"memory":  mustQuantity("512Mi"),
				"secrets": mustQuantity("0"),
			},
		},
	}

	d, _ := kube.QuotaSchema{}.Describe(q)

	assert.Equal(t, "pods=90%", text(d, kube.AttrQuotaNearLimit))
	assert.Equal(t, "cpu,secrets", text(d, kube.AttrExhausted))
}

func TestVolumeAttachmentSchemaRecordsDetachError(t *testing.T) {
	va := &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "csi-1"},
		Spec:       storagev1.VolumeAttachmentSpec{NodeName: "n1"},
		Status: storagev1.VolumeAttachmentStatus{
			Attached: true,
			DetachError: &storagev1.VolumeError{
				Message: "rpc error: volume in use",
			},
		},
	}

	d, _ := kube.VolumeAttachmentSchema{}.Describe(va)

	assert.Equal(t, "rpc error: volume in use",
		text(d, kube.AttrDetachError))
}
