package reason

import (
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// ReferenceRule blames a missing or failing object the pod depends on: a
// Secret, ConfigMap or ServiceAccount that does not exist, or a volume
// claim that cannot bind.
type ReferenceRule struct{}

// Name implements Rule.
func (ReferenceRule) Name() string { return "dependency" }

// Explain implements Rule.
func (ReferenceRule) Explain(q Query, symptom signal.Signal) []Hypothesis {
	pod, ok := PodOf(q.Model, symptom.Entity)
	if !ok {
		return nil
	}
	var out []Hypothesis
	for _, relation := range []knowledge.RelationType{
		knowledge.References, knowledge.Mounts,
	} {
		for _, dep := range q.Model.Related(pod, relation,
			knowledge.Outgoing) {
			if h, ok := dependencyHypothesis(q, pod, dep, symptom); ok {
				out = append(out, h)
			}
		}
	}
	return out
}

func dependencyHypothesis(
	q Query, pod, dep knowledge.EntityID, symptom signal.Signal,
) (Hypothesis, bool) {
	missing := dependencyKinds[dep.Kind] && !q.Model.Exists(dep)
	depSignals := q.Signals.Active(dep)
	if !missing && len(depSignals) == 0 {
		return Hypothesis{}, false
	}
	s := newScorer(0.4)
	summary := string(dep.Kind) + " " + dep.Name
	if missing {
		s.support(0.3, "it does not exist")
		summary += " does not exist"
	} else {
		s.support(0.2, depSignals[0].Summary)
		summary += ": " + depSignals[0].Summary
	}
	if mentionsName(symptom, dep.Name) {
		s.support(0.25, "the error names it")
	}
	score, points := s.result()
	return Hypothesis{
		Root: dep, RootSignals: depSignals,
		Chain:   []knowledge.EntityID{dep, pod},
		Summary: summary, Points: points, Score: score,
	}, true
}

// dependencyKinds are reference kinds whose absence breaks a pod.
var dependencyKinds = map[knowledge.Kind]bool{
	kube.KindSecret: true, kube.KindConfigMap: true,
	kube.KindAccount: true, kube.KindPVC: true,
}

func mentionsName(symptom signal.Signal, name string) bool {
	return len(name) >= 3 && containsFold(evidenceText(symptom), name)
}
