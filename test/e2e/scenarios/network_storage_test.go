//go:build e2e

package scenarios

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioServiceWithoutEndpoints(t *testing.T) {
	inNamespace(t, "networking.service-endpoint", func(s *Scenario) {
		services := s.Env.Client.CoreV1().Services(s.Namespace)
		_, err := services.Create(s.Ctx, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-service"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "does-not-exist"},
				Ports:    []corev1.ServicePort{{Port: 8080}},
			},
		}, metav1.CreateOptions{})
		s.Must(err)

		// A Service that was never backed is empty on purpose. One that
		// emptied after its selector changed was broken by the edit.
		_, err = services.Patch(s.Ctx, "empty-service",
			types.MergePatchType,
			[]byte(`{"spec":{"selector":{"app":"renamed"}}}`),
			metav1.PatchOptions{})
		s.Must(err)
		s.ExpectIncident("empty-service", "ServiceNoEndpoints",
			detectors.DefaultNoEndpoints)
	})
}

func TestScenarioPersistentVolumeClaimFailure(t *testing.T) {
	inNamespace(t, "storage.pvc-pv", func(s *Scenario) {
		claims := s.Env.Client.CoreV1().PersistentVolumeClaims(s.Namespace)
		_, err := claims.Create(s.Ctx, &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "missing-volume"},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: stringPtr("kwatch-e2e-missing"),
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		}, metav1.CreateOptions{})
		s.Must(err)
		s.ExpectIncident("missing-volume", "PersistentVolumeClaimFailure",
			detectors.DefaultClaimPending)
		s.ExpectRoot(harness.RootExpectation{
			Root: "persistentvolumeclaim/" + s.Namespace +
				"/missing-volume",
			Tier:        "notify",
			MaxMessages: 2,
		})
	})
}

func stringPtr(value string) *string {
	return &value
}
