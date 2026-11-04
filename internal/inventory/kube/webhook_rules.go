package kube

import (
	"strings"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrWebhookClosedRules lists, for each Service a fail-closed webhook
// calls, the resources its rules intercept, one rule resource per line:
// "namespace/service|group|resource|scope|writes|namespaceSelector|
// objectSelector". writes is "1" when the rule covers CREATE or UPDATE;
// a selector is in label selector syntax, empty for "everything". It is
// absent when no fail-closed webhook has a rule or the list is over
// maxClosedRules, which leaves what the webhooks match unknown.
const AttrWebhookClosedRules = "webhook.closed.rules"

// maxClosedRules bounds the rule lines of one configuration.
const maxClosedRules = 64

// closedRuleFields is the number of fields of one rule line.
const closedRuleFields = 7

// hookSpec is the part of one webhook kwatch reads, whatever its type.
type hookSpec struct {
	name   string
	client admissionv1.WebhookClientConfig
	policy string
	rules  []admissionv1.RuleWithOperations
	nsSel  *metav1.LabelSelector
	objSel *metav1.LabelSelector
}

// mutatingSpecs and validatingSpecs read the webhooks of a
// configuration.
func mutatingSpecs(hooks []admissionv1.MutatingWebhook) []hookSpec {
	out := make([]hookSpec, 0, len(hooks))
	for _, h := range hooks {
		out = append(out, hookSpec{name: h.Name, client: h.ClientConfig,
			policy: failurePolicy(h.FailurePolicy), rules: h.Rules,
			nsSel: h.NamespaceSelector, objSel: h.ObjectSelector})
	}
	return out
}

func validatingSpecs(hooks []admissionv1.ValidatingWebhook) []hookSpec {
	out := make([]hookSpec, 0, len(hooks))
	for _, h := range hooks {
		out = append(out, hookSpec{name: h.Name, client: h.ClientConfig,
			policy: failurePolicy(h.FailurePolicy), rules: h.Rules,
			nsSel: h.NamespaceSelector, objSel: h.ObjectSelector})
	}
	return out
}

// withClosedRules adds the rule lines of the fail-closed webhooks that
// call a Service.
func withClosedRules(
	attrs map[string]inventory.Value, specs []hookSpec,
) map[string]inventory.Value {
	var lines []string
	for _, h := range specs {
		if h.client.Service == nil || strings.EqualFold(h.policy,
			string(admissionv1.Ignore)) {
			continue
		}
		lines = append(lines, ruleLines(h)...)
	}
	if len(lines) > 0 && len(lines) <= maxClosedRules {
		attrs[AttrWebhookClosedRules] = inventory.Text(
			strings.Join(lines, "\n"))
	}
	return attrs
}

// ruleLines are the rule lines of one webhook.
func ruleLines(h hookSpec) []string {
	service := h.client.Service.Namespace + "/" + h.client.Service.Name
	nsSel, objSel := hookSelectorText(h.nsSel), hookSelectorText(h.objSel)
	var out []string
	for _, rule := range h.rules {
		writes := "0"
		if coversWrites(rule.Operations) {
			writes = "1"
		}
		scope := "*"
		if rule.Scope != nil {
			scope = string(*rule.Scope)
		}
		for _, group := range rule.APIGroups {
			for _, resource := range rule.Resources {
				out = append(out, strings.Join([]string{service, group,
					resource, scope, writes, nsSel, objSel}, "|"))
			}
		}
	}
	return out
}

// coversWrites reports operations that include CREATE or UPDATE.
func coversWrites(ops []admissionv1.OperationType) bool {
	for _, op := range ops {
		switch op {
		case admissionv1.Create, admissionv1.Update,
			admissionv1.OperationAll:
			return true
		}
	}
	return false
}

// hookSelectorText writes a label selector in selector syntax: empty for
// none or an empty one, which both select everything.
func hookSelectorText(s *metav1.LabelSelector) string {
	if s == nil || (len(s.MatchLabels) == 0 &&
		len(s.MatchExpressions) == 0) {
		return ""
	}
	return metav1.FormatLabelSelector(s)
}
