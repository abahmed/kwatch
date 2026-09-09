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
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
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
	podUID := podUIDFromQueueKey(key)
	if i := strings.LastIndex(key, "#"); i >= 0 {
		key = key[:i]
	}
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid pod key %q: %w", key, err)
	}

	if deleted {
		// A delete for an old Pod can arrive after Kubernetes has already
		// created its replacement with the same name. Do not let that stale
		// tombstone clear the replacement's incidents or startup baseline.
		if podUID != "" {
			current, lookupErr := h.listers.Pod.Pods(namespace).Get(name)
			if lookupErr == nil && string(current.UID) != podUID {
				return nil
			}
		}
		h.correlator.RemovePodWithUID(namespace, name, podUID)
		return nil
	}

	pod, err := h.listers.Pod.Pods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.RemovePodWithUID(namespace, name, podUID)
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

func podUIDFromQueueKey(key string) string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '#' {
			return key[i+1:]
		}
	}
	return ""
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
		h.correlator.RemovePodWithUID(pod.Namespace, pod.Name, string(pod.UID))
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
			LogCache:    h.logCache,
			Now:         h.now,
		},
		Pod:    pod,
		EvType: "ADDED",
	}

	h.executePodFilters(&ctxF)
	h.executeContainersFilters(&ctxF)
	owner := h.podOwnerFor(pod)
	for _, obs := range DetectPodReferenceIssues(pod, h.listers) {
		obs.Owner = owner
		h.observe(obs)
	}

	if obs := DetectPodDeletionIssue(pod, h.now()); obs != nil {
		obs.Owner = owner
		h.observe(obs)
	} else {
		// A pod incident is keyed by its owning workload, which is what the
		// reference names -- the kind stays "pod" because that is the
		// resource the incident is about.
		h.correlator.Resolve(
			model.ObjectRef{
				Kind: "pod", Namespace: pod.Namespace, Name: owner.Name,
			},
			constant.ReasonPodStuckTerminating,
		)
	}

	if isPodHealthy(pod) {
		h.ClearBaselineForPod(pod.Namespace, pod.Name, owner)
	}
	h.correlator.ResolveHealthyPodContainers(
		pod.Namespace,
		pod.Name,
		recoveredContainers(pod),
	)
	return nil
}

// recoveredContainers names the containers that are running and ready, which
// is the observed recovery for a container-level incident.
//
// Readiness is the load-bearing half. A crash-looping container is Running
// for a few seconds between restarts, so "Running" alone would resolve an
// incident mid-crash-loop; a container that has passed its readiness probe
// has actually started. The resolve hold-down is the second guard: a
// container that crashes again within it cancels the pending resolve.
func recoveredContainers(pod *corev1.Pod) map[string]bool {
	if pod == nil || pod.DeletionTimestamp != nil {
		return nil
	}
	if pod.Status.Phase != corev1.PodRunning {
		return nil
	}
	var out map[string]bool
	ready := 0
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Running == nil || !cs.Ready {
			continue
		}
		if out == nil {
			out = make(map[string]bool, len(pod.Status.ContainerStatuses))
		}
		out[cs.Name] = true
		ready++
	}
	// The empty name is the Pod itself, for incidents recorded without a
	// container. It only counts as recovered when every container is ready:
	// otherwise a Pod whose sidecar is still crashing would close a
	// Pod-scoped incident.
	if ready > 0 && ready == len(pod.Status.ContainerStatuses) {
		out[""] = true
	}
	// An init container that finished successfully is no longer failing, and
	// its incident is keyed by its own name.
	for _, cs := range pod.Status.InitContainerStatuses {
		if cs.State.Terminated == nil ||
			cs.State.Terminated.ExitCode != 0 {
			continue
		}
		if out == nil {
			out = make(map[string]bool)
		}
		out[cs.Name] = true
	}
	return out
}

// podOwnerFor resolves the owning workload through the listers, falling back
// to the pod itself when they cannot answer.
//
// The fallback used to be "namespace/pod" -- the inverse of every other
// producer -- so a stuck-terminating or missing-reference incident was filed
// under a different owner than that pod's crash incidents, and neither
// cascade suppression nor owner-health gating could connect them.
func (h *handler) podOwnerFor(pod *corev1.Pod) model.ObjectRef {
	if owner := h.Owners().OwnerOf(pod); owner.Name != "" {
		return owner
	}
	return observe.SelfOwner("Pod", pod.Namespace, pod.Name)
}

// DetectPodDeletionIssue catches pods that remain terminating because a
// finalizer or kubelet/runtime cleanup is stuck. The regular pod disruption
// filter suppresses planned deletion symptoms, while this independent signal
// preserves visibility into the stuck lifecycle itself.
func DetectPodDeletionIssue(
	pod *corev1.Pod, now time.Time,
) *model.Observation {
	if pod == nil || pod.DeletionTimestamp == nil || len(pod.Finalizers) == 0 {
		return nil
	}
	if now.Sub(pod.DeletionTimestamp.Time) < stuckPodDeletionGrace {
		return nil
	}
	return observe.PodOwnedBy(
		pod, "", constant.ReasonPodStuckTerminating, model.ObjectRef{},
	).WithHint(fmt.Sprintf(
		"pod has been terminating for %s with finalizers: %s",
		now.Sub(pod.DeletionTimestamp.Time).Round(time.Minute),
		strings.Join(pod.Finalizers, ", "),
	))
}
