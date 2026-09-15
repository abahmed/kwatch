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
	store := NewManager(client, "kwatch")

	require.NoError(t, store.MarkAsInitialized(
		context.Background(), "cluster", "version",
	))

	result := store.LastMigration()
	require.Equal(t, MigrationUnsupported, result.Status)
	require.True(t, result.Recoverable)
	require.True(t, result.MonitoringMayContinue)
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
	store := NewManager(client, "kwatch")

	_, err := store.MigrateLegacyBaselineWithResult(context.Background())
	require.Error(t, err)
	require.Equal(t, MigrationFailed, store.LastMigration().Status)
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
	store := NewManager(client, "kwatch")

	require.NoError(t, store.MarkAsInitialized(
		context.Background(), "cluster", "version",
	))

	result := store.LastMigration()
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
	store := NewManager(client, "kwatch")

	require.NoError(t, store.MarkAsInitialized(
		context.Background(), "cluster", "version",
	))
	_, err := store.MigrateLegacyBaselineWithResult(context.Background())
	require.NoError(t, err)

	report := store.MigrationReport()
	require.Len(t, report, 2)
	require.Equal(t, "kwatch-state", report[0].SourceFormat)
	require.Equal(t, "kwatch-state/baseline", report[1].SourceFormat)

	report[0].Detail = "mutated"
	require.NotEqual(t, "mutated", store.MigrationReport()[0].Detail)
}
