package reason

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/signal"
)

// maxTopologyPods bounds the pods sampled per zone or pool, keeping the
// rule proportional to the failure rather than to the cluster.
const maxTopologyPods = 500

// TopologyRule blames a zone or node pool when failures span several of
// its nodes while replicas elsewhere are healthy: a zone outage or a bad
// node image, not one broken node.
type TopologyRule struct{}

// Name implements Rule.
func (TopologyRule) Name() string { return "topology" }

// Explain implements Rule.
func (TopologyRule) Explain(q Query, symptom signal.Signal) []Hypothesis {
	pod, ok := PodOf(q.Model, symptom.Entity)
	if !ok {
		return nil
	}
	nodes := q.Model.Related(pod, knowledge.RunsOn, knowledge.Outgoing)
	if len(nodes) == 0 {
		return nil
	}
	var out []Hypothesis
	for _, group := range q.Model.Related(
		nodes[0], knowledge.PartOf, knowledge.Outgoing,
	) {
		if h, ok := topologyHypothesis(q, pod, group); ok {
			out = append(out, h)
		}
	}
	return out
}

func topologyHypothesis(
	q Query, pod, group knowledge.EntityID,
) (Hypothesis, bool) {
	failingNodes, failing, total := 0, 0, 0
	for _, node := range q.Model.Related(
		group, knowledge.PartOf, knowledge.Incoming,
	) {
		pods := podsOnNode(q.Model, node)
		if total+len(pods) > maxTopologyPods {
			pods = pods[:max(0, maxTopologyPods-total)]
		}
		nodeFailing, nodeTotal := failingShare(q, pods)
		failing += nodeFailing
		total += nodeTotal
		if nodeFailing > 0 {
			failingNodes++
		}
	}
	if failingNodes < 2 || total == 0 || failing*2 < total {
		return Hypothesis{}, false
	}
	s := newScorer(0.3)
	s.support(0.2, fmt.Sprintf("pods fail on %d nodes of this %s",
		failingNodes, group.Kind))
	if siblingsOutside(q, pod, group) {
		s.support(0.25, "replicas outside it are healthy")
	}
	score, points := s.result()
	return Hypothesis{
		Root:  group,
		Chain: []knowledge.EntityID{group, pod},
		Summary: fmt.Sprintf("%s %s: %d of %d pods failing", group.Kind,
			group.Name, failing, total),
		Points: points, Score: score,
	}, true
}

// siblingsOutside reports whether the pod has replicas on nodes outside
// the group and all of them are healthy.
func siblingsOutside(q Query, pod, group knowledge.EntityID) bool {
	found := false
	for _, sibling := range siblingPods(q.Model, pod) {
		for _, node := range q.Model.Related(
			sibling, knowledge.RunsOn, knowledge.Outgoing,
		) {
			inGroup := false
			for _, g := range q.Model.Related(
				node, knowledge.PartOf, knowledge.Outgoing,
			) {
				inGroup = inGroup || g == group
			}
			if inGroup {
				continue
			}
			if podFailing(q, sibling) {
				return false
			}
			found = true
		}
	}
	return found
}
