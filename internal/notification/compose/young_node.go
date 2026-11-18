package compose

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
)

// weightYoungNode ranks the node-age fact with the other placement facts.
const weightYoungNode = 0.35

// youngNodeSentences say that the failing pods began failing minutes
// after their node joined: "The failing pods run on node n3, which
// joined 2 minutes before they began failing." Pods on a node that
// just joined can fail while its network or DNS is still being set up;
// the fact is said, not concluded from.
func youngNodeSentences(f caseFacts) []sentence {
	minutes := map[string]int{}
	for _, m := range f.members {
		node, after, ok := youngNodeOf(m)
		if !ok {
			continue
		}
		if seen, known := minutes[node]; !known || after < seen {
			minutes[node] = after
		}
	}
	if len(minutes) == 0 {
		return nil
	}
	nodes := make([]string, 0, len(minutes))
	least := -1
	for node, after := range minutes {
		nodes = append(nodes, node)
		if least < 0 || after < least {
			least = after
		}
	}
	sort.Strings(nodes)
	which, verb := "node "+nodes[0]+", which", "joined"
	if len(nodes) > 1 {
		which = "nodes " + joinAnd(nodes, 3) + ", which"
	}
	return []sentence{proof(weightYoungNode, "The failing pods run on "+
		which+" "+verb+" "+minutesWords(time.Duration(least)*time.Minute)+
		" before they began failing.")}
}

// youngNodeOf reads the node-age evidence: the node and the minutes.
func youngNodeOf(m detection.Finding) (string, int, bool) {
	node, raw, ok := strings.Cut(evidence(m, detection.EvidenceYoungNode),
		" ")
	after, err := strconv.Atoi(raw)
	return node, after, ok && err == nil && node != ""
}
