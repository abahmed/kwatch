package core

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/reason"
	"github.com/abahmed/kwatch/internal/signal"
	"github.com/abahmed/kwatch/internal/signal/detect"
	"github.com/abahmed/kwatch/internal/story"
)

// harness drives the pipeline one step at a time with a manual clock.
type harness struct {
	t         *testing.T
	engine    *Engine
	checks    *rechecks
	now       time.Time
	decisions []problem.Decision
	messages  []story.Message
}

func newHarness(t *testing.T, start time.Time) *harness {
	t.Helper()
	h := &harness{t: t, checks: newRechecks(), now: start}
	rules := reason.NewEngine(0,
		reason.NodeRule{}, reason.RolloutRule{}, reason.ConfigRule{},
		reason.ReferenceRule{}, reason.BackendRule{},
		reason.SchedulingRule{}, reason.PodsRule{},
		reason.AdmissionRule{},
		reason.QuotaRule{},
		reason.NetworkPolicyRule{},
		reason.MetricsAPIRule{},
		reason.DNSRule{},
		reason.TopologyRule{},
	)
	engine, err := NewEngine(Dependencies{
		Model:     knowledge.NewModel(knowledge.Options{}),
		Detectors: signal.NewRegistry(nil, detectors()...),
		Problems:  problem.NewManager(problem.Config{}, rules),
		Sink: func(_ context.Context, d problem.Decision, m story.Message) {
			h.decisions = append(h.decisions, d)
			h.messages = append(h.messages, m)
		},
		Clock: fixedClock{h},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.engine = engine
	return h
}

func detectors() []signal.Detector {
	return []signal.Detector{
		detect.Container{}, detect.NewPod(detect.PodThresholds{}),
		detect.NewNode(0), detect.NewWorkload(0), detect.Service{},
		detect.Missing{},
		detect.Event{},
		detect.Budget{},
		detect.Quota{},
		detect.Attachment{},
		detect.Webhook{},
		detect.NodeUsage{},
		detect.VolumeUsage{},
		detect.Custom{},
		detect.ClusterService{},
	}
}

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
