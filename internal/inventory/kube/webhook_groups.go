package kube

import (
	"sort"
	"strings"

	admissionv1 "k8s.io/api/admissionregistration/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrWebhookGroups lists the custom API groups the rules of a webhook
// configuration cover, space-separated and sorted. A webhook that
// admits a custom group is served by that group's controller, so the
// workload behind it is evidence of who handles the group's objects.
// Wildcard groups and built-in groups are left out.
const AttrWebhookGroups = "webhook.groups"

// configAttributes are the attributes of a webhook configuration:
// failure policies, webhook names and the custom groups its rules cover.
func configAttributes(
	policies, hooks []string, rules []admissionv1.RuleWithOperations,
) map[string]inventory.Value {
	attrs := map[string]inventory.Value{
		AttrFailurePolicy: inventory.Text(strings.Join(policies, ",")),
		AttrWebhookNames:  inventory.Text(strings.Join(hooks, ",")),
	}
	if groups := webhookRuleGroups(rules); len(groups) > 0 {
		attrs[AttrWebhookGroups] = inventory.Text(
			strings.Join(groups, " "))
	}
	return attrs
}

// webhookRuleGroups are the distinct custom API groups of the rules, at
// most maxRBACItems of them.
func webhookRuleGroups(rules []admissionv1.RuleWithOperations) []string {
	seen := map[string]bool{}
	var out []string
	for _, rule := range rules {
		for _, group := range rule.APIGroups {
			if customGroup(group) && !seen[group] {
				seen[group] = true
				out = append(out, group)
			}
		}
	}
	sort.Strings(out)
	if len(out) > maxRBACItems {
		out = out[:maxRBACItems]
	}
	return out
}
