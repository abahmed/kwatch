package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/filter"
)

const stuckPodDeletionGrace = 10 * time.Minute

func isPodHealthy(pod *corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodRunning ||
		pod.Status.Phase == corev1.PodSucceeded {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil &&
				cs.State.Waiting.Reason != "ContainerCreating" &&
				cs.State.Waiting.Reason != "PodInitializing" {
				return false
			}
			if cs.State.Terminated != nil &&
				cs.State.Terminated.ExitCode != 0 &&
				cs.State.Terminated.Reason != "Completed" {
				return false
			}
		}
		return true
	}
	return false
}

func (h *handler) ProcessPod(
	ctx context.Context,
	key string,
	deleted bool,
) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid pod key %q: %w", key, err)
	}

	if deleted {
		h.correlator.RemovePod(namespace, name)
		return nil
	}

	pod, err := h.listers.Pod.Pods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.RemovePod(namespace, name)
			return nil
		}
		return fmt.Errorf(
			"failed to get pod %s/%s from cache: %w",
			namespace,
			name,
			err,
		)
	}

	return h.ProcessPodObject(ctx, pod, false)
}

func (h *handler) ProcessPodObject(
	parent context.Context,
	pod *corev1.Pod,
	deleted bool,
) error {
	if pod == nil {
		return nil
	}

	if deleted {
		h.correlator.RemovePod(pod.Namespace, pod.Name)
		return nil
	}

	ctxF := filter.Context{
		Sources: filter.Sources{
			Ctx:         parent,
			Client:      h.kclient,
			Config:      h.config,
			RSLister:    h.listers.RS,
			DSLister:    h.listers.DS,
			SSLister:    h.listers.SS,
			EventLister: h.listers.Event,
			EventsByPod: h.listers.EventsByPod,
			Now:         h.now,
		},
		Pod:    pod,
		EvType: "ADDED",
	}

	h.executePodFilters(&ctxF)
	h.executeContainersFilters(&ctxF)

	if sig := DetectPodDeletionIssue(pod, h.now()); sig != nil {
		h.signalEvent(sig)
	} else {
		h.correlator.MarkResolved(correlation.BuildKey(
			pod.Namespace, pod.Namespace+"/"+pod.Name,
			constant.ReasonPodStuckTerminating, "",
		))
	}

	if isPodHealthy(pod) {
		h.ClearBaselineForPod(pod.Namespace, pod.Name)
	}
	return nil
}

// DetectPodDeletionIssue catches pods that remain terminating because a
// finalizer or kubelet/runtime cleanup is stuck. The regular pod disruption
// filter suppresses planned deletion symptoms, while this independent signal
// preserves visibility into the stuck lifecycle itself.
func DetectPodDeletionIssue(pod *corev1.Pod, now time.Time) *event.Signal {
	if pod == nil || pod.DeletionTimestamp == nil || len(pod.Finalizers) == 0 {
		return nil
	}
	if now.Sub(pod.DeletionTimestamp.Time) < stuckPodDeletionGrace {
		return nil
	}
	return &event.Signal{
		Resource: "pod", Namespace: pod.Namespace, PodName: pod.Name,
		NodeName: pod.Spec.NodeName, Owner: pod.Namespace + "/" + pod.Name,
		Reason: constant.ReasonPodStuckTerminating, Labels: pod.Labels,
		Hint: fmt.Sprintf("pod has been terminating for %s with finalizers: %s",
			now.Sub(pod.DeletionTimestamp.Time).Round(time.Minute),
			strings.Join(pod.Finalizers, ", ")),
	}
}
