package state

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/insight"
)

const telemetryStateKey = "kubelet-telemetry"

const changeHistoryStateKey = "change-history"

const maxChangeHistoryBytes = 64 * 1024

const maxRCAFeedbackBytes = 64 * 1024

const rcaFeedbackKey = "records"

func (s *StateManager) LoadRCAFeedback(ctx context.Context) ([]insight.RCARecord, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, rcaConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		cm, err = s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
	}
	var records []insight.RCARecord
	if raw := cm.Data[rcaFeedbackKey]; raw != "" {
		err = json.Unmarshal([]byte(raw), &records)
	}
	return records, err
}

func (s *StateManager) SaveRCAFeedback(ctx context.Context, records []insight.RCARecord) error {
	data, err := json.Marshal(records)
	if err != nil {
		return err
	}
	for len(records) > 1 && len(data) > maxRCAFeedbackBytes {
		records = records[len(records)/2:]
		data, err = json.Marshal(records)
		if err != nil {
			return err
		}
	}
	if len(data) > maxRCAFeedbackBytes {
		return fmt.Errorf("rca feedback exceeds %d bytes", maxRCAFeedbackBytes)
	}
	return s.rcaMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		cm.Data[rcaFeedbackKey] = string(data)
		return nil
	})
}

func (s *StateManager) LoadChangeHistory(ctx context.Context) ([]kwcontext.Change, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, changesConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		cm, err = s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
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
			return s.changesMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
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
