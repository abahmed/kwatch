package incident

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// isolationBlocksAdmission reports an incident rooted at a policy that
// isolates the backend of a fail-closed webhook: the creates the webhook
// intercepts fail in every namespace, as if the webhook itself were the
// root.
func isolationBlocksAdmission(model inventory.Reader, p *Incident) bool {
	if p.Cause == nil || p.Cause.Root != p.Root ||
		p.Cause.Rule != explain.RulePolicyIsolatesBackend {
		return false
	}
	for _, kind := range webhookKinds {
		for _, hook := range model.Entities(kind) {
			if failsClosed(model, hook) && serves(model, hook, p.Root) {
				return true
			}
		}
	}
	return false
}

// failsClosed reports a webhook whose failurePolicy is Fail.
func failsClosed(model inventory.Reader, hook inventory.EntityID) bool {
	e, ok := model.Entity(hook)
	if !ok {
		return false
	}
	policy, _ := e.Attribute(kube.AttrFailurePolicy)
	return strings.Contains(policy.Value.AsText(), "Fail")
}

// serves reports a webhook whose Service has the policy among the
// policies that isolate its pods.
func serves(
	model inventory.Reader, hook, policy inventory.EntityID,
) bool {
	for _, svc := range model.Related(
		hook, inventory.Serves, inventory.Outgoing) {
		for _, id := range kube.IsolatingPolicies(model, svc) {
			if id == policy {
				return true
			}
		}
	}
	return false
}
