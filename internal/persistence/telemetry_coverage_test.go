package persistence

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestLastSeenAndTelemetryStateRoundTrip(t *testing.T) {
	client := fake.NewSimpleClientset()
	manager := newTestManager(client, "kwatch")
	want := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	if err := manager.SetLastSeen(context.Background(), want); err != nil {
		t.Fatalf("SetLastSeen() error = %v", err)
	}
	got, err := manager.GetLastSeen(context.Background())
	if err != nil || !got.Equal(want) {
		t.Fatalf("GetLastSeen() = %v, %v", got, err)
	}
	if err := manager.SetTelemetryLastSent(
		context.Background(), want,
	); err != nil {
		t.Fatalf("SetTelemetryLastSent() error = %v", err)
	}
	got, err = manager.GetTelemetryLastSent(context.Background())
	if err != nil || !got.Equal(want) {
		t.Fatalf("GetTelemetryLastSent() = %v, %v", got, err)
	}
	if err := manager.SaveTelemetryState(
		context.Background(), []byte("state"),
	); err != nil {
		t.Fatalf("SaveTelemetryState() error = %v", err)
	}
	state, err := manager.LoadTelemetryState(context.Background())
	if err != nil || string(state) != "state" {
		t.Fatalf("LoadTelemetryState() = %q, %v", state, err)
	}
}

func TestPersistenceMissingAndMalformedTimeState(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: stateConfigMapName, Namespace: "kwatch",
		},
		Data: map[string]string{
			lastSeenKey:          "not-a-time",
			telemetryLastSentKey: "not-a-time",
		},
	})
	manager := newTestManager(client, "kwatch")
	if _, err := manager.GetLastSeen(context.Background()); err == nil {
		t.Fatal("malformed last-seen state succeeded")
	}
	if _, err := manager.GetTelemetryLastSent(context.Background()); err == nil {
		t.Fatal("malformed telemetry state succeeded")
	}
	if _, err := manager.StatusJSON(); err != nil {
		t.Fatalf("StatusJSON() error = %v", err)
	}
}

func TestLoadTelemetryStateFallsBackToLegacyState(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: stateConfigMapName, Namespace: "kwatch",
		},
		Data: map[string]string{telemetryStateKey: "legacy"},
	})
	manager := newTestManager(client, "kwatch")
	got, err := manager.LoadTelemetryState(context.Background())
	if err != nil || string(got) != "legacy" {
		t.Fatalf("LoadTelemetryState() = %q, %v", got, err)
	}
}
