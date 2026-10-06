package kube

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// schedulerMessage matches "0/20 nodes are available: 12 Insufficient
// memory, 3 node(s) had untolerated taint {...}."
var (
	schedulerTotal  = regexp.MustCompile(`0/(\d+) nodes are available`)
	schedulerReason = regexp.MustCompile(`^(\d+) (.+)$`)
)

// Blocker is one reason the scheduler rejected nodes.
type Blocker struct {
	Reason string
	Nodes  int
}

// ParseSchedulerMessage decodes a FailedScheduling message into blockers,
// most common first, and the node count.
func ParseSchedulerMessage(message string) ([]Blocker, int) {
	head, tail, found := strings.Cut(message, ":")
	if !found {
		return nil, 0
	}
	// The scheduler appends its preemption verdict, which repeats the node
	// count ("preemption: 0/5 nodes are available: 5 Preemption is not
	// helpful ..."); those are not scheduling blockers.
	if i := strings.Index(strings.ToLower(tail), "preemption:"); i >= 0 {
		tail = tail[:i]
	}
	total := 0
	if m := schedulerTotal.FindStringSubmatch(head); m != nil {
		total, _ = strconv.Atoi(m[1])
	}
	// Items are separated by ", ". Dots belong to the item (extended
	// resources such as nvidia.com/gpu, taint keys); only the one that
	// ends the message is dropped.
	tail = strings.TrimSuffix(strings.TrimSpace(tail), ".")
	var blockers []Blocker
	for _, part := range strings.Split(tail, ", ") {
		m := schedulerReason.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			continue
		}
		nodes, _ := strconv.Atoi(m[1])
		reason := strings.TrimSpace(strings.TrimPrefix(m[2], "node(s) "))
		if i := strings.Index(reason, " {"); i > 0 {
			reason = reason[:i]
		}
		blockers = append(blockers, Blocker{Reason: reason, Nodes: nodes})
	}
	sort.SliceStable(blockers, func(i, j int) bool {
		return blockers[i].Nodes > blockers[j].Nodes
	})
	return blockers, total
}
