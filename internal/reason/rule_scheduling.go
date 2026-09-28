package reason

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/signal"
)

// KindScheduling is the virtual entity for a cluster-wide scheduling
// constraint, named after the dominant blocker ("Insufficient memory").
const KindScheduling knowledge.Kind = "scheduling"

// schedulerMessage matches "0/20 nodes are available: 12 Insufficient
// memory, 3 node(s) had untolerated taint {...}."
var (
	schedulerTotal  = regexp.MustCompile(`0/(\d+) nodes are available`)
	schedulerReason = regexp.MustCompile(`(\d+) ([^,.]+)`)
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
	total := 0
	if m := schedulerTotal.FindStringSubmatch(head); m != nil {
		total, _ = strconv.Atoi(m[1])
	}
	var blockers []Blocker
	for _, part := range strings.Split(tail, ",") {
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

// SchedulingRule explains unschedulable pods by the constraint that
// rejects most nodes.
type SchedulingRule struct{}

// Name implements Rule.
func (SchedulingRule) Name() string { return "scheduling" }

// Explain implements Rule.
func (SchedulingRule) Explain(_ Query, symptom signal.Signal) []Hypothesis {
	if symptom.Reason != constant.ReasonUnschedulable {
		return nil
	}
	blockers, total := ParseSchedulerMessage(evidenceValue(symptom,
		"scheduler"))
	if len(blockers) == 0 {
		return nil
	}
	top := blockers[0]
	s := newScorer(0.5)
	s.support(0.25, fmt.Sprintf("the scheduler rejected %d of %d nodes "+
		"for this reason", top.Nodes, total))
	score, points := s.result()
	return []Hypothesis{{
		Root: knowledge.NewEntityID(KindScheduling, "", top.Reason),
		Chain: []knowledge.EntityID{
			knowledge.NewEntityID(KindScheduling, "", top.Reason),
			symptom.Entity,
		},
		Summary: fmt.Sprintf("%s on %d of %d nodes", top.Reason,
			top.Nodes, total),
		Points: points, Score: score,
	}}
}
