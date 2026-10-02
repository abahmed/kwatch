package kube

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Attribute names for namespace policy checks.
const (
	AttrPodSecurityInvalid                = "pod.security.invalid"
	AttrLimitsInvalid                     = "limits.invalid"
	KindLimitRange         inventory.Kind = "limitrange"
)

// podSecurityIncident validates Pod Security Admission labels; an invalid
// level or version makes the admission controller reject pods.
func podSecurityIncident(labels map[string]string) string {
	for _, mode := range []string{"enforce", "warn", "audit"} {
		level := labels["pod-security.kubernetes.io/"+mode]
		if level != "" && level != "privileged" && level != "baseline" &&
			level != "restricted" {
			return fmt.Sprintf("%s level %q is not privileged, baseline "+
				"or restricted", mode, level)
		}
		version := labels["pod-security.kubernetes.io/"+mode+"-version"]
		if version != "" && version != "latest" &&
			!strings.HasPrefix(version, "v1.") {
			return fmt.Sprintf("%s version %q is not latest or v1.<minor>",
				mode, version)
		}
	}
	return ""
}

// LimitRangeSchema describes LimitRanges and validates their constraints.
type LimitRangeSchema struct{}

// Kind implements Schema.
func (LimitRangeSchema) Kind() inventory.Kind { return KindLimitRange }

// RelationTypes implements Schema.
func (LimitRangeSchema) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{inventory.Constrains}
}

// Describe implements Schema.
func (LimitRangeSchema) Describe(obj any) (Description, bool) {
	lr, ok := obj.(*corev1.LimitRange)
	if !ok {
		return Description{}, false
	}
	rel := relations{}
	rel.add(inventory.Constrains,
		inventory.CoreID(KindNamespace, "", lr.Namespace))
	return Description{
		ID: objectID(KindLimitRange, lr), UID: string(lr.UID),
		Attributes: map[string]inventory.Value{
			AttrLimitsInvalid: inventory.Text(limitRangeIncident(lr)),
		},
		Relations: rel,
	}, true
}

// Diff implements Schema.
func (LimitRangeSchema) Diff(_, _ any) []inventory.FieldChange { return nil }

// limitRangeIncident finds contradictory constraints that make every pod in
// the namespace fail admission.
func limitRangeIncident(lr *corev1.LimitRange) string {
	var incidents []string
	for _, item := range lr.Spec.Limits {
		for resource, minimum := range item.Min {
			if maximum, ok := item.Max[resource]; ok &&
				minimum.Cmp(maximum) > 0 {
				incidents = append(incidents, fmt.Sprintf(
					"%s min %s exceeds max %s", resource, minimum.String(),
					maximum.String()))
			}
		}
		for resource, request := range item.DefaultRequest {
			if limit, ok := item.Default[resource]; ok &&
				request.Cmp(limit) > 0 {
				incidents = append(incidents, fmt.Sprintf(
					"%s default request %s exceeds default limit %s",
					resource, request.String(), limit.String()))
			}
		}
	}
	sort.Strings(incidents)
	return strings.Join(incidents, "; ")
}
