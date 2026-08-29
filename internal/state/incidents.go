package state

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// ── Incident persistence ─────────────────────────────────────

func (s *StateManager) SaveIncidents(ctx context.Context, incidents any) error {
	return s.incidentsMgr.UpdateWithRetry(
		ctx,
		func(cm *corev1.ConfigMap) error {
			data, err := gzJSON(incidents)
			if err != nil {
				return err
			}
			if len(data) > baselineMaxBytes {
				klog.ErrorS(
					nil,
					"incidents too large for ConfigMap, skipping save",
					"size",
					len(data),
					"max",
					baselineMaxBytes,
				)
				return fmt.Errorf(
					"incidents %d bytes exceeds ConfigMap budget %d",
					len(data),
					baselineMaxBytes,
				)
			}
			if cm.BinaryData == nil {
				cm.BinaryData = map[string][]byte{}
			}
			cm.BinaryData[incidentsKey] = data
			return nil
		},
	)
}

func (s *StateManager) GetIncidents(ctx context.Context, out any) error {
	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		incidentsConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil // nothing saved yet
		}
		return err
	}
	if gz, ok := cm.BinaryData[incidentsKey]; ok && len(gz) > 0 {
		return gunzipJSON(gz, out)
	}
	return nil
}

// SavePersistedIncidents and LoadPersistedIncidents are the typed contract for
// incident persistence.
//
// SaveIncidents/GetIncidents take `any`, and that is how the two sides of this
// format silently drifted apart: the writer stored a map[string]*model.Incident
// while the reader asked for a []model.PersistedIncident, so every restore
// failed and the only symptom was one log line. Nothing in the type system
// objected. Routing production code through these two functions puts the
// compiler back in charge of the contract; the untyped pair stays for the codec
// tests.
func (s *StateManager) SavePersistedIncidents(
	ctx context.Context,
	incidents []model.PersistedIncident,
) error {
	return s.SaveIncidents(ctx, trimIncidentsToBudget(incidents))
}

// trimIncidentsToBudget drops the least recently seen incidents until the
// gzipped payload fits a ConfigMap.
//
// The previous behaviour was all-or-nothing: one oversized snapshot meant
// nothing was written at all, so on a large cluster kwatch silently stopped
// persisting and lost every incident on each restart. Keeping the freshest
// incidents that fit is strictly better — partial memory beats none, and the
// ones dropped are the stalest.
func trimIncidentsToBudget(
	incidents []model.PersistedIncident,
) []model.PersistedIncident {
	if len(incidents) == 0 {
		return incidents
	}
	if data, err := gzJSON(
		incidents,
	); err == nil &&
		len(data) <= baselineMaxBytes {
		return incidents
	}

	// Freshest first, so truncation sheds the stalest.
	trimmed := make([]model.PersistedIncident, len(incidents))
	copy(trimmed, incidents)
	sort.SliceStable(trimmed, func(i, j int) bool {
		return trimmed[i].LastSeen.After(trimmed[j].LastSeen)
	})

	// Halve until it fits; a linear walk over thousands of incidents would
	// re-gzip thousands of times.
	n := len(trimmed)
	for n > 1 {
		n /= 2
		data, err := gzJSON(trimmed[:n])
		if err != nil {
			// Returning nil here would persist an empty list and erase every
			// incident. Hand back the input and let the save fail loudly.
			return incidents
		}
		if len(data) <= baselineMaxBytes {
			break
		}
	}
	klog.ErrorS(
		nil,
		"incident state exceeds the ConfigMap budget; keeping the most recent",
		"kept",
		n,
		"dropped",
		len(incidents)-n,
		"max",
		baselineMaxBytes,
	)
	return trimmed[:n]
}

// LoadPersistedIncidents reads incidents back, accepting the legacy object
// layout written by older versions. Without this an upgrade drops correlation
// memory and re-announces everything already broken as brand new.
func (s *StateManager) LoadPersistedIncidents(
	ctx context.Context,
) ([]model.PersistedIncident, error) {
	var incidents []model.PersistedIncident
	err := s.GetIncidents(ctx, &incidents)
	if err == nil {
		return incidents, nil
	}

	// Older releases stored an object keyed by incident id. Field names differ
	// only in case, which encoding/json matches, so the entries decode.
	var legacy map[string]model.PersistedIncident
	if legacyErr := s.GetIncidents(
		ctx,
		&legacy,
	); legacyErr != nil ||
		len(legacy) == 0 {
		return nil, err
	}

	keys := make([]string, 0, len(legacy))
	for k := range legacy {
		keys = append(keys, k)
	}
	// Map iteration order is random; keep restores deterministic.
	sort.Strings(keys)

	migrated := make([]model.PersistedIncident, 0, len(legacy))
	for _, k := range keys {
		migrated = append(migrated, legacy[k])
	}
	klog.InfoS("migrated incident state from the legacy object layout",
		"count", len(migrated))
	return migrated, nil
}
