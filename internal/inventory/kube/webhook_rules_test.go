package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// writeRule is a CREATE and UPDATE rule for resource in group.
func writeRule(group, resource string) admissionv1.RuleWithOperations {
	return admissionv1.RuleWithOperations{
		Operations: []admissionv1.OperationType{
			admissionv1.Create, admissionv1.Update},
		Rule: admissionv1.Rule{APIGroups: []string{group},
			Resources: []string{resource}}}
}

// guardHook is a validating webhook calling Service guard/guard-svc with
// policy and rules.
func guardHook(
	name string, policy admissionv1.FailurePolicyType,
	rules ...admissionv1.RuleWithOperations,
) admissionv1.ValidatingWebhook {
	return admissionv1.ValidatingWebhook{Name: name,
		FailurePolicy: &policy, Rules: rules,
		ClientConfig: admissionv1.WebhookClientConfig{
			Service: &admissionv1.ServiceReference{
				Namespace: "guard", Name: "guard-svc"}}}
}

func describeHooks(
	t *testing.T, hooks ...admissionv1.ValidatingWebhook,
) map[string]string {
	t.Helper()
	d, ok := kube.WebhookSchema{}.Describe(
		&admissionv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "guard"},
			Webhooks:   hooks})
	require.True(t, ok)
	out := map[string]string{}
	for name, v := range d.Attributes {
		out[name] = v.AsText()
	}
	return out
}

func TestWebhookClosedRulesListTheFailClosedRules(t *testing.T) {
	hook := guardHook("a", admissionv1.Fail, writeRule("", "pods"))
	hook.NamespaceSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{"team": "web"}}
	ignored := guardHook("b", admissionv1.Ignore,
		writeRule("apps", "deployments"))

	got := describeHooks(t, hook, ignored)

	assert.Equal(t, "guard/guard-svc||pods|*|1|team=web|",
		got[kube.AttrWebhookClosedRules])
}

func TestWebhookClosedRulesMarkRulesForOtherOperations(t *testing.T) {
	rule := writeRule("", "namespaces")
	rule.Operations = []admissionv1.OperationType{admissionv1.Delete}
	scope := admissionv1.ClusterScope
	rule.Scope = &scope

	got := describeHooks(t, guardHook("a", admissionv1.Fail, rule))

	assert.Equal(t, "guard/guard-svc||namespaces|Cluster|0||",
		got[kube.AttrWebhookClosedRules])
}

// A webhook without rules, or that ignores failure, leaves what its
// webhooks intercept unknown: the attribute is absent.
func TestWebhookClosedRulesAbsentWithoutFailClosedRules(t *testing.T) {
	got := describeHooks(t, guardHook("a", admissionv1.Fail),
		guardHook("b", admissionv1.Ignore, writeRule("", "pods")))

	assert.NotContains(t, got, kube.AttrWebhookClosedRules)
}
