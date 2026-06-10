package handler

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/filter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
)

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
		Client: h.kclient,
		Config: h.config,
		Pod:    pod,
		EvType: "ADDED",
	}

	h.executePodFilters(&ctx)
	h.executeContainersFilters(&ctx)
	return nil
}
