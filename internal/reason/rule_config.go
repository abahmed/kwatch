package reason

import (
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// ConfigWindow is how long after a Secret or ConfigMap change a consumer
// failure is attributed to it.
const ConfigWindow = 30 * time.Minute

// ConfigRule blames a recent change to a Secret or ConfigMap the failing
// pod references. Values are never compared; changed key names are.
type ConfigRule struct{}

// Name implements Rule.
func (ConfigRule) Name() string { return "config-change" }

// Explain implements Rule.
func (ConfigRule) Explain(q Query, symptom signal.Signal) []Hypothesis {
	pod, ok := PodOf(q.Model, symptom.Entity)
	if !ok {
		return nil
	}
	var out []Hypothesis
	for _, ref := range q.Model.Related(
		pod, knowledge.References, knowledge.Outgoing,
	) {
		if ref.Kind != kube.KindSecret && ref.Kind != kube.KindConfigMap {
			continue
		}
		change, ok := latestChange(q, ref, symptom.Since, ConfigWindow)
		if !ok {
			continue
		}
		out = append(out, configHypothesis(q, pod, ref, change, symptom))
	}
	return out
}

func latestChange(
	q Query, id knowledge.EntityID, before time.Time, window time.Duration,
) (knowledge.Change, bool) {
	changes := q.Model.Changes(id, before.Add(-window))
	for i := len(changes) - 1; i >= 0; i-- {
		if !changes[i].At.After(before) && !changes[i].Created {
			return changes[i], true
		}
	}
	return knowledge.Change{}, false
}

func configHypothesis(
	q Query, pod, ref knowledge.EntityID,
	change knowledge.Change, symptom signal.Signal,
) Hypothesis {
	s := newScorer(0.25)
	s.support(0.15, "it changed "+short(symptom.Since.Sub(change.At))+
		" before the failure")
	if change.Deleted {
		s.support(0.35, "it was deleted")
	}
	if mentionsChange(symptom, change) {
		s.support(0.35, "the error names a changed key")
	}
	consumers := q.Model.Related(ref, knowledge.References,
		knowledge.Incoming)
	failing, total := failingShare(q, podsAmong(consumers))
	if total > 1 && failing == total {
		s.support(0.15, "every pod using it is failing")
	} else if total > 1 && failing*2 < total {
		s.contradict(0.2, "most pods using it are healthy")
	}
	score, points := s.result()
	c := change
	return Hypothesis{
		Root: ref, Change: &c,
		Chain: []knowledge.EntityID{ref, pod},
		Summary: string(ref.Kind) + " " + ref.Name + " " +
			describeFields(change),
		Points: points, Score: score,
	}
}

func podsAmong(ids []knowledge.EntityID) []knowledge.EntityID {
	var pods []knowledge.EntityID
	for _, id := range ids {
		if id.Kind == kube.KindPod {
			pods = append(pods, id)
		}
	}
	return pods
}
