package reason

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

type harness struct {
	t       *testing.T
	model   *knowledge.Model
	signals fakeSignals
	now     time.Time
}

func newHarness(t *testing.T, now time.Time) *harness {
	return &harness{
		t:       t,
		model:   knowledge.NewModel(knowledge.Options{}),
		signals: make(fakeSignals),
		now:     now,
	}
}

func (h *harness) add(schema kube.Schema, objects ...any) {
	tr := kube.NewTranslator(schema)
	for _, obj := range objects {
		for _, f := range tr.Added(obj, true, h.now) {
			h.model.Apply(f)
		}
	}
}

func (h *harness) query() Query {
	return Query{Model: h.model, Signals: h.signals, Now: h.now}
}

type fakeSignals map[knowledge.EntityID][]signal.Signal

func (f fakeSignals) Active(id knowledge.EntityID) []signal.Signal {
	return f[id]
}

func (f fakeSignals) Has(id knowledge.EntityID) bool {
	return len(f[id]) > 0
}

func (h *harness) signal(
	entity knowledge.EntityID, reason string, since time.Time,
	sev signal.Severity,
) signal.Signal {
	s := signal.Signal{
		Entity: entity, Reason: reason, Since: since,
		Severity: sev, Summary: reason,
	}
	h.signals[entity] = append(h.signals[entity], s)
	return s
}

func node(name string, since time.Time) *corev1.Node {
	conds := []corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
	}
	if !since.IsZero() {
		conds = append(conds, corev1.NodeCondition{
			Type:               corev1.NodeMemoryPressure,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(since),
		})
	}
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name,
			UID: types.UID(name)},
		Status: corev1.NodeStatus{Conditions: conds},
	}
}

func pod(
	name, ns, owner, nodeName string, ready bool, since time.Time,
) *corev1.Pod {
	yes := true
	status := corev1.ConditionTrue
	if !ready {
		status = corev1.ConditionFalse
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns,
			UID: types.UID(name),
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "ReplicaSet", Name: owner, Controller: &yes,
			}}},
		Spec: corev1.PodSpec{NodeName: nodeName,
			Containers: []corev1.Container{
				{Name: "app", Image: "v1"},
			}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type: corev1.PodReady, Status: status,
				LastTransitionTime: metav1.NewTime(since),
			}}},
	}
}

func deployment(name, ns string) *appsv1.Deployment {
	replicas := int32(2)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name,
			Namespace: ns, UID: types.UID(name)},
		Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
	}
}

func replicaset(name, ns, owner string) *appsv1.ReplicaSet {
	replicas := int32(2)
	yes := true
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: name,
			Namespace: ns, UID: types.UID(name),
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "Deployment", Name: owner, Controller: &yes,
			}}},
		Spec: appsv1.ReplicaSetSpec{Replicas: &replicas},
	}
}
