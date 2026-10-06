package detectors

import (
	"sort"
	"strconv"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// placementBlockers checks the pod's required pod affinity, required
// pod anti-affinity and topology spread. All three are best-effort:
// they count the pods kwatch knows on the nodes it checks, and spread
// ignores which nodes the pod is eligible for.
func (c *fitCase) placementBlockers(n fitNode) []blocker {
	var out []blocker
	for _, term := range c.spec.Anti {
		if peer, ok := c.matchInDomain(term, n); ok {
			out = append(out, blocker{kind: blockPlacement,
				text: "already runs " + peer + " in its " +
					term.TopologyKey + ", which the pod's anti-affinity " +
					"refuses"})
		}
	}
	for _, term := range c.spec.Affinity {
		if c.affinityMissing(term, n) {
			out = append(out, blocker{kind: blockPlacement,
				text: "has no pod in its " + term.TopologyKey +
					" that the pod's affinity requires"})
		}
	}
	for _, spread := range c.spec.Spread {
		if text := c.spreadBreak(spread, n); text != "" {
			out = append(out, blocker{kind: blockPlacement, text: text})
		}
	}
	return out
}

// peerMatches reports whether a pod on a node is one the term names.
func (c *fitCase) peerMatches(term kube.PodTerm, p fitPeer) bool {
	if !term.Any {
		namespaces := term.Namespaces
		if len(namespaces) == 0 {
			namespaces = []string{c.pod.Namespace}
		}
		if !contains(namespaces, p.namespace) {
			return false
		}
	}
	return len(term.Selector) > 0 && kube.MatchesAll(term.Selector, p.labels)
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// matchInDomain finds a pod the term names in the node's topology
// domain. A node without the topology label has no domain.
func (c *fitCase) matchInDomain(
	term kube.PodTerm, n fitNode,
) (string, bool) {
	domain, ok := n.labels[term.TopologyKey]
	if !ok {
		return "", false
	}
	for _, other := range c.nodes {
		if other.labels[term.TopologyKey] != domain {
			continue
		}
		for _, p := range c.peers[other.id.Name] {
			if c.peerMatches(term, p) {
				return p.namespace + "/" + p.name, true
			}
		}
	}
	return "", false
}

// affinityMissing is true when the node's domain holds no pod the term
// names. The scheduler lets the first pod of a group start a domain, so
// nothing is missing when no such pod exists anywhere and the pod
// matches its own term.
func (c *fitCase) affinityMissing(term kube.PodTerm, n fitNode) bool {
	if _, ok := n.labels[term.TopologyKey]; !ok {
		return true
	}
	if _, found := c.matchInDomain(term, n); found {
		return false
	}
	for _, other := range c.nodes {
		for _, p := range c.peers[other.id.Name] {
			if c.peerMatches(term, p) {
				return true
			}
		}
	}
	selfMatch := kube.MatchesAll(term.Selector, c.labels)
	return !selfMatch
}

// spreadBreak returns why placing the pod on the node would leave the
// matching pods more than maxSkew apart, or "".
func (c *fitCase) spreadBreak(s kube.Spread, n fitNode) string {
	domain, ok := n.labels[s.TopologyKey]
	if !ok {
		return "has no " + s.TopologyKey + " label the pod spreads by"
	}
	counts := map[string]int{}
	for _, other := range c.nodes {
		value, has := other.labels[s.TopologyKey]
		if !has {
			continue
		}
		counts[value] += 0
		for _, p := range c.peers[other.id.Name] {
			if p.namespace == c.pod.Namespace &&
				kube.MatchesAll(s.Selector, p.labels) {
				counts[value]++
			}
		}
	}
	lowest := lowestCount(counts, s.MinDomains)
	if counts[domain]+1-lowest <= s.MaxSkew {
		return ""
	}
	return "already holds " + strconv.Itoa(counts[domain]) +
		" matching pods in " + s.TopologyKey + " " + domain +
		" while the emptiest has " + strconv.Itoa(lowest) +
		" (max skew " + strconv.Itoa(s.MaxSkew) + ")"
}

// lowestCount is the smallest pod count over the domains; with fewer
// domains than minDomains the scheduler counts the minimum as zero.
func lowestCount(counts map[string]int, minDomains int) int {
	if len(counts) < minDomains {
		return 0
	}
	values := make([]int, 0, len(counts))
	for _, v := range counts {
		values = append(values, v)
	}
	sort.Ints(values)
	return values[0]
}
