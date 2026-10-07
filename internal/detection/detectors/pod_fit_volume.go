package detectors

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// label names the claim and, once bound, its volume.
func (c fitClaim) label() string {
	if c.volume == "" {
		return "claim " + c.name
	}
	return "claim " + c.name + " (volume " + c.volume + ")"
}

// pinnedNode is the node a volume is tied to by hostname, as local
// volumes are. It is empty for any other kind of limit.
func (c fitClaim) pinnedNode() string {
	if len(c.terms) != 1 || len(c.terms[0]) != 1 {
		return ""
	}
	r := c.terms[0][0]
	if r.Key != "kubernetes.io/hostname" || r.Op != "In" ||
		len(r.Values) != 1 {
		return ""
	}
	return r.Values[0]
}

// nowhere words a claim whose limit no node meets.
func (c fitClaim) nowhere() string {
	if node := c.pinnedNode(); node != "" {
		return c.label() + " lives on node " + node + ", which is gone"
	}
	return c.label() + " can only attach where " +
		requirementText(c.terms[0]) + ", and no node is there"
}

// volumeNodes explains, for each claim that some node can serve, why
// the nodes it can use still cannot take the pod: the reuse of the
// node checks, worded for the volume. It also returns those nodes so
// the pool lines do not repeat them.
func (c *fitCase) volumeNodes(
	results []nodeVerdict,
) ([]detection.Evidence, map[inventory.EntityID]bool) {
	var out []detection.Evidence
	used := map[inventory.EntityID]bool{}
	for _, claim := range c.claims {
		var usable []nodeVerdict
		for _, r := range results {
			if r.node.labelsOK &&
				anyTermMatches(claim.terms, r.node.labels) {
				usable = append(usable, r)
				used[r.node.id] = true
			}
		}
		if len(usable) == 0 {
			continue
		}
		out = append(out, detection.Evidence{Label: detection.EvidenceFit,
			Value: claim.usableText(usable)})
	}
	return out, used
}

// usableText says why the nodes a volume can attach to cannot take
// the pod.
func (c fitClaim) usableText(usable []nodeVerdict) string {
	best := usable[0]
	for _, r := range usable[1:] {
		if nearer(r.blockers, best.blockers) {
			best = r
		}
	}
	texts := make([]string, 0, len(best.blockers))
	for _, b := range best.blockers {
		texts = append(texts, b.text)
	}
	why := strings.Join(texts, " and ")
	if node := c.pinnedNode(); node != "" {
		return c.label() + " lives on node " + node + ", which " + why
	}
	which := "the only node there, "
	if len(usable) > 1 {
		which = "the closest of " + strconv.Itoa(len(usable)) +
			" nodes there, "
	}
	return c.label() + " can only attach where " +
		requirementText(c.terms[0]) + "; " + which +
		best.node.id.Name + ", " + why
}

// withoutNodes drops the pools whose nearest node is one of nodes.
func withoutNodes(
	groups []groupBest, nodes map[inventory.EntityID]bool,
) []groupBest {
	var out []groupBest
	for _, g := range groups {
		if !nodes[g.best.node.id] {
			out = append(out, g)
		}
	}
	return out
}
