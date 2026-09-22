//go:build e2e

package harness

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
)

func (e *Environment) DeleteNamespace(
	ctx context.Context,
	namespace string,
) error {
	err := e.Client.CoreV1().Namespaces().Delete(
		ctx, namespace, metav1.DeleteOptions{},
	)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return wait.PollUntilContextTimeout(ctx, 500*time.Millisecond,
		2*time.Minute, true, func(ctx context.Context) (bool, error) {
			_, getErr := e.Client.CoreV1().Namespaces().Get(
				ctx, namespace, metav1.GetOptions{},
			)
			return apierrors.IsNotFound(getErr), nil
		})
}

func (e *Environment) WaitForDeployment(
	ctx context.Context,
	namespace, name string,
) error {
	return wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 10*time.Minute,
		true, func(ctx context.Context) (bool, error) {
			deployment, err := e.Client.AppsV1().Deployments(namespace).Get(
				ctx, name, metav1.GetOptions{},
			)
			if err != nil {
				return false, nil
			}
			return deployment.Status.AvailableReplicas >= deployment.Status.Replicas &&
				deployment.Status.AvailableReplicas > 0, nil
		})
}

func (e *Environment) WaitForPod(
	ctx context.Context,
	namespace, name string,
	check func(*corev1.Pod) bool,
) error {
	pod, err := e.Client.CoreV1().Pods(namespace).Get(
		ctx, name, metav1.GetOptions{},
	)
	if err == nil && check(pod) {
		return nil
	}
	watcher, err := e.Client.CoreV1().Pods(namespace).Watch(
		ctx, metav1.ListOptions{FieldSelector: "metadata.name=" + name},
	)
	if err != nil {
		return fmt.Errorf("watch Pod %s/%s: %w", namespace, name, err)
	}
	defer watcher.Stop()
	for event := range watcher.ResultChan() {
		if event.Type == watch.Error {
			continue
		}
		pod, ok := event.Object.(*corev1.Pod)
		if ok && check(pod) {
			return nil
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("Pod %s/%s watch ended", namespace, name)
}

func (e *Environment) WaitForLeaseHolder(
	ctx context.Context,
	namespace, name string,
) (string, error) {
	var holder string
	err := wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, 10*time.Minute,
		true, func(ctx context.Context) (bool, error) {
			lease, err := e.Client.CoordinationV1().Leases(namespace).Get(
				ctx, name, metav1.GetOptions{},
			)
			if err != nil || lease.Spec.HolderIdentity == nil {
				return false, nil
			}
			holder = string(*lease.Spec.HolderIdentity)
			return strings.TrimSpace(holder) != "", nil
		})
	return holder, err
}

func (e *Environment) WaitForLeaseChange(
	ctx context.Context,
	namespace, name, previous string,
) (string, error) {
	var holder string
	err := wait.PollUntilContextTimeout(ctx, 250*time.Millisecond,
		10*time.Minute, true, func(ctx context.Context) (bool, error) {
			lease, err := e.Client.CoordinationV1().Leases(namespace).Get(
				ctx, name, metav1.GetOptions{},
			)
			if err != nil || lease.Spec.HolderIdentity == nil {
				return false, nil
			}
			holder = string(*lease.Spec.HolderIdentity)
			return holder != "" && holder != previous, nil
		})
	return holder, err
}

func (e *Environment) WaitForPodCount(
	ctx context.Context,
	namespace string,
	check func([]corev1.Pod) bool,
) error {
	return wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 10*time.Minute,
		true, func(ctx context.Context) (bool, error) {
			pods, err := e.Client.CoreV1().Pods(namespace).List(
				ctx, metav1.ListOptions{},
			)
			if err != nil {
				return false, nil
			}
			return check(pods.Items), nil
		})
}

func PodHasReason(pod *corev1.Pod, reason string) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Terminated != nil &&
			string(status.State.Terminated.Reason) == reason {
			return true
		}
		if status.State.Waiting != nil && status.State.Waiting.Reason == reason {
			return true
		}
	}
	return false
}
