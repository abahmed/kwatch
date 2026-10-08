package incident

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
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
	// failurePolicy is Fail and a request was refused because of it,
	// so only a webhook seen blocking pages (see idle_webhook.go).
	critical bool
	// admission also matches an incident whose root is a fail-closed
	// webhook blocking creates (Incident.admissionBlocked).
	admission bool
	// kinds match when the root holds one of these kinds, or when the
	// impact holds one and the incident lost traffic: one failing pod
	// behind an Ingress is not an outage while its Service still has
	// healthy backends.
	kinds []inventory.Kind
	// when matches an incident by a fact the manager computed from the
	// model, such as a user-facing workload with no ready replica.
	when func(*Incident) bool
}

// pageRules are every way an incident pages. A page interrupts someone
// at any hour, so it needs two things: a critical member, and one of
// these rules, each of which is a way users feel the failure.
//
//  1. Users lose traffic: a Service that an Ingress, a Gateway route, a
//     LoadBalancer or a NodePort exposes has no ready backend
//     (traffic-lost, exposed-service-lost).
//  2. A user-facing workload has no replica left: every replica of a
//     Deployment, StatefulSet or DaemonSet that such a Service selects
//     is not ready (last-replica-down). A workload only other workloads
//     call is not user-facing; the callers that fail are judged
//     instead, and the incident notifies.
//  3. A cluster-critical component is down: cluster DNS, the API server
//     and etcd, the scheduler, the controller manager, a fail-closed
//     admission webhook that blocks every create, or a node, which
//     takes its capacity and its network plugin with it (node-lost).
//
// Everything else that is critical notifies, and what is only worth
// knowing waits for the digest. docs/incident-lifecycle/announce.md says
// the same in prose; change both together.
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
	{name: "exposed-service-lost",
		when: func(p *Incident) bool { return p.trafficLost }},
	{name: "last-replica-down",
		when: func(p *Incident) bool { return p.servingDown }},
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
	if r.when != nil && r.when(p) {
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
// called blocks every create it intercepts, in every namespace. A
// webhook that answered and denied one workload's request is a policy
// decision, not an outage: that notifies.
func admissionBlocked(model inventory.Reader, p *Incident) bool {
	if !hasKind(webhookKinds, p.Root.Kind) || p.Cause == nil ||
		p.Cause.Root != p.Root || p.Cause.Rule != webhookRejectsRule ||
		p.Cause.Mode == explain.ModeWebhookDenied {
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
