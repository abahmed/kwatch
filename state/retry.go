package state

import (
	"context"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	maxRetries    = 3
	retryDelay    = 100 * time.Millisecond
	configMapName = "kwatch-state"
)

type ConfigMapUpdater func(cm *corev1.ConfigMap) error

type RetryConfigMapManager struct {
	client    kubernetes.Interface
	namespace string
}

func NewRetryConfigMapManager(client kubernetes.Interface, namespace string) *RetryConfigMapManager {
	return &RetryConfigMapManager{
		client:    client,
		namespace: namespace,
	}
}

func (r *RetryConfigMapManager) UpdateWithRetry(
	ctx context.Context,
	updater ConfigMapUpdater,
) error {
	for i := 0; i < maxRetries; i++ {
		cm, err := r.client.CoreV1().
			ConfigMaps(r.namespace).
			Get(ctx, configMapName, metav1.GetOptions{})
		if err != nil {
			return err
		}

		if err := updater(cm); err != nil {
			return err
		}

		_, err = r.client.CoreV1().
			ConfigMaps(r.namespace).
			Update(ctx, cm, metav1.UpdateOptions{})
		if err == nil {
			return nil
		}

		if !isConflictError(err) {
			return err
		}

		logrus.Debugf("configmap conflict, retry %d/%d", i+1, maxRetries)
		time.Sleep(retryDelay)
	}

	return &ConflictError{Message: "failed after max retries"}
}

type ConflictError struct {
	Message string
}

func (e *ConflictError) Error() string {
	return e.Message
}

func isConflictError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "conflict") ||
		strings.Contains(msg, "Conflict") ||
		strings.Contains(msg, "was changed")
}
