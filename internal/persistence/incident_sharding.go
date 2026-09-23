package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/model"
)

type incidentShardManifest struct {
	Schema     string `json:"schema"`
	ShardCount int    `json:"shardCount"`
	Generation string `json:"generation"`
	Checksum   string `json:"checksum"`
}

func (s *Manager) saveIncidentShards(
	ctx context.Context,
	incidents []model.PersistedIncident,
	cleanLegacy bool,
) error {
	shards, err := splitIncidentShards(incidents)
	if err != nil {
		return err
	}
	dataByShard := make([][]byte, len(shards))
	hash := sha256.New()
	for i, shard := range shards {
		data, marshalErr := gzJSON(shard)
		if marshalErr != nil {
			return marshalErr
		}
		hash.Write(data)
		dataByShard[i] = data
	}
	checksum := hex.EncodeToString(hash.Sum(nil))
	generation := checksum[:16]
	for i, data := range dataByShard {
		name := incidentShardName(generation, i)
		mgr := NewRetryConfigMapManager(s.client, s.namespace, name)
		if err := mgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
			setBinaryPayload(cm, incidentsKey, data)
			return nil
		}); err != nil {
			return fmt.Errorf("write incident shard %s: %w", name, err)
		}
	}
	manifest := incidentShardManifest{
		Schema: currentIncidentSchema, ShardCount: len(shards),
		Generation: checksum[:16], Checksum: checksum,
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := s.incidentsMgr.UpdateWithRetry(
		ctx, func(cm *corev1.ConfigMap) error {
			deletePayload(cm, incidentsKey)
			setStringPayload(cm, incidentManifestKey, string(manifestData))
			if cleanLegacy {
				deletePayload(cm, groupsKey)
				deletePayload(cm, threadsKey)
				deletePayload(cm, engineKey)
			}
			return nil
		},
	); err != nil {
		return err
	}
	return s.garbageCollectIncidentShards(ctx, generation)
}

func incidentShardName(generation string, index int) string {
	return fmt.Sprintf("%s%s-%03d", incidentShardPrefix, generation, index)
}

func (s *Manager) garbageCollectIncidentShards(
	ctx context.Context, generation string,
) error {
	list, err := s.client.CoreV1().ConfigMaps(s.namespace).List(
		ctx, metav1.ListOptions{},
	)
	if err != nil {
		return err
	}
	for _, cm := range list.Items {
		if !strings.HasPrefix(cm.Name, incidentShardPrefix) {
			continue
		}
		if generation != "" && strings.HasPrefix(
			cm.Name, incidentShardPrefix+generation+"-",
		) {
			continue
		}
		if err := s.client.CoreV1().ConfigMaps(s.namespace).Delete(
			ctx, cm.Name, metav1.DeleteOptions{},
		); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func splitIncidentShards(
	incidents []model.PersistedIncident,
) ([][]model.PersistedIncident, error) {
	var shards [][]model.PersistedIncident
	for start := 0; start < len(incidents); {
		low, high := start+1, len(incidents)
		best := 0
		for low <= high {
			middle := low + (high-low)/2
			data, err := gzJSON(incidents[start:middle])
			if err != nil {
				return nil, err
			}
			if len(data) <= configMapPayloadMaxBytes {
				best = middle
				low = middle + 1
				continue
			}
			high = middle - 1
		}
		if best == 0 {
			return nil, fmt.Errorf("incident cannot fit in a persistence shard")
		}
		shard := append([]model.PersistedIncident(nil), incidents[start:best]...)
		shards = append(shards, shard)
		start = best
	}
	return shards, nil
}

func (s *Manager) loadIncidentShards(
	ctx context.Context,
) ([]model.PersistedIncident, bool, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(
		ctx, incidentsConfigMapName, metav1.GetOptions{},
	)
	if apierrors.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	raw := cm.Data[incidentManifestKey]
	if raw == "" {
		return nil, false, nil
	}
	var manifest incidentShardManifest
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		return nil, true, fmt.Errorf("decode incident shard manifest: %w", err)
	}
	if manifest.ShardCount <= 0 || manifest.ShardCount > 1000 {
		return nil, true, fmt.Errorf("invalid incident shard count")
	}
	hash := sha256.New()
	var incidents []model.PersistedIncident
	for i := 0; i < manifest.ShardCount; i++ {
		name := manifestShardName(manifest, i)
		shard, getErr := s.client.CoreV1().ConfigMaps(s.namespace).Get(
			ctx, name, metav1.GetOptions{},
		)
		if getErr != nil {
			return nil, true, fmt.Errorf("load incident shard %s: %w", name, getErr)
		}
		data := shard.BinaryData[incidentsKey]
		if len(data) == 0 {
			return nil, true, fmt.Errorf("incident shard %s is empty", name)
		}
		hash.Write(data)
		var part []model.PersistedIncident
		if err := gunzipJSON(data, &part); err != nil {
			return nil, true, fmt.Errorf("decode incident shard %s: %w", name, err)
		}
		incidents = append(incidents, part...)
	}
	if hex.EncodeToString(hash.Sum(nil)) != manifest.Checksum {
		return nil, true, fmt.Errorf("incident shard checksum mismatch")
	}
	return incidents, true, nil
}

func manifestShardName(manifest incidentShardManifest, index int) string {
	if manifest.Schema == currentIncidentSchema {
		return incidentShardName(manifest.Generation, index)
	}
	return fmt.Sprintf("%s%03d", incidentShardPrefix, index)
}
