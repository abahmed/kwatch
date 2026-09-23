package persistence

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/change"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"

	"github.com/abahmed/kwatch/internal/model"
)

func TestConfigMapDataSizeCountsKeysAndValues(t *testing.T) {
	cm := &corev1.ConfigMap{
		Data: map[string]string{"a": "bc"},
		BinaryData: map[string][]byte{
			"de": []byte("fgh"),
		},
	}

	assert.Equal(t, int64(8), configMapDataSize(cm))
}

func TestSaveChangeHistoryCompactsOneOversizedEntry(t *testing.T) {
	manager := newTestManager(
		fake.NewSimpleClientset(), "kwatch",
	)
	changes := []kwcontext.Change{{Detail: strings.Repeat("x", 70*1024)}}

	start := time.Now()
	require.NoError(t, manager.SaveChangeHistory(context.Background(), changes))
	assert.Less(t, time.Since(start), time.Second)
	loaded, err := manager.LoadChangeHistory(context.Background())
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.LessOrEqual(t, len(loaded[0].Detail), maxChangeDetailBytes)
	assert.True(t, strings.HasSuffix(loaded[0].Detail, "…"))
}

func TestCompactChangeHistoryPreservesIdentityAndCountsOmittedFields(
	t *testing.T,
) {
	fields := make([]change.FieldChange, maxChangeFields+2)
	for i := range fields {
		fields[i] = change.FieldChange{
			Path: "spec.value", Before: strings.Repeat("b", 600),
			After: strings.Repeat("a", 600), Action: "updated",
		}
	}
	input := []kwcontext.Change{{
		Resource: "Deployment", Namespace: "prod", Name: "api",
		Type: kwcontext.ChangeUpdate, Fields: fields,
	}}

	got := compactChangeHistory(input)
	require.Len(t, got, 1)
	assert.Equal(t, "Deployment", got[0].Resource)
	assert.Equal(t, "prod", got[0].Namespace)
	assert.Equal(t, "api", got[0].Name)
	assert.Equal(t, 2, got[0].Additional)
	require.Len(t, got[0].Fields, maxChangeFields)
	assert.Len(t, got[0].Fields[0].Before, maxChangeValueBytes)
	assert.True(t, strings.HasSuffix(got[0].Fields[0].Before, "…"))
}

func TestSaveChangeHistoryEmptySnapshotIsNoOp(t *testing.T) {
	manager := newTestManager(
		fake.NewSimpleClientset(), "kwatch",
	)

	require.NoError(t, manager.SaveChangeHistory(
		context.Background(), nil,
	))
}

func TestSaveChangeHistoryKeepsNewestEntriesWhenTrimming(t *testing.T) {
	manager := newTestManager(
		fake.NewSimpleClientset(), "kwatch",
	)
	changes := []kwcontext.Change{
		{Detail: strings.Repeat("old", 20*1024)},
		{Detail: strings.Repeat("middle", 12*1024)},
		{Detail: "newest"},
	}

	require.NoError(t, manager.SaveChangeHistory(
		context.Background(), changes,
	))
	loaded, err := manager.LoadChangeHistory(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, loaded)
	assert.Equal(t, "newest", loaded[len(loaded)-1].Detail)
}

func TestPayloadHelpersReplaceBothConfigMapFields(t *testing.T) {
	cm := &corev1.ConfigMap{
		Data: map[string]string{"payload": "old"},
		BinaryData: map[string][]byte{
			"other":   []byte("keep"),
			"payload": []byte("stale"),
		},
	}

	setBinaryPayload(cm, "payload", []byte("new"))
	assert.Empty(t, cm.Data["payload"])
	assert.Equal(t, []byte("new"), cm.BinaryData["payload"])

	setStringPayload(cm, "payload", "text")
	assert.Equal(t, "text", cm.Data["payload"])
	assert.Empty(t, cm.BinaryData["payload"])

	deletePayload(cm, "payload")
	assert.Empty(t, cm.Data["payload"])
	assert.Empty(t, cm.BinaryData["payload"])
}

func TestRetryConfigMapManagerRejectsAggregateDataOverBudget(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "state",
			Namespace: "kwatch",
		},
		Data: map[string]string{
			"existing": strings.Repeat("x", configMapPayloadMaxBytes-8),
		},
	})
	mgr := NewRetryConfigMapManager(client, "kwatch", "state")

	err := mgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		setStringPayload(cm, "new", "value")
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds budget")
	stored, getErr := client.CoreV1().ConfigMaps("kwatch").Get(
		ctx, "state", metav1.GetOptions{},
	)
	require.NoError(t, getErr)
	assert.NotContains(t, stored.Data, "new")
}

func TestDedicatedEmptyPayloadDoesNotFallBackToLegacy(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kwatch-baseline", Namespace: "kwatch",
			},
			Data: map[string]string{},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kwatch-telemetry", Namespace: "kwatch",
			},
			Data: map[string]string{telemetryStateKey: ""},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kwatch-state", Namespace: "kwatch",
			},
			Data: map[string]string{
				baselineKey:       `{"old":"baseline"}`,
				telemetryStateKey: "old telemetry",
			},
		},
	)
	sm := newTestManager(client, "kwatch")

	assert.Nil(t, sm.GetBaseline(ctx))
	telemetry, err := sm.LoadTelemetryState(ctx)
	require.NoError(t, err)
	assert.Empty(t, telemetry)
}

func TestIncidentRetentionKeepsActiveBeforeResolved(t *testing.T) {
	now := time.Now().UTC()
	seed := uint64(0x9E3779B97F4A7C15)
	blob := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			seed ^= seed << 13
			seed ^= seed >> 7
			seed ^= seed << 17
			b[i] = byte('a' + seed%26)
		}
		return string(b)
	}

	const activeCount = 20
	incidents := make([]model.PersistedIncident, 0, 4020)
	for i := 0; i < activeCount; i++ {
		incidents = append(incidents, model.PersistedIncident{
			Key:      model.IncidentKey(fmt.Sprintf("active-%04d", i)),
			Name:     blob(600),
			LastSeen: now.Add(-48 * time.Hour).Add(-time.Duration(i) * time.Minute),
			State:    model.StateActive,
		})
	}
	for i := 0; i < 4000; i++ {
		incidents = append(incidents, model.PersistedIncident{
			Key:      model.IncidentKey(fmt.Sprintf("resolved-%04d", i)),
			Name:     blob(600),
			LastSeen: now.Add(-time.Duration(i) * time.Minute),
			State:    model.StateResolved,
		})
	}

	raw, err := gzJSON(incidents)
	require.NoError(t, err)
	require.Greater(t, len(raw), configMapPayloadMaxBytes)

	kept := trimIncidentsToBudget(incidents)
	require.NotEmpty(t, kept)
	for _, incident := range kept {
		if incident.State != model.StateActive {
			break
		}
		assert.Contains(t, incident.Key, "active-")
	}
	for i := 0; i < activeCount; i++ {
		key := model.IncidentKey(fmt.Sprintf("active-%04d", i))
		assert.True(t, containsIncident(kept, key), key)
	}
}

func containsIncident(
	incidents []model.PersistedIncident,
	key model.IncidentKey,
) bool {
	for _, incident := range incidents {
		if incident.Key == key {
			return true
		}
	}
	return false
}
