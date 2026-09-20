package cluster

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"

	corev1 "k8s.io/api/core/v1"
)

const defaultNamespaceStuckMinutes = 10

// DetectLimitRangeIssue reports contradictory resource constraints.
func DetectLimitRangeIssue(
	limitRange *corev1.LimitRange,
) *model.Observation {
	if limitRange == nil {
		return nil
	}
	for _, item := range limitRange.Spec.Limits {
		for resource, minimum := range item.Min {
			if maximum, ok := item.Max[resource]; ok &&
				minimum.Cmp(maximum) > 0 {
				return limitRangeSignal(
					limitRange,
					resource,
					fmt.Sprintf(
						"min %s exceeds max %s",
						minimum.String(),
						maximum.String(),
					),
				)
			}
		}
		for resource, defaultValue := range item.Default {
			if minimum, ok := item.Min[resource]; ok &&
				defaultValue.Cmp(minimum) < 0 {
				return limitRangeSignal(
					limitRange,
					resource,
					fmt.Sprintf(
						"default %s is below min %s",
						defaultValue.String(),
						minimum.String(),
					),
				)
			}
			if maximum, ok := item.Max[resource]; ok &&
				defaultValue.Cmp(maximum) > 0 {
				return limitRangeSignal(
					limitRange,
					resource,
					fmt.Sprintf(
						"default %s exceeds max %s",
						defaultValue.String(),
						maximum.String(),
					),
				)
			}
		}
		for resource, request := range item.DefaultRequest {
			if defaultValue, ok := item.Default[resource]; ok &&
				request.Cmp(defaultValue) > 0 {
				return limitRangeSignal(
					limitRange,
					resource,
					fmt.Sprintf(
						"defaultRequest %s exceeds default %s",
						request.String(),
						defaultValue.String(),
					),
				)
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
		owner,
		resource,
		detail,
	))
}

// DetectResourceQuotaIssue reports exhausted hard limits.
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
			"resourcequota", quota,
			constant.ReasonResourceQuotaExhausted,
		).WithHint(fmt.Sprintf(
			"ResourceQuota %s exhausted for %s (%s used)",
			owner,
			resource,
			used.String(),
		))
	}
	return nil
}

// DetectNamespaceIssue reports a namespace stuck in Terminating.
func DetectNamespaceIssue(
	namespace *corev1.Namespace,
	now time.Time,
	sustainedMinutes int,
) *model.Observation {
	if namespace == nil ||
		namespace.Status.Phase != corev1.NamespaceTerminating ||
		namespace.DeletionTimestamp == nil {
		return nil
	}
	if sustainedMinutes <= 0 {
		sustainedMinutes = defaultNamespaceStuckMinutes
	}
	age := now.Sub(namespace.DeletionTimestamp.Time)
	if age < time.Duration(sustainedMinutes)*time.Minute {
		return nil
	}
	return observe.Namespace(
		namespace,
		constant.ReasonNamespaceStuck,
	).WithHint(fmt.Sprintf(
		"namespace %s has been terminating for %s; inspect finalizers",
		namespace.Name,
		age.Round(time.Minute),
	))
}

// DetectPodSecurityLabelIssue reports invalid Pod Security Admission labels.
func DetectPodSecurityLabelIssue(
	namespace *corev1.Namespace,
) *model.Observation {
	if namespace == nil {
		return nil
	}
	for _, mode := range []string{"enforce", "warn", "audit"} {
		level := namespace.Labels["pod-security.kubernetes.io/"+mode]
		if level != "" && level != "privileged" &&
			level != "baseline" && level != "restricted" {
			return observe.Namespace(
				namespace,
				constant.ReasonPodSecurityPolicyInvalid,
			).WithHint(fmt.Sprintf(
				"Pod Security Admission %s policy has invalid level %q;"+
					" expected privileged, baseline, or restricted",
				mode,
				level,
			))
		}
		version := namespace.Labels["pod-security.kubernetes.io/"+mode+"-version"]
		if version != "" && version != "latest" &&
			!strings.HasPrefix(version, "v1.") {
			return observe.Namespace(
				namespace,
				constant.ReasonPodSecurityPolicyInvalid,
			).WithHint(fmt.Sprintf(
				"Pod Security Admission %s policy has invalid version %q;"+
					" expected latest or v1.<minor>",
				mode,
				version,
			))
		}
	}
	return nil
}
