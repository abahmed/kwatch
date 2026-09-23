package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/model"
)

func TestPersistedIncidentsShardAndRestoreCompleteGeneration(t *testing.T) {
	seed := uint64(0x9e3779b97f4a7c15)
	blob := func(size int) string {
		out := make([]byte, size)
		for i := range out {
			seed ^= seed << 13
			seed ^= seed >> 7
			seed ^= seed << 17
			out[i] = byte(seed)
		}
		return string(out)
	}
	incidents := make([]model.PersistedIncident, 0, 1800)
	for i := 0; i < 1800; i++ {
		incidents = append(incidents, model.PersistedIncident{
			Key:   model.IncidentKey(fmt.Sprintf("ns:key-%04d", i)),
			Name:  blob(700),
			State: model.StateActive,
		})
	}
	manager := newTestManager(fake.NewSimpleClientset(), "kwatch")
	require.NoError(t, manager.SavePersistedIncidents(
		context.Background(), incidents,
	))

	base, err := manager.client.CoreV1().ConfigMaps("kwatch").Get(
		context.Background(), incidentsConfigMapName, metav1.GetOptions{},
	)
	require.NoError(t, err)
	require.NotEmpty(t, base.Data[incidentManifestKey])
	require.Empty(t, base.BinaryData[incidentsKey])

	loaded, err := manager.LoadPersistedIncidents(context.Background())
	require.NoError(t, err)
	require.Len(t, loaded, len(incidents))
	require.Equal(t, incidents[0].Key, loaded[0].Key)
	require.Equal(t, incidents[len(incidents)-1].Key,
		loaded[len(loaded)-1].Key)
}

func TestPersistedIncidentsKeepPreviousGenerationWhenShardWriteFails(
	t *testing.T,
) {
	client := fake.NewSimpleClientset()
	manager := newTestManager(client, "kwatch")
	previous := shardedTestIncidents("previous")
	require.NoError(t, manager.SavePersistedIncidents(
		context.Background(), previous,
	))

	shardWrites := 0
	failShardWrite := func(action k8stesting.Action) (
		bool, runtime.Object, error,
	) {
		object := action.(interface {
			GetObject() runtime.Object
		}).GetObject()
		cm, ok := object.(*corev1.ConfigMap)
		if !ok {
			return false, nil, nil
		}
		name := cm.Name
		if !strings.HasPrefix(name, incidentShardPrefix) {
			return false, nil, nil
		}
		shardWrites++
		if shardWrites == 2 {
			return true, nil, fmt.Errorf("injected shard write failure")
		}
		return false, nil, nil
	}
	client.PrependReactor("create", "configmaps", failShardWrite)
	client.PrependReactor("update", "configmaps", failShardWrite)

	require.Error(t, manager.SavePersistedIncidents(
		context.Background(), shardedTestIncidents("next"),
	))
	loaded, err := manager.LoadPersistedIncidents(context.Background())
	require.NoError(t, err)
	require.Len(t, loaded, len(previous))
	require.Equal(t, previous[0].Key, loaded[0].Key)
}

func TestPersistedIncidentsLoadSchemaThreeFixedShards(t *testing.T) {
	client := fake.NewSimpleClientset()
	manager := newTestManager(client, "kwatch")
	incidents := []model.PersistedIncident{{
		Key: "ns:legacy", Name: "legacy", State: model.StateActive,
	}}
	data, err := gzJSON(incidents)
	require.NoError(t, err)
	checksum := sha256.Sum256(data)
	manifest, err := json.Marshal(incidentShardManifest{
		Schema: "3", ShardCount: 1,
		Checksum: hex.EncodeToString(checksum[:]),
	})
	require.NoError(t, err)
	_, err = client.CoreV1().ConfigMaps("kwatch").Create(
		context.Background(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: incidentsConfigMapName},
			Data:       map[string]string{incidentManifestKey: string(manifest)},
		}, metav1.CreateOptions{},
	)
	require.NoError(t, err)
	_, err = client.CoreV1().ConfigMaps("kwatch").Create(
		context.Background(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: incidentShardPrefix + "000"},
			BinaryData: map[string][]byte{incidentsKey: data},
		}, metav1.CreateOptions{},
	)
	require.NoError(t, err)

	loaded, err := manager.LoadPersistedIncidents(context.Background())
	require.NoError(t, err)
	require.Equal(t, incidents, loaded)
}

func shardedTestIncidents(prefix string) []model.PersistedIncident {
	incidents := make([]model.PersistedIncident, 0, 1800)
	seed := uint64(0x9e3779b97f4a7c15)
	if prefix == "next" {
		seed++
	}
	for i := 0; i < 1800; i++ {
		blob := make([]byte, 700)
		for index := range blob {
			seed ^= seed << 13
			seed ^= seed >> 7
			seed ^= seed << 17
			blob[index] = byte(seed)
		}
		incidents = append(incidents, model.PersistedIncident{
			Key:   model.IncidentKey(fmt.Sprintf("ns:%s-%04d", prefix, i)),
			Name:  string(blob),
			State: model.StateActive,
		})
	}
	return incidents
}
