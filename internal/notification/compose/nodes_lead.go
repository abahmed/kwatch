package compose

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxNodeNames is how many failing nodes a lead names before it counts
// the rest.
const maxNodeNames = 3

// leadIsNodes reports an incident that speaks for several nodes that
// failed together: one outage, led by how many nodes are down.
func leadIsNodes(f caseFacts) bool {
	nodes, _ := failingNodes(f)
	return f.p.Grouped() && nodes > 1
}

// nodesLead says how many nodes are down and which, and what they report:
// "Three nodes (prod-eu-1) have not reported for about two minutes: n1,
// n2 and n3".
func nodesLead(f caseFacts) string {
	count, state := failingNodes(f)
	text := upperFirst(numberWord(count)) + " nodes" + f.clusterTag()
	if state != "" {
		text += " " + pluralPredicate(state)
	}
	return text + ": " + nodeNames(f)
}

// nodeNames lists the failing nodes, sorted, and counts the ones past
// maxNodeNames.
func nodeNames(f caseFacts) string {
	seen := map[string]bool{}
	for _, m := range failing(f.members) {
		if m.Entity.Kind == kube.KindNode {
			seen[m.Entity.Name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) <= maxNodeNames {
		return andList(strings.Join(names, ", "))
	}
	more := len(names) - maxNodeNames
	return strings.Join(names[:maxNodeNames], ", ") + " and " +
		numberWord(more) + " more"
}
