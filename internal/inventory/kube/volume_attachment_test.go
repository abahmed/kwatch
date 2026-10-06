package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A VolumeAttachment that is being deleted says so and since when, which
// is how a detach that never finishes is noticed.
func TestVolumeAttachmentDescribesItsDeletion(t *testing.T) {
	when := time.Date(2024, 12, 18, 16, 9, 18, 0, time.UTC)
	stamp := metav1.NewTime(when)
	pv := "pv-1"
	va := &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "csi-1", DeletionTimestamp: &stamp,
		},
		Spec: storagev1.VolumeAttachmentSpec{
			NodeName: "n1",
			Source:   storagev1.VolumeAttachmentSource{PersistentVolumeName: &pv},
		},
	}

	desc, ok := kube.VolumeAttachmentSchema{}.Describe(va)

	assert.True(t, ok)
	deleting, _ := desc.Attributes[kube.AttrDeleting].AsBool()
	assert.True(t, deleting)
	assert.True(t, desc.Attributes[kube.AttrDeletingSince].AsTime().Equal(when))
}

func TestVolumeAttachmentThatIsNotDeletingSaysSo(t *testing.T) {
	va := &storagev1.VolumeAttachment{ObjectMeta: metav1.ObjectMeta{Name: "c"}}

	desc, _ := kube.VolumeAttachmentSchema{}.Describe(va)

	deleting, known := desc.Attributes[kube.AttrDeleting].AsBool()
	assert.True(t, known)
	assert.False(t, deleting)
	assert.True(t, desc.Attributes[kube.AttrDeletingSince].AsTime().IsZero())
}
