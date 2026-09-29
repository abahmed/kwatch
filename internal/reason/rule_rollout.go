package reason

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// RolloutWindow is how long after a rollout a failure is attributed to it.
const RolloutWindow = 15 * time.Minute

// RolloutRule blames a recent pod-template change of the owning workload.
// Only template changes are rollouts; replica and metadata edits are not.
type RolloutRule struct{}

// Name implements Rule.
func (RolloutRule) Name() string { return "rollout" }

// Explain implements Rule.
func (RolloutRule) Explain(q Query, symptom signal.Signal) []Hypothesis {
	pod, ok := PodOf(q.Model, symptom.Entity)
	if !ok {
		return nil
	}
	for _, owner := range ownerChain(q.Model, pod) {
		change, ok := latestRollout(q, owner, symptom.Since)
		if !ok {
			continue
		}
		return []Hypothesis{rolloutHypothesis(q, pod, owner, change,
			symptom)}
	}
	return nil
}

func latestRollout(
	q Query, owner knowledge.EntityID, before time.Time,
) (knowledge.Change, bool) {
	changes := q.Model.Changes(owner, before.Add(-RolloutWindow))
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		if change.At.After(before) {
			continue
		}
		if isRollout(change) {
			return change, true
		}
	}
	return knowledge.Change{}, false
}

func isRollout(change knowledge.Change) bool {
	for _, field := range change.Fields {
		if strings.HasPrefix(field.Path, "containers[") ||
			strings.HasPrefix(field.Path, "template.") ||
			field.Path == "spec.template" {
			return true
		}
	}
	return false
}

func rolloutHypothesis(
	q Query, pod, owner knowledge.EntityID,
	change knowledge.Change, symptom signal.Signal,
) Hypothesis {
	s := newScorer(0.3)
	gap := symptom.Since.Sub(change.At)
	switch {
	case gap < time.Minute:
		s.support(0.25, "the failure started right after the rollout")
	case gap <= 5*time.Minute:
		s.support(0.25, "the failure started "+short(gap)+" after the "+
			"rollout")
	default:
		s.support(0.1, "the failure started within the rollout window")
	}
	if newRevisionOnly(q, pod) {
		s.support(0.3, "only pods of the new revision fail")
	} else if failingElsewhereOldRevision(q, pod) {
		s.contradict(0.3, "pods of the previous revision fail too")
	}
	if mentionsChange(symptom, change) {
		s.support(0.2, "the error mentions what the rollout changed")
	}
	companions := companionChanges(q, owner, change)
	if len(companions) > 0 {
		s.support(0.05, "its configuration changed in the same release")
	}
	score, points := s.result()
	c := change
	return Hypothesis{
		Root: owner, Change: &c,
		Chain: []knowledge.EntityID{owner, pod},
		Summary: "the rollout that " + describeFields(change) +
			describeCompanions(companions),
		Points: points, Score: score,
	}
}

// newRevisionOnly reports whether the failing pod belongs to the newest
// ReplicaSet while pods of older ReplicaSets are healthy.
func newRevisionOnly(q Query, pod knowledge.EntityID) bool {
	chain := ownerChain(q.Model, pod)
	if len(chain) < 2 || chain[0].Kind != kube.KindReplicaSet {
		return false
	}
	current, sawOld := chain[0], false
	for _, rs := range q.Model.Related(
		chain[1], knowledge.OwnedBy, knowledge.Incoming,
	) {
		if rs == current {
			continue
		}
		for _, old := range q.Model.Related(
			rs, knowledge.OwnedBy, knowledge.Incoming,
		) {
			if podFailing(q, old) {
				return false
			}
			sawOld = true
		}
	}
	return sawOld
}

func failingElsewhereOldRevision(q Query, pod knowledge.EntityID) bool {
	chain := ownerChain(q.Model, pod)
	if len(chain) < 2 {
		return false
	}
	for _, rs := range q.Model.Related(
		chain[1], knowledge.OwnedBy, knowledge.Incoming,
	) {
		if rs == chain[0] {
			continue
		}
		failing, _ := failingShare(q, q.Model.Related(
			rs, knowledge.OwnedBy, knowledge.Incoming))
		if failing > 0 {
			return true
		}
	}
	return false
}

func mentionsChange(symptom signal.Signal, change knowledge.Change) bool {
	text := strings.ToLower(evidenceText(symptom))
	for _, field := range change.Fields {
		tokens := []string{field.After, lastSegment(field.Path)}
		for _, token := range tokens {
			if len(token) >= 4 &&
				strings.Contains(text, strings.ToLower(token)) {
				return true
			}
		}
	}
	return false
}
