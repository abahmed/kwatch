package state

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// ── Legacy baseline migration ─────────────────────────────────

// MigrateLegacyBaseline moves baseline data from kwatch-state.data[baseline]
// to the dedicated kwatch-baseline ConfigMap, then clears the legacy key.
// Idempotent — safe to call every startup.
func (s *StateManager) MigrateLegacyBaseline(ctx context.Context) {
	// Already in the dedicated CM? nothing to migrate.
	if cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		baselineConfigMapName,
		metav1.GetOptions{},
	); err == nil {
		if len(cm.BinaryData[baselineKey]) > 0 || cm.Data[baselineKey] != "" {
			s.clearLegacyBaseline(ctx)
			return
		}
	}
	old, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		stateConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		return
	}
	raw, ok := old.Data[baselineKey]
	if !ok || raw == "" {
		return
	}
	var b map[string]map[string]int64
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		klog.ErrorS(err, "migrate: bad legacy baseline json")
		return
	}
	if err := s.SaveBaseline(ctx, b); err != nil {
		klog.ErrorS(err, "migrate: save baseline to dedicated CM")
		return
	}
	s.clearLegacyBaseline(ctx)
}

func (s *StateManager) clearLegacyBaseline(ctx context.Context) {
	if err := s.stateMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		delete(cm.Data, baselineKey)
		return nil
	}); err != nil {
		klog.ErrorS(err, "migrate: clear legacy baseline from kwatch-state")
	}
}
