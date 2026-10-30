package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestCronJobReferencesTheClaimsOfItsTemplate(t *testing.T) {
	cron := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "shop"},
		Spec:       batchv1.CronJobSpec{Schedule: "0 2 * * *"},
	}
	cron.Spec.JobTemplate.Spec.Template.Spec.Volumes = []corev1.Volume{{
		Name: "data", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: "backups",
			},
		},
	}}

	described, ok := CronJobSchema{}.Describe(cron)

	require.True(t, ok)
	assert.Contains(t, described.Relations[inventory.References],
		inventory.CoreID(KindPVC, "shop", "backups"))
}
