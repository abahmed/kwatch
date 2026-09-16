package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MigrationStatus describes the outcome of a persistence migration.
type MigrationStatus string

const (
	MigrationNotRequired MigrationStatus = "not_required"
	MigrationCompleted   MigrationStatus = "completed"
	MigrationFailed      MigrationStatus = "failed"
	MigrationUnsupported MigrationStatus = "unsupported"
)

// MigrationResult records an operator-useful migration outcome without
// exposing persistence implementation details to startup code.
type MigrationResult struct {
	Store                 string          `json:"store"`
	SourceFormat          string          `json:"sourceFormat"`
	DestinationFormat     string          `json:"destinationFormat"`
	Status                MigrationStatus `json:"status"`
	Recoverable           bool            `json:"recoverable"`
	MonitoringMayContinue bool            `json:"monitoringMayContinue"`
	Detail                string          `json:"detail"`
}

// MigrationReport groups every migration operation observed during one
// startup cycle. Operations are copied when the report crosses the manager
// boundary so diagnostics cannot mutate persistence state.
type MigrationReport struct {
	Operations  []MigrationResult `json:"operations"`
	StartedAt   time.Time         `json:"startedAt"`
	CompletedAt time.Time         `json:"completedAt"`
}

// MigrateLegacyBaselineWithResult performs the legacy baseline migration and
// returns a structured result for startup diagnostics and health reporting.
func (s *Manager) MigrateLegacyBaselineWithResult(
	ctx context.Context,
) (MigrationResult, error) {
	result, err := s.migrateLegacyBaselineWithResult(ctx)
	s.recordMigrationResult(result, err)
	return result, err
}

func (s *Manager) migrateLegacyBaselineWithResult(
	ctx context.Context,
) (MigrationResult, error) {
	result := MigrationResult{
		Store:                 "baseline",
		SourceFormat:          "kwatch-state/baseline",
		DestinationFormat:     "kwatch-baseline/baseline",
		Status:                MigrationNotRequired,
		Recoverable:           true,
		MonitoringMayContinue: true,
		Detail:                "legacy baseline is not present",
	}

	// Already in the dedicated CM? nothing to migrate.
	if cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		baselineConfigMapName,
		metav1.GetOptions{},
	); err == nil {
		if len(cm.BinaryData[baselineKey]) > 0 || cm.Data[baselineKey] != "" {
			err := s.clearLegacyBaseline(ctx)
			if err != nil {
				return result, err
			}
			result.Status = MigrationCompleted
			result.Detail = "cleared already migrated legacy baseline"
			return result, nil
		}
	} else if !apierrors.IsNotFound(err) {
		return result, fmt.Errorf(
			"read dedicated baseline configmap: %w", err,
		)
	}
	old, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		stateConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return result, nil
		}
		return result, fmt.Errorf("read legacy state configmap: %w", err)
	}
	raw, ok := old.Data[baselineKey]
	if !ok || raw == "" {
		return result, nil
	}
	var b map[string]map[string]int64
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		result.Recoverable = false
		result.MonitoringMayContinue = true
		result.Detail = "legacy baseline payload is corrupt"
		return result, fmt.Errorf("decode legacy baseline: %w", err)
	}
	if err := s.SaveBaseline(ctx, b); err != nil {
		return result, fmt.Errorf("save migrated baseline: %w", err)
	}
	if err := s.clearLegacyBaseline(ctx); err != nil {
		return result, err
	}
	result.Status = MigrationCompleted
	result.Detail = "moved legacy baseline to dedicated storage"
	return result, nil
}

func (s *Manager) clearLegacyBaseline(ctx context.Context) error {
	if err := s.configMapStore.UpdateWithRetry(
		ctx,
		func(cm *corev1.ConfigMap) error {
			delete(cm.Data, baselineKey)
			return nil
		},
	); err != nil {
		return fmt.Errorf("clear legacy baseline: %w", err)
	}
	return nil
}
