package persistence

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/model"
)

func TestWithStatePrefixWritesUnderScopedName(t *testing.T) {
	client := fake.NewSimpleClientset()
	sm := NewManagerWithClock(
		client, "kwatch", clock.RealClock{}, WithStatePrefix("team"),
	)

	err := sm.SaveRuntimeSession(
		context.Background(), model.RuntimeSession{},
	)
	if err != nil {
		t.Fatalf("SaveRuntimeSession() returned error: %v", err)
	}

	_, err = client.CoreV1().ConfigMaps("kwatch").Get(
		context.Background(), "team-state", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("expected team-state ConfigMap, got error: %v", err)
	}
}

func TestWithStatePrefixDefaultKeepsHistoricalName(t *testing.T) {
	client := fake.NewSimpleClientset()
	sm := NewManagerWithClock(
		client, "kwatch", clock.RealClock{}, WithStatePrefix("kwatch"),
	)

	err := sm.SaveRuntimeSession(
		context.Background(), model.RuntimeSession{},
	)
	if err != nil {
		t.Fatalf("SaveRuntimeSession() returned error: %v", err)
	}

	_, err = client.CoreV1().ConfigMaps("kwatch").Get(
		context.Background(), "kwatch-state", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("expected kwatch-state ConfigMap, got error: %v", err)
	}
}

func TestGetConfigMapFallsBackToLegacyName(t *testing.T) {
	client := fake.NewSimpleClientset()
	legacy := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kwatch-state",
			Namespace: "kwatch",
		},
		Data: map[string]string{clusterIDKey: "legacy-cluster"},
	}
	if _, err := client.CoreV1().ConfigMaps("kwatch").Create(
		context.Background(), legacy, metav1.CreateOptions{},
	); err != nil {
		t.Fatalf("failed to seed legacy ConfigMap: %v", err)
	}

	sm := NewManagerWithClock(
		client, "kwatch", clock.RealClock{}, WithStatePrefix("team"),
	)

	clusterID, err := sm.GetClusterID(context.Background())
	if err != nil {
		t.Fatalf("GetClusterID() returned error: %v", err)
	}
	if clusterID != "legacy-cluster" {
		t.Fatalf("expected fallback to legacy ConfigMap, got %q", clusterID)
	}

	if _, err := client.CoreV1().ConfigMaps("kwatch").Get(
		context.Background(), "team-state", metav1.GetOptions{},
	); !apierrors.IsNotFound(err) {
		t.Fatalf("expected no scoped ConfigMap to be created by a read")
	}
}
