//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioServiceWithoutEndpoints(t *testing.T) {
	runScenario(t, "networking.service-endpoint", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		_, err := e.Client.CoreV1().Services(namespace).Create(ctx,
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-service"},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "does-not-exist"},
					Ports:    []corev1.ServicePort{{Port: 8080}},
				},
			}, metav1.CreateOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "empty-service",
			Reason: "ServiceNoEndpoints", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioPersistentVolumeClaimFailure(t *testing.T) {
	runScenario(t, "storage.pvc-pv", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		_, err := e.Client.CoreV1().PersistentVolumeClaims(namespace).Create(
			ctx, &corev1.PersistentVolumeClaim{
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
			}, metav1.CreateOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := wait.PollUntilContextTimeout(waitCtx, 500*time.Millisecond,
			5*time.Minute, true, func(ctx context.Context) (bool, error) {
				pvc, err := e.Client.CoreV1().PersistentVolumeClaims(namespace).
					Get(ctx, "missing-volume", metav1.GetOptions{})
				return err == nil && pvc.Status.Phase == corev1.ClaimPending, nil
			}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "missing-volume",
			Reason: "PersistentVolumeClaimFailure", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func stringPtr(value string) *string {
	return &value
}
