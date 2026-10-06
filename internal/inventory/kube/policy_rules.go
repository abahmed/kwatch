package kube

import (
	"encoding/json"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrPolicyRules is the part of a NetworkPolicy that decides traffic,
// as JSON (see PolicyRules). It is left out when it would exceed
// maxPolicyRulesSize: a policy kwatch cannot read whole proves nothing.
const AttrPolicyRules = "policy.rules"

// maxPolicyRulesSize bounds the stored rules of one policy.
const maxPolicyRulesSize = 8 << 10

// PolicyRules are the rules of one NetworkPolicy that traffic checks
// need: which directions it restricts and what it allows. Selectors are
// kept as label-selector text.
type PolicyRules struct {
	// Ingress and Egress say which directions the policy restricts for
	// the pods it selects (spec.policyTypes, defaulted as Kubernetes
	// does).
	Ingress bool         `json:"i,omitempty"`
	Egress  bool         `json:"e,omitempty"`
	In      []PolicyRule `json:"in,omitempty"`
	Out     []PolicyRule `json:"out,omitempty"`
}

// PolicyRule allows traffic to or from the peers on the ports. No peers
// means every peer; no ports means every port.
type PolicyRule struct {
	Peers []PolicyPeer `json:"p,omitempty"`
	Ports []PolicyPort `json:"o,omitempty"`
}

// PolicyPeer is one peer of a rule: pods, namespaces or an address
// range. A peer that names only namespaces means every pod in them; one
// that names only pods means pods of the policy's own namespace.
type PolicyPeer struct {
	Pods       string   `json:"pods,omitempty"`
	HasPods    bool     `json:"hp,omitempty"`
	Namespaces string   `json:"ns,omitempty"`
	HasNS      bool     `json:"hn,omitempty"`
	CIDR       string   `json:"cidr,omitempty"`
	Except     []string `json:"x,omitempty"`
}

// PolicyPort is one allowed port or range. A named port has Name set
// and Port zero.
type PolicyPort struct {
	Proto string `json:"t,omitempty"`
	Port  int    `json:"n,omitempty"`
	End   int    `json:"end,omitempty"`
	Name  string `json:"name,omitempty"`
}

// policyRulesOf reads the rules of a NetworkPolicy.
func policyRulesOf(np *networkingv1.NetworkPolicy) PolicyRules {
	rules := PolicyRules{}
	if len(np.Spec.PolicyTypes) == 0 {
		// Kubernetes restricts ingress always, egress only when the
		// policy has egress rules.
		rules.Ingress, rules.Egress = true, len(np.Spec.Egress) > 0
	}
	for _, t := range np.Spec.PolicyTypes {
		rules.Ingress = rules.Ingress || t == networkingv1.PolicyTypeIngress
		rules.Egress = rules.Egress || t == networkingv1.PolicyTypeEgress
	}
	for _, rule := range np.Spec.Ingress {
		rules.In = append(rules.In, PolicyRule{
			Peers: peersOf(rule.From), Ports: portsOf(rule.Ports)})
	}
	for _, rule := range np.Spec.Egress {
		rules.Out = append(rules.Out, PolicyRule{
			Peers: peersOf(rule.To), Ports: portsOf(rule.Ports)})
	}
	return rules
}

func peersOf(peers []networkingv1.NetworkPolicyPeer) []PolicyPeer {
	out := make([]PolicyPeer, 0, len(peers))
	for _, peer := range peers {
		p := PolicyPeer{}
		if peer.PodSelector != nil {
			p.HasPods, p.Pods = true, selectorString(peer.PodSelector)
		}
		if peer.NamespaceSelector != nil {
			p.HasNS = true
			p.Namespaces = selectorString(peer.NamespaceSelector)
		}
		if block := peer.IPBlock; block != nil {
			p.CIDR, p.Except = block.CIDR, block.Except
		}
		out = append(out, p)
	}
	return out
}

// selectorString is the label-selector text of sel; "" selects all.
func selectorString(sel *metav1.LabelSelector) string {
	selector, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return "invalid selector"
	}
	return selector.String()
}

func portsOf(ports []networkingv1.NetworkPolicyPort) []PolicyPort {
	out := make([]PolicyPort, 0, len(ports))
	for _, port := range ports {
		p := PolicyPort{Proto: "TCP"}
		if port.Protocol != nil {
			p.Proto = string(*port.Protocol)
		}
		if port.Port != nil {
			p.Port, p.Name = port.Port.IntValue(), port.Port.StrVal
			if p.Name != "" {
				p.Port = 0
			}
		}
		if port.EndPort != nil {
			p.End = int(*port.EndPort)
		}
		out = append(out, p)
	}
	return out
}

// policyRulesValue is the attribute value for np, and false when the
// rules are too large to keep.
func policyRulesValue(np *networkingv1.NetworkPolicy) (inventory.Value, bool) {
	data, err := json.Marshal(policyRulesOf(np))
	if err != nil || len(data) > maxPolicyRulesSize {
		return inventory.Value{}, false
	}
	return inventory.Text(string(data)), true
}

// ParsePolicyRules reads the AttrPolicyRules value.
func ParsePolicyRules(text string) (PolicyRules, bool) {
	var rules PolicyRules
	if text == "" || json.Unmarshal([]byte(text), &rules) != nil {
		return PolicyRules{}, false
	}
	return rules, true
}
