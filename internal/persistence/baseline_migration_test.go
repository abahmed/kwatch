package persistence

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestMigrateLegacyBaselineMovesAndClearsPayload(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: stateConfigMapName, Namespace: "kwatch",
		},
		Data: map[string]string{
			baselineKey: `{"ns:deployment/api:Error:":{"pod":1}}`,
		},
	})
	store := newTestManager(client, "kwatch")

	result, err := store.MigrateLegacyBaselineWithResult(ctx)
	require.NoError(t, err)
	require.Equal(t, MigrationCompleted, result.Status)
	require.NotNil(t, store.GetBaseline(ctx))

	legacy, err := client.CoreV1().ConfigMaps("kwatch").Get(
		ctx, stateConfigMapName, metav1.GetOptions{},
	)
	require.NoError(t, err)
	_, exists := legacy.Data[baselineKey]
	require.False(t, exists, "legacy baseline should be cleared")
}

func TestMigrateLegacyBaselineReturnsCompletionResult(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: stateConfigMapName, Namespace: "kwatch",
		},
		Data: map[string]string{
			baselineKey: `{"ns:deployment/api:Error:":{"pod":1}}`,
		},
	})
	store := newTestManager(client, "kwatch")

	result, err := store.MigrateLegacyBaselineWithResult(
		context.Background(),
	)

	require.NoError(t, err)
	require.Equal(t, MigrationCompleted, result.Status)
	require.True(t, result.Recoverable)
	require.True(t, result.MonitoringMayContinue)
}

func TestMigrateLegacyBaselineReportsNotRequired(t *testing.T) {
	store := newTestManager(fake.NewSimpleClientset(), "kwatch")

	result, err := store.MigrateLegacyBaselineWithResult(
		context.Background(),
	)

	require.NoError(t, err)
	require.Equal(t, MigrationNotRequired, result.Status)
	require.True(t, result.MonitoringMayContinue)
}

func TestMigrateLegacyBaselineReportsCorruptPayload(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: stateConfigMapName, Namespace: "kwatch",
		},
		Data: map[string]string{baselineKey: "not-json"},
	})
	store := newTestManager(client, "kwatch")

	result, err := store.MigrateLegacyBaselineWithResult(context.Background())
	require.Error(t, err)
	require.False(t, result.Recoverable)
}
