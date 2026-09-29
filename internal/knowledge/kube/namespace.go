package kube

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// Attribute names for namespace policy checks.
const (
	AttrPodSecurityInvalid                = "pod.security.invalid"
	AttrLimitsInvalid                     = "limits.invalid"
	KindLimitRange         knowledge.Kind = "limitrange"
)

// podSecurityProblem validates Pod Security Admission labels; an invalid
// level or version makes the admission controller reject pods.
func podSecurityProblem(labels map[string]string) string {
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
func (LimitRangeSchema) Kind() knowledge.Kind { return KindLimitRange }

// RelationTypes implements Schema.
func (LimitRangeSchema) RelationTypes() []knowledge.RelationType {
	return []knowledge.RelationType{knowledge.Constrains}
}

// Describe implements Schema.
func (LimitRangeSchema) Describe(obj any) (Description, bool) {
	lr, ok := obj.(*corev1.LimitRange)
	if !ok {
		return Description{}, false
	}
	rel := relations{}
	rel.add(knowledge.Constrains,
		knowledge.NewEntityID(KindNamespace, "", lr.Namespace))
	return Description{
		ID: objectID(KindLimitRange, lr), UID: string(lr.UID),
		Attributes: map[string]knowledge.Value{
			AttrLimitsInvalid: knowledge.Text(limitRangeProblem(lr)),
		},
		Relations: rel,
	}, true
}

// Diff implements Schema.
func (LimitRangeSchema) Diff(_, _ any) []knowledge.FieldChange { return nil }

// limitRangeProblem finds contradictory constraints that make every pod in
// the namespace fail admission.
func limitRangeProblem(lr *corev1.LimitRange) string {
	var problems []string
	for _, item := range lr.Spec.Limits {
		for resource, minimum := range item.Min {
			if maximum, ok := item.Max[resource]; ok &&
				minimum.Cmp(maximum) > 0 {
				problems = append(problems, fmt.Sprintf(
					"%s min %s exceeds max %s", resource, minimum.String(),
					maximum.String()))
			}
		}
		for resource, request := range item.DefaultRequest {
			if limit, ok := item.Default[resource]; ok &&
				request.Cmp(limit) > 0 {
				problems = append(problems, fmt.Sprintf(
					"%s default request %s exceeds default limit %s",
					resource, request.String(), limit.String()))
			}
		}
	}
	sort.Strings(problems)
	return strings.Join(problems, "; ")
}
