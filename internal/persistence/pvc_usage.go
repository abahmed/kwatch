package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// PVC-usage persistence.

func (s *Manager) GetPvcUsage(ctx context.Context) map[string]model.PVCSample {
	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		pvcConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		return nil
	}
	if gz, ok := cm.BinaryData[pvcUsageKey]; ok && len(gz) > 0 {
		var result map[string]model.PVCSample
		if err := gunzipJSON(gz, &result); err != nil {
			klog.ErrorS(err, "failed to gunzip pvc usage")
			return nil
		}
		return result
	}
	raw, ok := cm.Data[pvcUsageKey]
	if !ok || raw == "" {
		return nil
	}
	var result map[string]model.PVCSample
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		klog.ErrorS(err, "failed to unmarshal pvc usage")
		return nil
	}
	return result
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
