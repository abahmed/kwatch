package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// PVC-usage persistence.

func (s *Manager) GetPvcUsage(ctx context.Context) map[string]model.PVCSample {
	usage, err := s.GetPvcUsageWithError(ctx)
	if err != nil {
		return nil
	}
	return usage
}

// GetPvcUsageWithError distinguishes an empty first run from an unavailable
// or corrupt persisted snapshot. The PVC runtime uses this boundary to avoid
// silently starting with incomplete hysteresis state.
func (s *Manager) GetPvcUsageWithError(
	ctx context.Context,
) (map[string]model.PVCSample, error) {
	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		pvcConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if gz, ok := cm.BinaryData[pvcUsageKey]; ok && len(gz) > 0 {
		var result map[string]model.PVCSample
		if err := gunzipJSON(gz, &result); err != nil {
			return nil, fmt.Errorf("decode pvc usage: %w", err)
		}
		return result, nil
	}
	raw, ok := cm.Data[pvcUsageKey]
	if !ok || raw == "" {
		return nil, nil
	}
	var result map[string]model.PVCSample
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("decode pvc usage: %w", err)
	}
	return result, nil
}

func (s *Manager) SavePvcUsage(
	ctx context.Context,
	usage map[string]model.PVCSample,
) error {
	return s.pvcMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		data, err := gzJSON(usage)
		if err != nil {
			return err
		}
		if len(data) > configMapPayloadMaxBytes {
			klog.ErrorS(nil, "pvc usage too large for ConfigMap, skipping save",
				"size", len(data), "max", configMapPayloadMaxBytes)
			return fmt.Errorf(
				"pvc usage %d bytes exceeds ConfigMap budget %d",
				len(data),
				configMapPayloadMaxBytes,
			)
		}
		setBinaryPayload(cm, pvcUsageKey, data)
		if err := validateConfigMapData(cm); err != nil {
			return fmt.Errorf("pvc usage payload: %w", err)
		}
		return nil
	})
}
