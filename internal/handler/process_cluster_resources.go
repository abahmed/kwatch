package handler

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

const defaultNamespaceStuckMinutes = 10

func (h *handler) ProcessResourceQuota(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid resourcequota key %q: %w", key, err)
	}
	subject := model.NewObjectRef("resourcequota", namespace, name)
	if deleted {
		h.reconcileGone(subject)
		return nil
	}
	quota, err := h.listers.ResourceQuota.ResourceQuotas(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.reconcileGone(subject)
			return nil
		}
		return fmt.Errorf(
			"failed to get resourcequota %s/%s from cache: %w",
			namespace, name, err,
		)
	}
	h.reconcile(subject, findings(DetectResourceQuotaIssue(quota)))
	return nil
}

func (h *handler) ProcessLimitRange(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid limitrange key %q: %w", key, err)
	}
	subject := model.NewObjectRef("limitrange", namespace, name)
	if deleted {
		h.reconcileGone(subject)
		return nil
	}
	limitRange, err := h.listers.LimitRange.LimitRanges(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.reconcileGone(subject)
			return nil
		}
		return fmt.Errorf(
			"failed to get limitrange %s/%s from cache: %w",
			namespace, name, err,
		)
	}
	h.reconcile(subject, findings(DetectLimitRangeIssue(limitRange)))
	return nil
}

// DetectLimitRangeIssue catches contradictory resource constraints that can
// make admissions fail or produce unusable defaults. Kubernetes normally
// rejects these at creation time, but older objects and upgraded clusters can
// still expose them through the informer cache.
func DetectLimitRangeIssue(
	limitRange *corev1.LimitRange,
) *model.Observation {
	if limitRange == nil {
		return nil
	}
	for _, item := range limitRange.Spec.Limits {
		for resource, minimum := range item.Min {
			if maximum, ok := item.Max[resource]; ok && minimum.Cmp(maximum) > 0 {
				return limitRangeSignal(limitRange, resource, fmt.Sprintf("min %s exceeds max %s", minimum.String(), maximum.String()))
			}
		}
		for resource, defaultValue := range item.Default {
			if minimum, ok := item.Min[resource]; ok && defaultValue.Cmp(minimum) < 0 {
				return limitRangeSignal(limitRange, resource, fmt.Sprintf("default %s is below min %s", defaultValue.String(), minimum.String()))
			}
			if maximum, ok := item.Max[resource]; ok && defaultValue.Cmp(maximum) > 0 {
				return limitRangeSignal(limitRange, resource, fmt.Sprintf("default %s exceeds max %s", defaultValue.String(), maximum.String()))
			}
		}
		for resource, request := range item.DefaultRequest {
			if defaultValue, ok := item.Default[resource]; ok && request.Cmp(defaultValue) > 0 {
				return limitRangeSignal(limitRange, resource, fmt.Sprintf("defaultRequest %s exceeds default %s", request.String(), defaultValue.String()))
			}
		}
	}
	return nil
}

func limitRangeSignal(
	limitRange *corev1.LimitRange,
	resource corev1.ResourceName,
	detail string,
) *model.Observation {
	owner := limitRange.Namespace + "/" + limitRange.Name
	return observe.Object(
		"limitrange", limitRange, constant.ReasonLimitRangeInvalid,
	).WithHint(fmt.Sprintf(
		"LimitRange %s has invalid %s constraint: %s",
		owner, resource, detail,
	))
}

// DetectResourceQuotaIssue reports only exhausted hard limits. Usage close to
// a limit is not a failure and is intentionally left to metrics/prediction
// monitors, which prevents quota alerts from becoming noisy.
func DetectResourceQuotaIssue(
	quota *corev1.ResourceQuota,
) *model.Observation {
	if quota == nil {
		return nil
	}
	for resource, hard := range quota.Status.Hard {
		used, ok := quota.Status.Used[resource]
		if !ok || hard.IsZero() || used.Cmp(hard) < 0 {
			continue
		}
		owner := quota.Namespace + "/" + quota.Name
		return observe.Object(
			"resourcequota", quota, constant.ReasonResourceQuotaExhausted,
		).WithHint(fmt.Sprintf(
			"ResourceQuota %s exhausted for %s (%s used)",
			owner, resource, used.String(),
		))
	}
	return nil
}

func (h *handler) ProcessNamespace(key string, deleted bool) error {
	if !h.namespaceInScope(key) {
		return nil
	}
	// A namespace is its own namespace, which is how its incidents are keyed.
	subject := model.NewObjectRef("namespace", key, key)
	if deleted {
		h.reconcileGone(subject)
		return nil
	}
	ns, err := h.listers.Namespace.Get(key)
	if err != nil {
		if errors.IsNotFound(err) {
			h.reconcileGone(subject)
			return nil
		}
		return fmt.Errorf("failed to get namespace %s from cache: %w", key, err)
	}
	h.reconcile(subject, findings(
		DetectNamespaceIssue(
			ns, h.now(), h.config.ClusterResourceMonitor.SustainedMinutes,
		),
		DetectPodSecurityLabelIssue(ns),
	))
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

func DetectNamespaceIssue(
	ns *corev1.Namespace, now time.Time, sustainedMinutes int,
) *model.Observation {
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
	return observe.Namespace(ns, constant.ReasonNamespaceStuck).
		WithHint(fmt.Sprintf(
			"namespace %s has been terminating for %s; inspect finalizers",
			ns.Name, age.Round(time.Minute),
		))
}

// DetectPodSecurityLabelIssue catches malformed Pod Security Admission labels.
// Actual admission denials are returned synchronously by the API server and
// are not retained as Namespace status, so the monitor reports only durable,
// objectively invalid policy configuration here.
func DetectPodSecurityLabelIssue(ns *corev1.Namespace) *model.Observation {
	if ns == nil {
		return nil
	}
	for _, mode := range []string{"enforce", "warn", "audit"} {
		level := ns.Labels["pod-security.kubernetes.io/"+mode]
		if level != "" && level != "privileged" && level != "baseline" && level != "restricted" {
			return observe.Namespace(
				ns, constant.ReasonPodSecurityPolicyInvalid,
			).WithHint(fmt.Sprintf(
				"Pod Security Admission %s policy has invalid level %q;"+
					" expected privileged, baseline, or restricted",
				mode, level,
			))
		}
		version := ns.Labels["pod-security.kubernetes.io/"+mode+"-version"]
		if version != "" && version != "latest" && !strings.HasPrefix(version, "v1.") {
			return observe.Namespace(
				ns, constant.ReasonPodSecurityPolicyInvalid,
			).WithHint(fmt.Sprintf(
				"Pod Security Admission %s policy has invalid version %q;"+
					" expected latest or v1.<minor>",
				mode, version,
			))
		}
	}
	return nil
}
