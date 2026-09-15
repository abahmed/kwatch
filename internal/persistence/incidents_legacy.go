package persistence

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SaveIncidents retains the pre-migration, untyped incident persistence API.
// New production code must use SavePersistedIncidents.
func (s *Manager) SaveIncidents(ctx context.Context, incidents any) error {
	return s.incidentsMgr.UpdateWithRetry(ctx, applyIncidents(incidents))
}

// GetIncidents retains the pre-migration, untyped incident persistence API.
// New production code must use LoadPersistedIncidents.
func (s *Manager) GetIncidents(ctx context.Context, out any) error {
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
