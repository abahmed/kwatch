package state

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// ── Baseline persistence ──────────────────────────────────────

// 1,048,576 — K8s ConfigMap data hard cap (MaxSecretSize)
const configMapDataLimit = 1 << 20

// ~1,032,192; 16 KiB reserve for safety
const baselineMaxBytes = configMapDataLimit - 16*1024

func (s *StateManager) GetBaseline(
	ctx context.Context,
) map[string]map[string]int64 {
	var result map[string]map[string]int64

	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		baselineConfigMapName,
		metav1.GetOptions{},
	)
	if err == nil {
		if gz, ok := cm.BinaryData[baselineKey]; ok && len(gz) > 0 {
			if err := gunzipJSON(gz, &result); err != nil {
				klog.ErrorS(err, "failed to gunzip baseline")
				return nil
			}
			return result
		}
		if raw, ok := cm.Data[baselineKey]; ok && raw != "" {
			if err := json.Unmarshal([]byte(raw), &result); err != nil {
				klog.ErrorS(err, "failed to unmarshal baseline")
				return nil
			}
			return result
		}
	}

	// migration: fall back to the pre-split location
	// kwatch-state.data[baseline]
	if old, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		stateConfigMapName,
		metav1.GetOptions{},
	); err == nil {
		if raw, ok := old.Data[baselineKey]; ok && raw != "" {
			if err := json.Unmarshal([]byte(raw), &result); err != nil {
				klog.ErrorS(err, "failed to unmarshal legacy baseline")
				return nil
			}
			return result
		}
	}

	return nil
}

func (s *StateManager) SaveBaseline(
	ctx context.Context,
	baseline map[string]map[string]int64,
) error {
	return s.baselineMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		data, err := gzJSON(baseline)
		if err != nil {
			return err
		}
		if len(data) > baselineMaxBytes {
			klog.ErrorS(nil, "baseline too large even gzipped, skipping save",
				"size", len(data), "max", baselineMaxBytes)
			return fmt.Errorf(
				"baseline %d gz-bytes exceeds budget %d",
				len(data),
				baselineMaxBytes,
			)
		}
		if cm.BinaryData == nil {
			cm.BinaryData = map[string][]byte{}
		}
		cm.BinaryData[baselineKey] = data
		delete(cm.Data, baselineKey)
		return nil
	})
}
