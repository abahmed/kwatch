package handler

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
)

const defaultNamespaceStuckMinutes = 10

func (h *handler) ProcessResourceQuota(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid resourcequota key %q: %w", key, err)
	}
	owner := namespace + "/" + name
	if deleted {
		h.correlator.ResolveByResource("resourcequota", owner)
		return nil
	}
	quota, err := h.listers.ResourceQuota.ResourceQuotas(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("resourcequota", owner)
			return nil
		}
		return fmt.Errorf("failed to get resourcequota %s from cache: %w", owner, err)
	}
	if sig := DetectResourceQuotaIssue(quota); sig != nil {
		h.signalEvent(sig)
	} else {
		h.correlator.ResolveByResource("resourcequota", owner)
	}
	return nil
}

// DetectResourceQuotaIssue reports only exhausted hard limits. Usage close to
// a limit is not a failure and is intentionally left to metrics/prediction
// monitors, which prevents quota alerts from becoming noisy.
func DetectResourceQuotaIssue(quota *corev1.ResourceQuota) *event.Signal {
	if quota == nil {
		return nil
	}
	for resource, hard := range quota.Status.Hard {
		used, ok := quota.Status.Used[resource]
		if !ok || hard.IsZero() || used.Cmp(hard) < 0 {
			continue
		}
		owner := quota.Namespace + "/" + quota.Name
		return &event.Signal{
			Resource: "resourcequota", Namespace: quota.Namespace,
			PodName: quota.Name, Owner: owner,
			Reason: constant.ReasonResourceQuotaExhausted,
			Labels: quota.Labels,
			Hint: fmt.Sprintf("ResourceQuota %s exhausted for %s (%s used)",
				owner, resource, used.String()),
		}
	}
	return nil
}

func (h *handler) ProcessNamespace(key string, deleted bool) error {
	if !h.namespaceInScope(key) {
		return nil
	}
	if deleted {
		h.correlator.ResolveByResource("namespace", key)
		return nil
	}
	ns, err := h.listers.Namespace.Get(key)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("namespace", key)
			return nil
		}
		return fmt.Errorf("failed to get namespace %s from cache: %w", key, err)
	}
	if sig := DetectNamespaceIssue(ns, h.now(), h.config.ClusterResourceMonitor.SustainedMinutes); sig != nil {
		h.signalEvent(sig)
	} else {
		h.correlator.ResolveByResource("namespace", key)
	}
	return nil
}

func (h *handler) namespaceInScope(name string) bool {
	for _, forbidden := range h.config.ForbiddenNamespaces {
		if forbidden == name {
			return false
		}
	}
	if h.namespaceScopeAll {
		return true
	}
	if len(h.namespaceScope) > 0 {
		_, ok := h.namespaceScope[name]
		return ok
	}
	if len(h.config.AllowedNamespaces) == 0 {
		return true
	}
	for _, allowed := range h.config.AllowedNamespaces {
		if allowed == name {
			return true
		}
	}
	return false
}

func DetectNamespaceIssue(ns *corev1.Namespace, now time.Time, sustainedMinutes int) *event.Signal {
	if ns == nil || ns.Status.Phase != corev1.NamespaceTerminating || ns.DeletionTimestamp == nil {
		return nil
	}
	if sustainedMinutes <= 0 {
		sustainedMinutes = defaultNamespaceStuckMinutes
	}
	age := now.Sub(ns.DeletionTimestamp.Time)
	if age < time.Duration(sustainedMinutes)*time.Minute {
		return nil
	}
	return &event.Signal{
		Resource: "namespace", Namespace: ns.Name, PodName: ns.Name,
		Owner: ns.Name, Reason: constant.ReasonNamespaceStuck, Labels: ns.Labels,
		Hint: fmt.Sprintf("namespace %s has been terminating for %s; inspect finalizers", ns.Name, age.Round(time.Minute)),
	}
}
