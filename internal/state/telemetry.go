package state

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const telemetryStateKey = "kubelet-telemetry"

func (s *StateManager) LoadTelemetryState(ctx context.Context) ([]byte, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return []byte(cm.Data[telemetryStateKey]), nil
}

func (s *StateManager) SaveTelemetryState(ctx context.Context, data []byte) error {
	return s.stateMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		cm.Data[telemetryStateKey] = string(data)
		return nil
	})
}
