package kube

import (
	"sort"
	"strconv"
	"strings"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory"
)

// This file holds the spec diffs of the smaller kinds: where traffic is
// sent (Ingress, routes), what scales (HPA), what may be disrupted (PDB)
// and where pods may run (node labels). Each reports short readable text.

func ingressRuleChanges(
	before, after *networkingv1.Ingress,
) []inventory.FieldChange {
	return appendText(nil, "spec.rules",
		ingressRuleText(before), ingressRuleText(after))
}

// ingressRuleText lists "host/path" for every rule. The backends are
// reported on their own as spec.backends.
func ingressRuleText(ing *networkingv1.Ingress) string {
	var parts []string
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			parts = append(parts, rule.Host)
			continue
		}
		for _, path := range rule.HTTP.Paths {
			parts = append(parts, rule.Host+path.Path)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// routeChanges reports a Gateway API route's hostnames and backends.
func routeChanges(before, after *unstructured.Unstructured,
) []inventory.FieldChange {
	oldHosts, _, _ := unstructured.NestedStringSlice(
		before.Object, "spec", "hostnames")
	newHosts, _, _ := unstructured.NestedStringSlice(
		after.Object, "spec", "hostnames")
	fields := appendText(nil, "spec.hostnames",
		sortedJoin(oldHosts), sortedJoin(newHosts))
	return appendText(fields, "spec.backends",
		routeBackends(before), routeBackends(after))
}

func sortedJoin(values []string) string {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

// routeBackends lists "service:port" for every backend reference.
func routeBackends(u *unstructured.Unstructured) string {
	rules, _, _ := unstructured.NestedSlice(u.Object, "spec", "rules")
	var parts []string
	for _, rule := range rules {
		fields, _ := rule.(map[string]any)
		refs, _ := fields["backendRefs"].([]any)
		for _, ref := range refs {
			backend, _ := ref.(map[string]any)
			name, _ := backend["name"].(string)
			if port, ok := backend["port"].(int64); ok {
				name += ":" + strconv.FormatInt(port, 10)
			}
			parts = append(parts, name)
		}
	}
	return sortedJoin(parts)
}

// hpaTargetChanges reports the metric targets an autoscaler aims for.
func hpaTargetChanges(
	before, after *autoscalingv2.HorizontalPodAutoscaler,
) []inventory.FieldChange {
	return appendText(nil, "spec.metrics",
		hpaMetrics(before), hpaMetrics(after))
}

func hpaMetrics(hpa *autoscalingv2.HorizontalPodAutoscaler) string {
	var parts []string
	for _, metric := range hpa.Spec.Metrics {
		if metric.Type == autoscalingv2.ResourceMetricSourceType &&
			metric.Resource != nil {
			parts = append(parts, string(metric.Resource.Name)+" "+
				targetText(metric.Resource.Target))
			continue
		}
		parts = append(parts, string(metric.Type))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func targetText(target autoscalingv2.MetricTarget) string {
	switch {
	case target.AverageUtilization != nil:
		return strconv.Itoa(int(*target.AverageUtilization)) + "%"
	case target.AverageValue != nil:
		return target.AverageValue.String()
	case target.Value != nil:
		return target.Value.String()
	}
	return ""
}

// pdbChanges reports the budget's limits and which pods it covers.
func pdbChanges(before, after *policyv1.PodDisruptionBudget,
) []inventory.FieldChange {
	fields := appendText(nil, "spec.minAvailable",
		intText(before.Spec.MinAvailable), intText(after.Spec.MinAvailable))
	fields = appendText(fields, "spec.maxUnavailable",
		intText(before.Spec.MaxUnavailable),
		intText(after.Spec.MaxUnavailable))
	return appendText(fields, "spec.selector",
		selectorText(before.Spec.Selector), selectorText(after.Spec.Selector))
}

func intText(value *intstr.IntOrString) string {
	if value == nil {
		return ""
	}
	return value.String()
}

// nodeLabelChanges reports node labels that were added, changed or
// removed, as one field per key. Scheduling reads labels, so an edit can
// make pods unschedulable.
func nodeLabelChanges(before, after *corev1.Node) []inventory.FieldChange {
	var fields []inventory.FieldChange
	for _, key := range sortedKeys(after.Labels) {
		fields = appendText(fields, "labels."+key,
			before.Labels[key], after.Labels[key])
	}
	for _, key := range sortedKeys(before.Labels) {
		if _, ok := after.Labels[key]; !ok {
			fields = append(fields, inventory.FieldChange{
				Path: "labels." + key, Before: before.Labels[key]})
		}
	}
	return fields
}

// policyChanges reports what a NetworkPolicy now selects and how many
// rules it holds. The rules themselves are summarised: a change is a
// count and the digest of the whole spec.
func policyChanges(before, after *networkingv1.NetworkPolicy,
) []inventory.FieldChange {
	b, a := &before.Spec, &after.Spec
	fields := appendText(nil, "spec.podSelector",
		selectorText(&b.PodSelector), selectorText(&a.PodSelector))
	fields = appendText(fields, "spec.policyTypes",
		policyTypes(b), policyTypes(a))
	fields = appendText(fields, "spec.ingress",
		ruleCount(len(b.Ingress)), ruleCount(len(a.Ingress)))
	return appendText(fields, "spec.egress",
		ruleCount(len(b.Egress)), ruleCount(len(a.Egress)))
}

func policyTypes(spec *networkingv1.NetworkPolicySpec) string {
	parts := make([]string, 0, len(spec.PolicyTypes))
	for _, t := range spec.PolicyTypes {
		parts = append(parts, string(t))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func ruleCount(n int) string { return strconv.Itoa(n) + " rules" }
