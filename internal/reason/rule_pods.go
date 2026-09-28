package reason

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// PodsRule explains a controller-level symptom (unavailable replicas, a
// stuck rollout) by its failing pods, so the pods' own cause is found
// next. A workload with no failing pod is not explained here.
type PodsRule struct{}

// Name implements Rule.
func (PodsRule) Name() string { return "failing-pods" }

// Explain implements Rule.
func (PodsRule) Explain(q Query, symptom signal.Signal) []Hypothesis {
	if !controllerKinds[symptom.Entity.Kind] {
		return nil
	}
	pods := OwnedPods(q.Model, symptom.Entity)
	var failingPod knowledge.EntityID
	var podSignals []signal.Signal
	failing := 0
	for _, pod := range pods {
		active := podSignalsOf(q, pod)
		if len(active) == 0 {
			continue
		}
		failing++
		if podSignals == nil {
			failingPod, podSignals = pod, active
		}
	}
	if failing == 0 {
		return nil
	}
	s := newScorer(0.45)
	s.support(0.25, fmt.Sprintf("%d of %d pods are failing",
		failing, len(pods)))
	score, points := s.result()
	return []Hypothesis{{
		Root: failingPod, RootSignals: podSignals,
		Chain:   []knowledge.EntityID{failingPod, symptom.Entity},
		Summary: podSignals[0].Summary,
		Points:  points, Score: score,
	}}
}

// controllerKinds own pods through a template.
var controllerKinds = map[knowledge.Kind]bool{
	kube.KindDeployment: true, kube.KindReplicaSet: true,
	kube.KindStatefulSet: true, kube.KindDaemonSet: true,
	kube.KindJob: true,
}

// podSignalsOf returns the pod's signals, or its containers' when the pod
// itself has none.
func podSignalsOf(q Query, pod knowledge.EntityID) []signal.Signal {
	if active := q.Signals.Active(pod); len(active) > 0 {
		return active
	}
	var out []signal.Signal
	for _, container := range q.Model.Related(
		pod, knowledge.PartOf, knowledge.Incoming,
	) {
		out = append(out, q.Signals.Active(container)...)
	}
	return out
}
