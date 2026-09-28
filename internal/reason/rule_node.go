package reason

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/signal"
)

// NodeRule blames an unhealthy node for failures of pods running on it.
// A healthy node is never proposed, however connected it is.
type NodeRule struct{}

// Name implements Rule.
func (NodeRule) Name() string { return "node" }

// Explain implements Rule.
func (NodeRule) Explain(q Query, symptom signal.Signal) []Hypothesis {
	pod, ok := PodOf(q.Model, symptom.Entity)
	if !ok {
		return nil
	}
	nodes := q.Model.Related(pod, knowledge.RunsOn, knowledge.Outgoing)
	if len(nodes) == 0 {
		return nil
	}
	node := nodes[0]
	nodeSignals := q.Signals.Active(node)
	if len(nodeSignals) == 0 {
		return nil
	}
	s := newScorer(0.35)
	first := earliest(nodeSignals)
	if !first.Since.After(symptom.Since) {
		s.support(0.2, "the node problem started before the pod failed")
	} else {
		s.contradict(0.15, "the pod failed before the node problem started")
	}
	failing, total := failingShare(q, podsOnNode(q.Model, node))
	if total > 0 && failing*10 >= total*3 {
		s.support(0.2, fmt.Sprintf("%d of %d pods on the node are failing",
			failing, total))
	}
	siblings := siblingPods(q.Model, pod)
	if healthyElsewhere(q, siblings, node) {
		s.support(0.2, "replicas of the same workload on other nodes are "+
			"healthy")
	} else if failingElsewhere(q, siblings, node) {
		s.contradict(0.3, "replicas on other nodes fail the same way")
	}
	score, points := s.result()
	return []Hypothesis{{
		Root: node, RootSignals: nodeSignals,
		Chain:   []knowledge.EntityID{node, pod},
		Summary: "node " + node.Name + ": " + first.Summary,
		Points:  points, Score: score,
	}}
}

func earliest(signals []signal.Signal) signal.Signal {
	first := signals[0]
	for _, s := range signals[1:] {
		if s.Since.Before(first.Since) {
			first = s
		}
	}
	return first
}

func healthyElsewhere(
	q Query, siblings []knowledge.EntityID, node knowledge.EntityID,
) bool {
	found := false
	for _, pod := range siblings {
		if onNode(q, pod, node) {
			continue
		}
		if podFailing(q, pod) {
			return false
		}
		found = true
	}
	return found
}

func failingElsewhere(
	q Query, siblings []knowledge.EntityID, node knowledge.EntityID,
) bool {
	for _, pod := range siblings {
		if !onNode(q, pod, node) && podFailing(q, pod) {
			return true
		}
	}
	return false
}

func onNode(q Query, pod, node knowledge.EntityID) bool {
	for _, n := range q.Model.Related(pod, knowledge.RunsOn,
		knowledge.Outgoing) {
		if n == node {
			return true
		}
	}
	return false
}
