package handler

import (
	"context"
	"fmt"

	"github.com/abahmed/kwatch/internal/filter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
)

func isPodHealthy(pod *corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodSucceeded {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason != "ContainerCreating" && cs.State.Waiting.Reason != "PodInitializing" {
				return false
			}
			if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 && cs.State.Terminated.Reason != "Completed" {
				return false
			}
		}
		return true
	}
	return false
}

func (h *handler) ProcessPod(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid pod key %q: %w", key, err)
	}

	if deleted {
		h.correlator.RemovePod(namespace, name)
		return nil
	}

	pod, err := h.podLister.Pods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.RemovePod(namespace, name)
			return nil
		}
		return fmt.Errorf("failed to get pod %s/%s from cache: %w", namespace, name, err)
	}

	return h.ProcessPodObject(pod, false)
}

func (h *handler) ProcessPodObject(pod *corev1.Pod, deleted bool) error {
	if pod == nil {
		return nil
	}

	if deleted {
		h.correlator.RemovePod(pod.Namespace, pod.Name)
		return nil
	}

	ctx := filter.Context{
		Ctx:       context.Background(),
		Client:   h.kclient,
		Config:   h.config,
		Pod:      pod,
		EvType:   "ADDED",
		RSLister:   h.rsLister,
		DSLister:   h.dsLister,
		SSLister:   h.ssLister,
		EventLister: h.eventLister,
	}

	h.executePodFilters(&ctx)
	h.executeContainersFilters(&ctx)

	if isPodHealthy(pod) {
		h.ClearSeenForPod(pod.Namespace, pod.Name)
	}
	return nil
}
