package incident

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// pageRule is one named reason a critical incident pages: it takes
// something away from everyone, not from one workload.
type pageRule struct {
	// name is stable; tests and docs refer to it.
	name string
	// reasons match a member with one of these finding reasons.
	reasons []string
	// critical asks the matching member itself to be critical. The
	// webhook detector marks a webhook critical only when its
	// failurePolicy is Fail, so only a blocking webhook pages.
	critical bool
	// admission also matches an incident whose root is a fail-closed
	// webhook blocking creates (Incident.admissionBlocked).
	admission bool
	// kinds match when the root holds one of these kinds, or when the
	// impact holds one and the incident lost traffic: one failing pod
	// behind an Ingress is not an outage while its Service still has
	// healthy backends.
	kinds []inventory.Kind
}

// pageRules are every way an incident pages, checked in order. An
// incident pages only when it also has a critical member; everything
// else critical notifies.
var pageRules = []pageRule{
	{name: "cluster-dns-failing",
		reasons: []string{reasons.CoreDNSUnavailable}},
	{name: "api-unavailable", reasons: []string{
		reasons.APIServerUnavailable, reasons.EtcdUnavailable}},
	{name: "scheduler-down",
		reasons: []string{reasons.SchedulerUnavailable}},
	{name: "controller-manager-down",
		reasons: []string{reasons.ControllerManagerUnavailable}},
	{name: "admission-blocked", critical: true, admission: true,
		reasons: []string{reasons.WebhookNoEndpoints,
			reasons.WebhookBackendNotFound}},
	{name: "traffic-lost", kinds: trafficKinds},
	{name: "node-lost", reasons: []string{
		reasons.NodeNotReady, reasons.NodeHeartbeatStale}},
}

// trafficKinds route users' requests into the cluster: when one of them
// loses its backends, users see errors.
var trafficKinds = []inventory.Kind{
	kube.KindIngress, "httproute", "grpcroute", "tlsroute", "tcproute",
}

// pageRuleOf names the first page rule p matches, or "" for none.
func pageRuleOf(p *Incident) string {
	for _, rule := range pageRules {
		if rule.matches(p) {
			return rule.name
		}
	}
	return ""
}

func (r pageRule) matches(p *Incident) bool {
	if r.admission && p.admissionBlocked {
		return true
	}
	for _, s := range p.Members {
		if r.matchesFinding(s) {
			return true
		}
	}
	if len(r.kinds) == 0 {
		return false
	}
	if hasKind(r.kinds, p.Root.Kind) {
		return true
	}
	if !p.trafficLost {
		return false
	}
	for _, id := range p.Impact {
		if hasKind(r.kinds, id.Kind) {
			return true
		}
	}
	return false
}

func (r pageRule) matchesFinding(s detection.Finding) bool {
	if r.critical && s.Severity != detection.Critical {
		return false
	}
	for _, reason := range r.reasons {
		if s.Reason == reason {
			return true
		}
	}
	return false
}

// webhookKinds are the admission webhook configurations.
var webhookKinds = []inventory.Kind{
	kube.KindValidatingHook, kube.KindMutatingWebhook,
}

// admissionBlocked reports an incident rooted at a webhook
// configuration that rejects creates (explain's webhook-rejects row)
// and fails closed: with failurePolicy Fail, a webhook that cannot be
// called blocks every create it intercepts, in every namespace.
func admissionBlocked(model inventory.Reader, p *Incident) bool {
	if !hasKind(webhookKinds, p.Root.Kind) || p.Cause == nil ||
		p.Cause.Root != p.Root || p.Cause.Rule != webhookRejectsRule {
		return false
	}
	e, ok := model.Entity(p.Root)
	if !ok {
		return false
	}
	policy, _ := e.Attribute(kube.AttrFailurePolicy)
	return strings.Contains(policy.Value.AsText(), "Fail")
}

// webhookRejectsRule is the explain row of a webhook blocking creates.
const webhookRejectsRule = "webhook-rejects"

func hasKind(kinds []inventory.Kind, kind inventory.Kind) bool {
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}
