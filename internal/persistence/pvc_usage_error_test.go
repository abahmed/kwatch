package persistence

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/clock"
)

func TestGetPvcUsageWithErrorTreatsMissingStateAsEmpty(t *testing.T) {
	manager := NewManagerWithClock(
		fake.NewSimpleClientset(), "kwatch", clock.RealClock{},
	)
	usage, err := manager.GetPvcUsageWithError(context.Background())
	if err != nil || usage != nil {
		t.Fatalf("missing PVC state = %#v, %v; want nil, nil", usage, err)
	}
}

func TestGetPvcUsageWithErrorReportsCorruptState(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: pvcConfigMapName, Namespace: "kwatch"},
		Data:       map[string]string{pvcUsageKey: "{"},
	})
	manager := NewManagerWithClock(client, "kwatch", clock.RealClock{})
	_, err := manager.GetPvcUsageWithError(context.Background())
	if err == nil {
		t.Fatal("corrupt PVC state returned no error")
	}
}

func TestGetPvcUsageWithErrorReportsUnavailableState(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("get", "configmaps", func(
		action clienttesting.Action,
	) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			corev1.Resource("configmaps"), pvcConfigMapName,
			fmt.Errorf("forbidden"),
		)
	})
	manager := NewManagerWithClock(client, "kwatch", clock.RealClock{})
	_, err := manager.GetPvcUsageWithError(context.Background())
	if err == nil {
		t.Fatal("unavailable PVC state returned no error")
	}
}
