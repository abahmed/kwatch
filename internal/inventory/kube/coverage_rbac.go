package kube

import (
	"sort"
	"strings"
)

// RBACRule is one ClusterRole rule, in the field names of
// rbac.authorization.k8s.io/v1 PolicyRule, so the dashboard can print it
// as YAML the operator copies into kwatch's ClusterRole.
type RBACRule struct {
	APIGroups []string `json:"apiGroups"`
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
}

// watchVerbs are what a watch needs: the initial list, the watch and the
// reads of single objects.
var watchVerbs = []string{"get", "list", "watch"}

// splitResourceKey reverses resourceKey: resources hold no dot, so the
// first dot separates the resource from its group.
func splitResourceKey(key string) (resource, group string) {
	resource, group, _ = strings.Cut(key, ".")
	return resource, group
}

// ruleFor is the rule that grants kwatch the watch of a kind refused to
// it, named by its resource key ("pods", "certificates.cert-manager.io").
func ruleFor(kind string) *RBACRule {
	resource, group := splitResourceKey(kind)
	return &RBACRule{
		APIGroups: []string{group}, Resources: []string{resource},
		Verbs: append([]string(nil), watchVerbs...),
	}
}

// missingRules merges the rules of every refused kind into one rule per
// API group, groups and resources sorted: the whole fix for the
// ClusterRole, the kinds a bounded list leaves out included.
func missingRules(gaps []KindGap) []RBACRule {
	byGroup := map[string]map[string]bool{}
	for _, gap := range gaps {
		if gap.Reason != ReasonPermissionDenied {
			continue
		}
		resource, group := splitResourceKey(gap.Kind)
		if byGroup[group] == nil {
			byGroup[group] = map[string]bool{}
		}
		byGroup[group][resource] = true
	}
	out := make([]RBACRule, 0, len(byGroup))
	for group, resources := range byGroup {
		names := make([]string, 0, len(resources))
		for name := range resources {
			names = append(names, name)
		}
		sort.Strings(names)
		out = append(out, RBACRule{
			APIGroups: []string{group}, Resources: names,
			Verbs: append([]string(nil), watchVerbs...),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].APIGroups[0] < out[j].APIGroups[0]
	})
	if len(out) == 0 {
		return nil
	}
	return out
}
