package persistence

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *Manager) IsFirstRun(ctx context.Context) (bool, error) {
	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		stateConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	_, exists := cm.Data[initKey]
	return !exists, nil
}

func (s *Manager) GetClusterID(ctx context.Context) (string, error) {
	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		stateConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		return "", err
	}
	return cm.Data[clusterIDKey], nil
}

func (s *Manager) GetStoredVersion(ctx context.Context) (string, error) {
	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		stateConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return cm.Data[versionKey], nil
}

// GetStateSchemaVersion reports the persisted state format. An empty value is
// a legacy installation that predates explicit schema tracking.
func (s *Manager) GetStateSchemaVersion(ctx context.Context) string {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(
		ctx,
		stateConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		return ""
	}
	return cm.Data[stateSchemaVersionKey]
}

func (s *Manager) GetNotifiedVersion(ctx context.Context) string {
	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		stateConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		return ""
	}
	return cm.Data[notifiedVersionKey]
}

func (s *Manager) SetNotifiedVersion(
	ctx context.Context,
	version string,
) error {
	return s.configMapStore.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		cm.Data[notifiedVersionKey] = version
		return nil
	})
}

// SetLastSeen and GetLastSeen maintain the liveness gap marker.

// SetLastSeen records that kwatch was alive at t. Written periodically so a
// later start can tell how long monitoring was actually down.
