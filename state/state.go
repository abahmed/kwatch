package state

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	stateConfigMapName = "kwatch-state"
	initKey            = "kwatch-init"
	clusterIDKey       = "cluster-id"
	versionKey         = "version"
	firstRunKey        = "first-run"
	notifiedVersionKey = "notified-version"
)

type StateManager struct {
	client    kubernetes.Interface
	namespace string
	retryMgr  *RetryConfigMapManager
}

func NewStateManager(client kubernetes.Interface, namespace string) *StateManager {
	return &StateManager{
		client:    client,
		namespace: namespace,
		retryMgr:  NewRetryConfigMapManager(client, namespace),
	}
}

func (s *StateManager) IsFirstRun(ctx context.Context) (bool, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		return true, nil
	}
	_, exists := cm.Data[initKey]
	return !exists, nil
}

func (s *StateManager) GetClusterID(ctx context.Context) (string, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return cm.Data[clusterIDKey], nil
}

func (s *StateManager) GetStoredVersion(ctx context.Context) string {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return cm.Data[versionKey]
}

func (s *StateManager) GetNotifiedVersion(ctx context.Context) string {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return cm.Data[notifiedVersionKey]
}

func (s *StateManager) SetNotifiedVersion(ctx context.Context, version string) error {
	return s.retryMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		cm.Data[notifiedVersionKey] = version
		return nil
	})
}

func (s *StateManager) EnsureClusterID(ctx context.Context) (string, error) {
	clusterID, err := s.GetClusterID(ctx)
	if err == nil && clusterID != "" {
		return clusterID, nil
	}
	return uuid.New().String(), nil
}

func (s *StateManager) MarkAsInitialized(ctx context.Context, clusterID, version string) error {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		cm = s.createConfigMap(clusterID, version)
		_, err = s.client.CoreV1().ConfigMaps(s.namespace).Create(ctx, cm, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		logrus.Infof("created state configmap with cluster ID: %s", clusterID)
		return nil
	}

	return s.retryMgr.UpdateWithRetry(ctx, func(c *corev1.ConfigMap) error {
		if _, exists := c.Data[initKey]; !exists {
			c.Data[initKey] = "true"
		}
		if _, exists := c.Data[clusterIDKey]; !exists || c.Data[clusterIDKey] == "" {
			c.Data[clusterIDKey] = clusterID
		}
		if _, exists := c.Data[firstRunKey]; !exists {
			c.Data[firstRunKey] = time.Now().UTC().Format(time.RFC3339)
		}
		c.Data[versionKey] = version
		return nil
	})
}

func (s *StateManager) createConfigMap(clusterID, version string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      stateConfigMapName,
			Namespace: s.namespace,
		},
		Data: map[string]string{
			initKey:      "true",
			clusterIDKey: clusterID,
			versionKey:   version,
			firstRunKey:  time.Now().UTC().Format(time.RFC3339),
		},
	}
}
