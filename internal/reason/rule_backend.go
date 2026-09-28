package reason

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// BackendRule explains a Service without ready endpoints by the workload
// whose pods back it. The workload's own problem then carries the deeper
// cause; the Service becomes part of its impact.
type BackendRule struct{}

// Name implements Rule.
func (BackendRule) Name() string { return "backend" }

// Explain implements Rule.
func (BackendRule) Explain(q Query, symptom signal.Signal) []Hypothesis {
	if symptom.Entity.Kind != kube.KindService {
		return nil
	}
	owners := map[knowledge.EntityID][]knowledge.EntityID{}
	var order []knowledge.EntityID
	for _, slice := range q.Model.Related(
		symptom.Entity, knowledge.Backs, knowledge.Incoming,
	) {
		for _, pod := range q.Model.Related(
			slice, knowledge.RoutesTo, knowledge.Outgoing,
		) {
			owner := TopOwner(q.Model, pod)
			if owners[owner] == nil {
				order = append(order, owner)
			}
			owners[owner] = append(owners[owner], pod)
		}
	}
	var out []Hypothesis
	for _, owner := range order {
		pods := owners[owner]
		failing, total := failingShare(q, pods)
		if failing == 0 {
			continue
		}
		s := newScorer(0.4)
		s.support(0.35, fmt.Sprintf("%d of %d backend pods are failing",
			failing, total))
		score, points := s.result()
		out = append(out, Hypothesis{
			Root:  owner,
			Chain: []knowledge.EntityID{owner, symptom.Entity},
			Summary: string(owner.Kind) + " " + owner.Name +
				" has no ready pods",
			Points: points, Score: score,
		})
	}
	return out
}
