package state

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	maxRetries = 3
	retryDelay = 100 * time.Millisecond
)

type ConfigMapUpdater func(cm *corev1.ConfigMap) error

type RetryConfigMapManager struct {
	client     kubernetes.Interface
	namespace  string
	configName string
}

func NewRetryConfigMapManager(client kubernetes.Interface, namespace, configName string) *RetryConfigMapManager {
	return &RetryConfigMapManager{
		client:     client,
		namespace:  namespace,
		configName: configName,
	}
}

func (r *RetryConfigMapManager) UpdateWithRetry(
	ctx context.Context,
	updater ConfigMapUpdater,
) error {
	for i := 0; i < maxRetries; i++ {
		cm, err := r.client.CoreV1().
			ConfigMaps(r.namespace).
			Get(ctx, r.configName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      r.configName,
					Namespace: r.namespace,
				},
				Data: map[string]string{},
			}
			if updErr := updater(cm); updErr != nil {
				return updErr
			}
			if _, cErr := r.client.CoreV1().ConfigMaps(r.namespace).Create(ctx, cm, metav1.CreateOptions{}); cErr != nil {
				if apierrors.IsAlreadyExists(cErr) {
					continue
				}
				return cErr
			}
			return nil
		}
		if err != nil {
			return err
		}

		if cm.Data == nil {
			cm.Data = map[string]string{}
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

		klog.V(4).InfoS("configmap conflict, retrying", "attempt", i+1, "maxRetries", maxRetries)
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
