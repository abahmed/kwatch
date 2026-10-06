package kube

import (
	"net"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Bounds on one reachability check, so a namespace with many policies or
// a Service with many pods costs a fixed amount.
const (
	maxBackends        = 16
	maxPoliciesPerPod  = 64
	namespaceNameLabel = "kubernetes.io/metadata.name"
)

// Block says why a call cannot reach its Service: on one side, every
// NetworkPolicy that selects the pod restricts that direction and none
// of them allows the traffic.
type Block struct {
	// Policies are the policies that select the pod on the denying
	// side, sorted.
	Policies []inventory.EntityID
	// Egress is true when the caller's egress denies the traffic, false
	// when the ingress of the Service's pods does.
	Egress bool
	// Pod is the pod the policies select.
	Pod inventory.EntityID
}

// CallBlocked reports whether NetworkPolicies stop pod caller from
// reaching Service service on port (0 when the port is unknown, which
// checks every port of the Service). It follows Kubernetes: a pod no
// policy selects for a direction is open in that direction; once a
// policy selects it, only what some rule of the selecting policies
// allows gets through. The call is blocked only when it is blocked to
// every pod behind the Service. Anything kwatch cannot read (a named
// port, an address range for a pod without an IP, no pods behind the
// Service) counts as allowed: only a denial that can be shown is
// reported.
func CallBlocked(
	r inventory.Reader, caller, service inventory.EntityID, port int,
) (Block, bool) {
	from, ok := podFactsOf(r, caller)
	backends := backendPods(r, service)
	if !ok || len(backends) == 0 {
		return Block{}, false
	}
	targets := serviceTargets(r, service, port)
	var first Block
	for i, to := range backends {
		block, blocked := podsBlocked(r, from, to, targets)
		if !blocked {
			return Block{}, false
		}
		if i == 0 {
			first = block
		}
	}
	return first, true
}

// target is a port and protocol a Service sends to; Port 0 is unknown.
type target struct {
	port  int
	proto string
}

// podFacts are what policy rules match a pod by.
type podFacts struct {
	id       inventory.EntityID
	labels   labels.Set
	nsLabels labels.Set
	ip       net.IP
}

func podFactsOf(r inventory.Reader, id inventory.EntityID) (podFacts, bool) {
	entity, ok := r.Entity(id)
	if !ok {
		return podFacts{}, false
	}
	facts := podFacts{id: id, labels: labelSet(entityText(entity, AttrLabels)),
		ip: net.ParseIP(entityText(entity, AttrPodIP))}
	facts.nsLabels = labels.Set{namespaceNameLabel: id.Namespace}
	if ns, ok := r.Entity(inventory.CoreID(KindNamespace, "",
		id.Namespace)); ok {
		for k, v := range labelSet(entityText(ns, AttrLabels)) {
			facts.nsLabels[k] = v
		}
	}
	return facts, true
}

// labelSet reads "k=v,k=v" label text.
func labelSet(text string) labels.Set {
	set, err := labels.ConvertSelectorToLabelsMap(text)
	if err != nil {
		return labels.Set{}
	}
	return set
}

// backendPods are the pods the Service selects, at most maxBackends.
func backendPods(r inventory.Reader, service inventory.EntityID) []podFacts {
	var out []podFacts
	for _, link := range Links(r, service) {
		if link.Type != inventory.Selects {
			continue
		}
		if facts, ok := podFactsOf(r, link.To); ok {
			out = append(out, facts)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].id.Name < out[j].id.Name
	})
	if len(out) > maxBackends {
		out = out[:maxBackends]
	}
	return out
}

// serviceTargets are the ports traffic to the Service's port arrives
// on, read from the Service's "port/PROTO->targetPort" list. A named
// target port is unknown (0).
func serviceTargets(
	r inventory.Reader, service inventory.EntityID, port int,
) []target {
	entity, ok := r.Entity(service)
	if !ok {
		return []target{{proto: "TCP"}}
	}
	var out []target
	for _, entry := range strings.Split(entityText(entity, AttrPorts), ",") {
		svcPort, rest, found := strings.Cut(entry, "/")
		proto, to, found2 := strings.Cut(rest, "->")
		number, err := strconv.Atoi(svcPort)
		if !found || !found2 || err != nil || (port != 0 && number != port) {
			continue
		}
		targetPort, _ := strconv.Atoi(to)
		out = append(out, target{port: targetPort, proto: proto})
	}
	if len(out) == 0 {
		return []target{{port: port, proto: "TCP"}}
	}
	return out
}

// podsBlocked checks one caller and one backend pod over every target.
func podsBlocked(
	r inventory.Reader, from, to podFacts, targets []target,
) (Block, bool) {
	var first Block
	for i, t := range targets {
		block, blocked := sideBlocked(r, from, to, t, true)
		if !blocked {
			block, blocked = sideBlocked(r, to, from, t, false)
		}
		if !blocked {
			return Block{}, false
		}
		if i == 0 {
			first = block
		}
	}
	return first, true
}

// policy is a NetworkPolicy that selects a pod, with its rules.
type policy struct {
	id    inventory.EntityID
	rules PolicyRules
}

// sideBlocked checks one direction of a pair: pod's own policies
// against the other pod. egress is true for the caller's side.
func sideBlocked(
	r inventory.Reader, pod, other podFacts, t target, egress bool,
) (Block, bool) {
	selecting := policiesSelecting(r, pod, egress)
	if len(selecting) == 0 {
		return Block{}, false
	}
	ids := make([]inventory.EntityID, 0, len(selecting))
	for _, p := range selecting {
		rules := p.rules.In
		if egress {
			rules = p.rules.Out
		}
		for _, rule := range rules {
			if ruleAllows(rule, p.id.Namespace, other, t) {
				return Block{}, false
			}
		}
		ids = append(ids, p.id)
	}
	return Block{Policies: ids, Egress: egress, Pod: pod.id}, true
}

// policiesSelecting lists the policies of the pod's namespace that
// restrict the direction for pod, sorted, at most maxPoliciesPerPod.
// A policy whose rules cannot be read is skipped, which leaves the pod
// open rather than blaming a policy kwatch cannot see.
func policiesSelecting(
	r inventory.Reader, pod podFacts, egress bool,
) []policy {
	var out []policy
	for _, id := range r.EntitiesIn(KindNetworkPolicy, pod.id.Namespace) {
		entity, ok := r.Entity(id)
		if !ok {
			continue
		}
		rules, ok := ParsePolicyRules(entityText(entity, AttrPolicyRules))
		if !ok || (egress && !rules.Egress) || (!egress && !rules.Ingress) ||
			!selectsPod(entityText(entity, AttrSelector), pod.labels) {
			continue
		}
		out = append(out, policy{id: id, rules: rules})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].id.Name < out[j].id.Name
	})
	if len(out) > maxPoliciesPerPod {
		out = out[:maxPoliciesPerPod]
	}
	return out
}

// selectsPod reports a policy pod selector matching the labels. "<none>"
// is the empty selector, which selects every pod.
func selectsPod(text string, set labels.Set) bool {
	if text == "<none>" || text == "" {
		return true
	}
	selector, err := labels.Parse(text)
	return err != nil || selector.Matches(set)
}
