package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/change"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
)

const telemetryStateKey = "kubelet-telemetry"

const changeHistoryStateKey = "change-history"

const maxChangeHistoryBytes = 64 * 1024

const maxRCAFeedbackBytes = 64 * 1024

const (
	maxChangeDetailBytes = 4 * 1024
	maxChangeFields      = 20
	maxChangeValueBytes  = 512
)

const rcaFeedbackKey = "records"

func (s *Manager) LoadRCAFeedback(
	ctx context.Context,
) ([]model.RCARecord, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(
		ctx, rcaConfigMapName, metav1.GetOptions{},
	)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		cm, err = s.client.CoreV1().ConfigMaps(s.namespace).Get(
			ctx, stateConfigMapName, metav1.GetOptions{},
		)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
	}
	var records []model.RCARecord
	if raw := cm.Data[rcaFeedbackKey]; raw != "" {
		err = json.Unmarshal([]byte(raw), &records)
	}
	return records, err
}

func (s *Manager) SaveRCAFeedback(
	ctx context.Context,
	records []model.RCARecord,
) error {
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
		setStringPayload(cm, rcaFeedbackKey, string(data))
		return nil
	})
}

func (s *Manager) LoadChangeHistory(
	ctx context.Context,
) ([]kwcontext.Change, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(
		ctx, changesConfigMapName, metav1.GetOptions{},
	)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		cm, err = s.client.CoreV1().ConfigMaps(s.namespace).Get(
			ctx, stateConfigMapName, metav1.GetOptions{},
		)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
	}
	if cm.Data[changeHistoryStateKey] == "" {
		return nil, nil
	}
	var changes []kwcontext.Change
	if err := json.Unmarshal(
		[]byte(cm.Data[changeHistoryStateKey]),
		&changes,
	); err != nil {
		return nil, err
	}
	return changes, nil
}

func (s *Manager) SaveChangeHistory(
	ctx context.Context,
	changes []kwcontext.Change,
) error {
	originalCount := len(changes)
	changes = compactChangeHistory(changes)
	if len(changes) != originalCount {
		metrics.DefaultRegistry().PersistenceCompactions.Add(1)
	}
	for len(changes) > 1 {
		data, err := json.Marshal(changes)
		if err != nil {
			return err
		}
		if len(data) <= maxChangeHistoryBytes {
			err = s.changesMgr.UpdateWithRetry(
				ctx,
				func(cm *corev1.ConfigMap) error {
					setStringPayload(cm, changeHistoryStateKey, string(data))
					return nil
				},
			)
			if err == nil {
				s.recordPersistenceSuccess(len(data))
			}
			return err
		}
		// Keep the newest half; history is context, not incident state, and
		// must never block updates to the state ConfigMap.
		removed := len(changes) / 2
		metrics.DefaultRegistry().PersistenceOmitted.Add(int64(removed))
		changes = changes[removed:]
	}
	if len(changes) == 1 {
		data, err := json.Marshal(changes)
		if err != nil {
			return err
		}
		if len(data) > maxChangeHistoryBytes {
			return fmt.Errorf(
				"compacted change history exceeds %d bytes",
				maxChangeHistoryBytes,
			)
		}
		err = s.changesMgr.UpdateWithRetry(
			ctx,
			func(cm *corev1.ConfigMap) error {
				setStringPayload(cm, changeHistoryStateKey, string(data))
				return nil
			},
		)
		if err == nil {
			s.recordPersistenceSuccess(len(data))
		}
		return err
	}
	return nil
}

// compactChangeHistory bounds user-controlled Kubernetes values before the
// snapshot is encoded. A single giant object must never make the required
// history saver fail and take the active leader down.
func compactChangeHistory(changes []kwcontext.Change) []kwcontext.Change {
	out := make([]kwcontext.Change, len(changes))
	for i, current := range changes {
		current.Detail = compactText(current.Detail, maxChangeDetailBytes)
		if len(current.Fields) > maxChangeFields {
			omitted := len(current.Fields) - maxChangeFields
			current.Additional += omitted
			metrics.DefaultRegistry().PersistenceOmitted.Add(int64(omitted))
			current.Fields = current.Fields[:maxChangeFields]
		}
		fields := make([]change.FieldChange, len(current.Fields))
		for j, field := range current.Fields {
			before, after := field.Before, field.After
			field.Before = compactText(field.Before, maxChangeValueBytes)
			field.After = compactText(field.After, maxChangeValueBytes)
			if before != field.Before || after != field.After {
				metrics.DefaultRegistry().PersistenceCompactions.Add(1)
			}
			fields[j] = field
		}
		current.Fields = fields
		out[i] = current
	}
	return out
}

func compactText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	if limit <= len("…") {
		return "…"
	}
	return value[:limit-len("…")] + "…"
}

func (s *Manager) recordPersistenceSuccess(size int) {
	registry := metrics.DefaultRegistry()
	registry.PersistencePayloadBytes.Store(int64(size))
	registry.PersistenceLastSuccess.Store(s.now().Unix())
}

// maxTelemetryStateBytes bounds the kubelet telemetry snapshot. It is
// advisory state -- losing it costs one interval of re-learned baselines --
// so an oversized payload is dropped rather than failing the save.
const maxTelemetryStateBytes = 512 * 1024

// LoadTelemetryState reads the kubelet telemetry snapshot, falling back to
// the shared state ConfigMap where earlier releases wrote it.
func (s *Manager) LoadTelemetryState(ctx context.Context) ([]byte, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(
		ctx, telemetryConfigMapName, metav1.GetOptions{},
	)
	if err == nil {
		// An existing dedicated ConfigMap is authoritative. In particular, an
		// empty value must not resurrect stale telemetry from kwatch-state.
		return []byte(cm.Data[telemetryStateKey]), nil
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}
	legacy, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(
		ctx, stateConfigMapName, metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return []byte(legacy.Data[telemetryStateKey]), nil
}

func (s *Manager) SaveTelemetryState(
	ctx context.Context,
	data []byte,
) error {
	if len(data) > maxTelemetryStateBytes {
		klog.ErrorS(nil,
			"kubelet telemetry state too large for ConfigMap, skipping",
			"size", len(data), "max", maxTelemetryStateBytes)
		return nil
	}
	return s.telemetryMgr.UpdateWithRetry(
		ctx,
		func(cm *corev1.ConfigMap) error {
			setStringPayload(cm, telemetryStateKey, string(data))
			return nil
		},
	)
}
