package persistence

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/model"
)

func (s *Manager) GetRuntimeSession(
	ctx context.Context,
) (model.RuntimeSession, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(
		ctx, stateConfigMapName, metav1.GetOptions{},
	)
	if apierrors.IsNotFound(err) {
		return model.RuntimeSession{}, nil
	}
	if err != nil {
		return model.RuntimeSession{}, err
	}
	var session model.RuntimeSession
	if err := json.Unmarshal(
		[]byte(cm.Data[runtimeSessionKey]), &session,
	); err != nil && cm.Data[runtimeSessionKey] != "" {
		return model.RuntimeSession{}, err
	}
	return session, nil
}

func (s *Manager) SaveRuntimeSession(
	ctx context.Context, session model.RuntimeSession,
) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.configMapStore.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		cm.Data[runtimeSessionKey] = string(data)
		return nil
	})
}

// ClaimStartupAnnouncement atomically records the startup generation that
// announced a baseline. A second replica observing the same generation gets
// false, which prevents duplicate startup messages during leader races.
func (s *Manager) ClaimStartupAnnouncement(
	ctx context.Context,
	key string,
) (bool, error) {
	claimed := false
	err := s.configMapStore.UpdateWithRetry(ctx, func(
		cm *corev1.ConfigMap,
	) error {
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		if cm.Data[startupAnnouncementKey] == key {
			claimed = false
			return nil
		}
		cm.Data[startupAnnouncementKey] = key
		claimed = true
		return nil
	})
	return claimed, err
}

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
