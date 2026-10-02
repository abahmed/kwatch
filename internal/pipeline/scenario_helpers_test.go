package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/notification"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// harness drives the pipeline one step at a time with a manual clock.
type harness struct {
	t         testing.TB
	engine    *Engine
	checks    *rechecks
	now       time.Time
	decisions []incident.Decision
	messages  []notification.Message
}

func newHarness(t testing.TB, start time.Time) *harness {
	t.Helper()
	return newHarnessWith(t, start, nil)
}

// newHarnessWith lets a test adjust the dependencies before the engine is
// built.
func newHarnessWith(
	t testing.TB, start time.Time, mutate func(*Dependencies),
) *harness {
	t.Helper()
	h := &harness{t: t, checks: newRechecks(), now: start}
	deps := Dependencies{
		Model:     inventory.NewModel(inventory.Options{}),
		Detectors: detection.NewRegistry(nil, builtinDetectors()...),
		// A fixed nonce keeps incident IDs, and so every ordering by
		// ID, the same on every run.
		Incidents: incident.NewManager(
			incident.Config{IDNonce: harnessNonce}, explain.NewSolver()),
		Sink: func(_ context.Context, d incident.Decision, m notification.Message) {
			h.decisions = append(h.decisions, d)
			h.messages = append(h.messages, m)
		},
		Clock: fixedClock{h},
		// Background workers never see wall-clock time either: their
		// timers only fire when a test drives them.
		Timer: neverFires,
	}
	if mutate != nil {
		mutate(&deps)
	}
	engine, err := NewEngine(deps)
	if err != nil {
		t.Fatal(err)
	}
	h.engine = engine
	return h
}

// builtinDetectors is the production detector set.
func builtinDetectors() []detection.Detector {
	return detectors.Default()
}

// harnessNonce is the incident ID nonce of every harness.
const harnessNonce = "7e57"

// neverFires is a worker timer that never fires.
func neverFires(time.Duration) <-chan time.Time { return nil }

type fixedClock struct{ h *harness }

func (c fixedClock) Now() time.Time { return c.h.now }

func (c fixedClock) After(time.Duration) <-chan time.Time { return nil }

// add submits objects as the informers' initial list.
func (h *harness) add(schema kube.Schema, objects ...any) {
	translator := kube.NewTranslator(schema)
	for _, obj := range objects {
		h.engine.Submit(context.Background(),
			translator.Added(obj, true, h.now)...)
	}
}

// run advances the clock in steps until until, running the pipeline.
func (h *harness) run(until time.Time, step time.Duration) {
	for !h.now.After(until) {
		h.engine.step(context.Background(), h.now, h.checks)
		h.now = h.now.Add(step)
	}
}

// runUntil steps the pipeline until done reports true or the clock
// passes limit, and reports whether done became true.
func (h *harness) runUntil(
	done func() bool, limit time.Time, step time.Duration,
) bool {
	for !h.now.After(limit) {
		h.engine.step(context.Background(), h.now, h.checks)
		h.now = h.now.Add(step)
		if done() {
			return true
		}
	}
	return false
}

func node(name string, pressureSince time.Time) *corev1.Node {
	conditions := []corev1.NodeCondition{{
		Type: corev1.NodeReady, Status: corev1.ConditionTrue,
	}}
	if !pressureSince.IsZero() {
		conditions = append(conditions, corev1.NodeCondition{
			Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(pressureSince),
		})
	}
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(name)},
		Status:     corev1.NodeStatus{Conditions: conditions},
	}
}

func deployment(name string) (*appsv1.Deployment, *appsv1.ReplicaSet) {
	replicas := int32(2)
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "shop",
			UID: types.UID(name)},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 1, AvailableReplicas: 1,
		},
	}
	yes := true
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name + "-7f", Namespace: "shop",
			UID: types.UID(name + "-rs"),
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "Deployment", Name: name, Controller: &yes,
			}},
		},
		Spec: appsv1.ReplicaSetSpec{Replicas: &replicas},
	}
	return d, rs
}

func pod(
	name, owner, nodeName string, ready bool, since time.Time,
) *corev1.Pod {
	yes := true
	status := corev1.ConditionTrue
	if !ready {
		status = corev1.ConditionFalse
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "shop", UID: types.UID(name),
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "ReplicaSet", Name: owner, Controller: &yes,
			}},
		},
		Spec: corev1.PodSpec{
			NodeName:   nodeName,
			Containers: []corev1.Container{{Name: "app", Image: "app:1"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type: corev1.PodReady, Status: status,
				LastTransitionTime: metav1.NewTime(since),
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "app", Ready: ready, Image: "app:1",
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
			}},
		},
	}
}

func ctxBackground() context.Context { return context.Background() }
