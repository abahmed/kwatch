package state

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kwcontext "github.com/abahmed/kwatch/internal/context"
)

const telemetryStateKey = "kubelet-telemetry"

const changeHistoryStateKey = "change-history"

const maxChangeHistoryBytes = 64 * 1024

func (s *StateManager) LoadChangeHistory(ctx context.Context) ([]kwcontext.Change, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	var changes []kwcontext.Change
	if err := json.Unmarshal([]byte(cm.Data[changeHistoryStateKey]), &changes); err != nil {
		return nil, err
	}
	return changes, nil
}

func (s *StateManager) SaveChangeHistory(ctx context.Context, changes []kwcontext.Change) error {
	for len(changes) > 0 {
		data, err := json.Marshal(changes)
		if err != nil {
			return err
		}
		if len(data) <= maxChangeHistoryBytes {
			return s.stateMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
				if cm.Data == nil {
					cm.Data = make(map[string]string)
				}
				cm.Data[changeHistoryStateKey] = string(data)
				return nil
			})
		}
		// Keep the newest half; history is context, not incident state, and
		// must never block updates to the state ConfigMap.
		changes = changes[len(changes)/2:]
	}
	return fmt.Errorf("change history exceeds %d bytes", maxChangeHistoryBytes)
}

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
