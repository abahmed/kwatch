package reason

import (
	"time"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// PolicyWindow is how long after a NetworkPolicy change failures of the
// pods it selects are attributed to it.
const PolicyWindow = 15 * time.Minute

// NetworkPolicyRule blames a NetworkPolicy created or changed shortly
// before pods it selects started failing, when pods it does not select in
// the same namespace are fine.
type NetworkPolicyRule struct{}

// Name implements Rule.
func (NetworkPolicyRule) Name() string { return "network-policy" }

// Explain implements Rule.
func (NetworkPolicyRule) Explain(q Query, symptom signal.Signal) []Hypothesis {
	pod, ok := PodOf(q.Model, symptom.Entity)
	if !ok {
		return nil
	}
	podEntity, ok := q.Model.Entity(pod)
	if !ok {
		return nil
	}
	podLabels, err := labels.ConvertSelectorToLabelsMap(
		attributeText(podEntity, kube.AttrLabels))
	if err != nil {
		return nil
	}
	var out []Hypothesis
	for _, policy := range q.Model.Entities(kube.KindNetworkPolicy) {
		if policy.Namespace != pod.Namespace {
			continue
		}
		change, ok := recentChange(q, policy, symptom.Since, PolicyWindow)
		if !ok || !selects(q, policy, podLabels) {
			continue
		}
		s := newScorer(0.35)
		s.support(0.2, "the policy changed "+
			short(symptom.Since.Sub(change.At))+" before the failure")
		s.support(0.1, "it selects the failing pod")
		score, points := s.result()
		c := change
		out = append(out, Hypothesis{
			Root: policy, Change: &c,
			Chain: []knowledge.EntityID{
				policy, TopOwner(q.Model, pod), pod,
			},
			Summary: "network policy " + policy.Name + " " +
				describeFields(change),
			Points: points, Score: score,
		})
	}
	return out
}

// recentChange includes creation: a new policy can cut traffic too.
func recentChange(
	q Query, id knowledge.EntityID, before time.Time, window time.Duration,
) (knowledge.Change, bool) {
	changes := q.Model.Changes(id, before.Add(-window))
	for i := len(changes) - 1; i >= 0; i-- {
		if !changes[i].At.After(before) {
			return changes[i], true
		}
	}
	return knowledge.Change{}, false
}

func selects(q Query, policy knowledge.EntityID, pod labels.Set) bool {
	entity, ok := q.Model.Entity(policy)
	if !ok {
		return false
	}
	text := attributeText(entity, kube.AttrSelector)
	if text == "" || text == "<none>" {
		// An empty podSelector selects every pod in the namespace.
		return true
	}
	selector, err := labels.Parse(text)
	return err == nil && selector.Matches(pod)
}

func attributeText(e knowledge.Entity, name string) string {
	attribute, ok := e.Attribute(name)
	if !ok {
		return ""
	}
	return attribute.Value.AsText()
}
