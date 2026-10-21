package compose

import (
	"sort"
	"strconv"
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

// nodesLabel names the failing nodes of a grouped outage for a
// sentence that is about all of them: "nodes n1–n5" when their names
// count up from one number, else "nodes n1, n2 and n3".
func nodesLabel(f caseFacts) string {
	seen := map[string]bool{}
	var names []string
	for _, m := range failing(f.members) {
		if m.Entity.Kind == kube.KindNode && !seen[m.Entity.Name] {
			seen[m.Entity.Name] = true
			names = append(names, m.Entity.Name)
		}
	}
	if span, ok := numberedSpan(names); ok {
		return "nodes " + span
	}
	sort.Strings(names)
	return "nodes " + joinAnd(names, maxNodeNames)
}

// numberedSpan words names that share a prefix and count up by one as a
// range: "n1–n5". Fewer than three names are listed, not spanned.
func numberedSpan(names []string) (string, bool) {
	if len(names) < 3 {
		return "", false
	}
	prefix := strings.TrimRight(names[0], "0123456789")
	numbers := make([]int, 0, len(names))
	for _, name := range names {
		digits, ok := strings.CutPrefix(name, prefix)
		n, err := strconv.Atoi(digits)
		if !ok || err != nil || digits == "" {
			return "", false
		}
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)
	for i := 1; i < len(numbers); i++ {
		if numbers[i] != numbers[i-1]+1 {
			return "", false
		}
	}
	return prefix + strconv.Itoa(numbers[0]) + "–" + prefix +
		strconv.Itoa(numbers[len(numbers)-1]), true
}
