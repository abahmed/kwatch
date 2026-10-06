package kube

import (
	"net"

	"k8s.io/apimachinery/pkg/labels"
)

// ruleAllows reports whether the rule lets the traffic between the
// policy's pod and other through, on target t.
func ruleAllows(
	rule PolicyRule, policyNamespace string, other podFacts, t target,
) bool {
	return peersAllow(rule.Peers, policyNamespace, other) &&
		portsAllow(rule.Ports, t)
}

// peersAllow: no peers means every peer.
func peersAllow(peers []PolicyPeer, namespace string, other podFacts) bool {
	if len(peers) == 0 {
		return true
	}
	for _, peer := range peers {
		if peerMatches(peer, namespace, other) {
			return true
		}
	}
	return false
}

func peerMatches(peer PolicyPeer, namespace string, other podFacts) bool {
	if peer.CIDR != "" {
		return cidrMatches(peer, other.ip)
	}
	if peer.HasNS {
		if !textMatches(peer.Namespaces, other.nsLabels) {
			return false
		}
	} else if other.id.Namespace != namespace {
		return false
	}
	return !peer.HasPods || textMatches(peer.Pods, other.labels)
}

// textMatches matches label-selector text; text kwatch cannot parse
// matches, so an unreadable rule never creates a denial.
func textMatches(text string, set labels.Set) bool {
	selector, err := labels.Parse(text)
	return err != nil || selector.Matches(set)
}

// cidrMatches reports ip inside the peer's range and outside its
// exceptions. A pod with no IP, or a range that does not parse, matches:
// it cannot be shown to be denied.
func cidrMatches(peer PolicyPeer, ip net.IP) bool {
	_, block, err := net.ParseCIDR(peer.CIDR)
	if ip == nil || err != nil {
		return true
	}
	if !block.Contains(ip) {
		return false
	}
	for _, except := range peer.Except {
		if _, skip, err := net.ParseCIDR(except); err == nil &&
			skip.Contains(ip) {
			return false
		}
	}
	return true
}

// portsAllow: no ports means every port. An unknown target port or a
// named rule port matches.
func portsAllow(ports []PolicyPort, t target) bool {
	if len(ports) == 0 {
		return true
	}
	for _, p := range ports {
		if portMatches(p, t) {
			return true
		}
	}
	return false
}

func portMatches(p PolicyPort, t target) bool {
	if p.Proto != "" && t.proto != "" && p.Proto != t.proto {
		return false
	}
	if p.Name != "" || t.port == 0 || p.Port == 0 {
		return true
	}
	last := p.Port
	if p.End > last {
		last = p.End
	}
	return t.port >= p.Port && t.port <= last
}
