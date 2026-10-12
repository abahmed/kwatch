package kube

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// webhookView is the part of a webhook that decides what it admits and
// how a failure of it is treated. Mutating and validating webhooks share
// these fields but not a type.
type webhookView struct {
	name, failurePolicy, timeout string
	namespaces, objects          string
	rules, caBundle              string
}

// webhookViews lists the webhooks of a configuration by name.
func webhookViews(obj any) (map[string]webhookView, bool) {
	out := map[string]webhookView{}
	switch config := obj.(type) {
	case *admissionv1.MutatingWebhookConfiguration:
		for _, h := range config.Webhooks {
			out[h.Name] = newWebhookView(h.Name, h.FailurePolicy,
				h.TimeoutSeconds, h.NamespaceSelector, h.ObjectSelector,
				h.Rules, h.ClientConfig)
		}
	case *admissionv1.ValidatingWebhookConfiguration:
		for _, h := range config.Webhooks {
			out[h.Name] = newWebhookView(h.Name, h.FailurePolicy,
				h.TimeoutSeconds, h.NamespaceSelector, h.ObjectSelector,
				h.Rules, h.ClientConfig)
		}
	default:
		return nil, false
	}
	return out, true
}

func newWebhookView(
	name string, policy *admissionv1.FailurePolicyType, timeout *int32,
	namespaces, objects *metav1.LabelSelector,
	rules []admissionv1.RuleWithOperations,
	client admissionv1.WebhookClientConfig,
) webhookView {
	view := webhookView{
		name: name, failurePolicy: failurePolicy(policy),
		namespaces: selectorText(namespaces),
		objects:    selectorText(objects),
		rules:      webhookRuleText(rules),
		caBundle:   bundleHash(client.CABundle),
	}
	if timeout != nil {
		view.timeout = fmt.Sprintf("%ds", *timeout)
	}
	return view
}

// webhookRuleText lists "operations apiGroups/apiVersions/resources".
func webhookRuleText(rules []admissionv1.RuleWithOperations) string {
	parts := make([]string, 0, len(rules))
	for _, rule := range rules {
		ops := make([]string, 0, len(rule.Operations))
		for _, op := range rule.Operations {
			ops = append(ops, string(op))
		}
		parts = append(parts, strings.Join(ops, "+")+" "+
			strings.Join(rule.APIGroups, "+")+"/"+
			strings.Join(rule.APIVersions, "+")+"/"+
			strings.Join(rule.Resources, "+"))
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

// bundleHash identifies a CA bundle without carrying it: a rotated
// certificate is a change, the bytes are not worth recording.
func bundleHash(bundle []byte) string {
	if len(bundle) == 0 {
		return ""
	}
	sum := sha256.Sum256(bundle)
	return hex.EncodeToString(sum[:6])
}

// webhookChanges compares two configurations hook by hook.
func webhookChanges(old, new any) []inventory.FieldChange {
	before, ok1 := webhookViews(old)
	after, ok2 := webhookViews(new)
	if !ok1 || !ok2 {
		return nil
	}
	names := make([]string, 0, len(after))
	for name := range after {
		names = append(names, name)
	}
	sort.Strings(names)
	var fields []inventory.FieldChange
	for _, name := range names {
		if _, known := before[name]; !known {
			fields = append(fields, inventory.FieldChange{
				Path: "webhooks." + name, After: "added"})
			continue
		}
		fields = append(fields,
			webhookViewChanges(before[name], after[name])...)
	}
	return append(fields, removedWebhooks(before, after)...)
}

func removedWebhooks(before, after map[string]webhookView,
) []inventory.FieldChange {
	var gone []string
	for name := range before {
		if _, kept := after[name]; !kept {
			gone = append(gone, name)
		}
	}
	sort.Strings(gone)
	fields := make([]inventory.FieldChange, 0, len(gone))
	for _, name := range gone {
		fields = append(fields, inventory.FieldChange{
			Path: "webhooks." + name, Before: "present",
			After: "removed"})
	}
	return fields
}

func webhookViewChanges(before, after webhookView) []inventory.FieldChange {
	prefix := "webhooks." + after.name + "."
	var fields []inventory.FieldChange
	for _, c := range []struct{ path, before, after string }{
		{"failurePolicy", before.failurePolicy, after.failurePolicy},
		{"timeoutSeconds", before.timeout, after.timeout},
		{"namespaceSelector", before.namespaces, after.namespaces},
		{"objectSelector", before.objects, after.objects},
		{"rules", before.rules, after.rules},
		{"caBundle", before.caBundle, after.caBundle},
	} {
		fields = appendText(fields, prefix+c.path, c.before, c.after)
	}
	return fields
}
