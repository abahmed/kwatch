package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// Baseline persistence.

// 1,048,576 — K8s ConfigMap data hard cap (MaxSecretSize)
const configMapDataLimit = 1 << 20

// ~1,032,192; 16 KiB reserve for safety. All state ConfigMaps share this
// budget because the Kubernetes data limit applies to each serialized map.
const configMapPayloadMaxBytes = configMapDataLimit - 16*1024

func (s *Manager) GetBaseline(
	ctx context.Context,
) map[string]map[string]int64 {
	result, err := s.GetBaselineWithError(ctx)
	if err != nil {
		klog.ErrorS(err, "failed to load baseline")
	}
	return result
}

// GetBaselineWithError is the fail-closed baseline load used by application
// activation. A missing ConfigMap is a valid first-run state; API and decode
// errors must not be mistaken for an empty baseline.
func (s *Manager) GetBaselineWithError(
	ctx context.Context,
) (map[string]map[string]int64, error) {
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
				return nil, fmt.Errorf("decode baseline: %w", err)
			}
			return result, nil
		}
		// An existing dedicated ConfigMap is authoritative, including when
		// its payload was intentionally cleared after a size failure.
		if _, ok := cm.BinaryData[baselineKey]; ok {
			return nil, nil
		}
		if raw, ok := cm.Data[baselineKey]; ok && raw != "" {
			if err := json.Unmarshal([]byte(raw), &result); err != nil {
				return nil, fmt.Errorf("decode baseline: %w", err)
			}
			return result, nil
		}
		if _, ok := cm.Data[baselineKey]; ok {
			return nil, nil
		}
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("read baseline configmap: %w", err)
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
				return nil, fmt.Errorf("decode legacy baseline: %w", err)
			}
			return result, nil
		}
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("read legacy baseline: %w", err)
	}

	return nil, nil
}

func (s *Manager) SaveBaseline(
	ctx context.Context,
	baseline map[string]map[string]int64,
) error {
	return s.baselineMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		data, err := gzJSON(baseline)
		if err != nil {
			return err
		}
		if len(data) > configMapPayloadMaxBytes {
			klog.ErrorS(nil, "baseline too large even gzipped, skipping save",
				"size", len(data), "max", configMapPayloadMaxBytes)
			return fmt.Errorf(
				"baseline %d gz-bytes exceeds budget %d",
				len(data),
				configMapPayloadMaxBytes,
			)
		}
		setBinaryPayload(cm, baselineKey, data)
		if err := validateConfigMapData(cm); err != nil {
			return fmt.Errorf("baseline payload: %w", err)
		}
		return nil
	})
}
