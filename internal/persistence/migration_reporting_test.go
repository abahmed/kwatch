package persistence

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestMarkAsInitializedReportsFutureStateSchema(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: stateConfigMapName, Namespace: "kwatch",
		},
		Data: map[string]string{
			stateSchemaVersionKey: "99",
		},
	})
	store := newTestManager(client, "kwatch")

	require.Error(t, store.MarkAsInitialized(
		context.Background(), "cluster", "version",
	))

	report := store.MigrationReport()
	result := report.Operations[len(report.Operations)-1]
	require.Equal(t, "state", result.Store)
	require.Equal(t, MigrationUnsupported, result.Status)
	require.True(t, result.Recoverable)
	require.False(t, result.MonitoringMayContinue)
	require.Equal(t, "kwatch-state/schema-v2", result.DestinationFormat)

	cm, err := client.CoreV1().ConfigMaps("kwatch").Get(
		context.Background(), stateConfigMapName, metav1.GetOptions{},
	)
	require.NoError(t, err)
	require.Equal(t, "99", cm.Data[stateSchemaVersionKey])
}

func TestMigrationResultRecordsBaselineFailure(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: stateConfigMapName, Namespace: "kwatch",
		},
		Data: map[string]string{baselineKey: "invalid"},
	})
	store := newTestManager(client, "kwatch")

	_, err := store.MigrateLegacyBaselineWithResult(context.Background())
	require.Error(t, err)
	report := store.MigrationReport()
	result := report.Operations[len(report.Operations)-1]
	require.Equal(t, MigrationFailed, result.Status)
	require.Equal(t, "baseline", result.Store)
}

func TestMarkAsInitializedReportsMalformedSchemaWithoutOverwriting(
	t *testing.T,
) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: stateConfigMapName, Namespace: "kwatch",
		},
		Data: map[string]string{
			stateSchemaVersionKey: "not-a-version",
		},
	})
	store := newTestManager(client, "kwatch")

	require.Error(t, store.MarkAsInitialized(
		context.Background(), "cluster", "version",
	))

	report := store.MigrationReport()
	result := report.Operations[len(report.Operations)-1]
	require.Equal(t, MigrationFailed, result.Status)
	require.Equal(t, "state schema version is malformed", result.Detail)
	require.NotContains(t, result.Detail, "not-a-version")

	cm, err := client.CoreV1().ConfigMaps("kwatch").Get(
		context.Background(), stateConfigMapName, metav1.GetOptions{},
	)
	require.NoError(t, err)
	require.Equal(t, "not-a-version", cm.Data[stateSchemaVersionKey])
}

func TestMigrationReportContainsAllStartupOperations(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: stateConfigMapName, Namespace: "kwatch",
		},
		Data: map[string]string{stateSchemaVersionKey: "1"},
	})
	store := newTestManager(client, "kwatch")

	require.NoError(t, store.MarkAsInitialized(
		context.Background(), "cluster", "version",
	))
	_, err := store.MigrateLegacyBaselineWithResult(context.Background())
	require.NoError(t, err)

	report := store.MigrationReport()
	require.Len(t, report.Operations, 2)
	require.Equal(t, "state", report.Operations[0].Store)
	require.Equal(t, "baseline", report.Operations[1].Store)
	require.Equal(t, "kwatch-state", report.Operations[0].SourceFormat)
	require.Equal(
		t, "kwatch-state/baseline", report.Operations[1].SourceFormat,
	)
	require.False(t, report.StartedAt.IsZero())
	require.False(t, report.CompletedAt.IsZero())

	report.Operations[0].Detail = "mutated"
	require.NotEqual(
		t, "mutated", store.MigrationReport().Operations[0].Detail,
	)
}

func TestMigrationReportPreservesCompleteStartupCycle(t *testing.T) {
	store := newTestManager(fake.NewSimpleClientset(), "kwatch")
	store.BeginMigrationReport()
	for _, name := range []string{
		"startup-metadata", "state-schema", "baseline", "incidents",
		"groups", "threads", "engine", "pvc-state", "telemetry",
		"feedback", "change-history",
	} {
		store.RecordMigrationResult(MigrationResult{
			Store:                 name,
			SourceFormat:          "source/" + name,
			DestinationFormat:     "runtime/" + name,
			Status:                MigrationCompleted,
			Recoverable:           true,
			MonitoringMayContinue: true,
			Detail:                "restored",
		}, nil)
	}

	report := store.MigrationReport()
	if len(report.Operations) != 11 {
		t.Fatalf("migration operations = %d, want 11", len(report.Operations))
	}
	if report.StartedAt.IsZero() || report.CompletedAt.IsZero() {
		t.Fatal("complete migration report must have lifecycle timestamps")
	}
	report.Operations[0].Store = "mutated"
	if store.MigrationReport().Operations[0].Store == "mutated" {
		t.Fatal("migration report operations must be detached")
	}
}
