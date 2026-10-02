package replay_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/pipeline"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

var rolloutStart = time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)

// newDependencies builds fresh engine state with the built-in detectors
// and rules, as the application does.
func newDependencies() pipeline.Dependencies {
	return pipeline.Dependencies{
		Model:     inventory.NewModel(inventory.Options{}),
		Detectors: detection.NewRegistry(nil, detectors.Default()...),
		// A fixed ID nonce keeps replays byte-for-byte comparable.
		Incidents: incident.NewManager(incident.Config{IDNonce: "0000"},
			explain.NewSolver()),
	}
}

// recordBadRollout records a rollout whose new revision crash-loops while
// the old revision stays healthy.
func recordBadRollout(t *testing.T) []byte {
	t.Helper()
	return recordRollout(t, 0)
}

// recordRollout records the bad rollout and, when recoverAfter is set,
// the new pod turning healthy that long after it started crashing.
func recordRollout(t *testing.T, recoverAfter time.Duration) []byte {
	t.Helper()
	now := rolloutStart
	var buf bytes.Buffer
	rec, err := replay.NewRecorder(&buf,
		func() time.Time { return now }, nil)
	if err != nil {
		t.Fatal(err)
	}
	add := func(schema kube.Schema, objects ...any) {
		translator := kube.NewTranslator(schema)
		for _, obj := range objects {
			rec.Submit(context.Background(),
				translator.Added(obj, true, now)...)
		}
	}
	add(kube.NodeSchema{}, node("n1"))
	old, oldRS := deployment("payments")
	old.Spec.Template = podTemplate("app:2.2")
	add(kube.DeploymentSchema(), old)
	add(kube.ReplicaSetSchema(), oldRS)
	add(kube.PodSchema{}, pod("payments-old", oldRS.Name, true, now))

	now = now.Add(70 * time.Second)
	updated := old.DeepCopy()
	updated.Spec.Template = podTemplate("app:2.3")
	rec.Submit(context.Background(), kube.NewTranslator(
		kube.DeploymentSchema()).Updated(old, updated, now)...)
	newRS := replicaSet("payments-9c", "payments")
	add(kube.ReplicaSetSchema(), newRS)
	crashing := crashingPod("payments-new", newRS.Name,
		now.Add(40*time.Second))
	add(kube.PodSchema{}, crashing)
	if recoverAfter > 0 {
		now = now.Add(recoverAfter)
		healthy := pod("payments-new", newRS.Name, true, now)
		rec.Submit(context.Background(), kube.NewTranslator(
			kube.PodSchema{}).Updated(crashing, healthy, now)...)
		ready := updated.DeepCopy()
		ready.Status.ReadyReplicas, ready.Status.AvailableReplicas = 2, 2
		rec.Submit(context.Background(), kube.NewTranslator(
			kube.DeploymentSchema()).Updated(updated, ready, now)...)
	}
	if err := rec.Err(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func node(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(name)},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type: corev1.NodeReady, Status: corev1.ConditionTrue,
		}}},
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
	rs := replicaSet(name+"-7f", name)
	rs.Spec.Replicas = &replicas
	return d, rs
}

func replicaSet(name, deploymentName string) *appsv1.ReplicaSet {
	yes := true
	replicas := int32(1)
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "shop", UID: types.UID(name),
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "Deployment", Name: deploymentName, Controller: &yes,
			}},
		},
		Spec: appsv1.ReplicaSetSpec{Replicas: &replicas},
	}
}

func podTemplate(image string) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{Spec: corev1.PodSpec{
		Containers: []corev1.Container{{Name: "app", Image: image}},
	}}
}

func pod(name, owner string, ready bool, since time.Time) *corev1.Pod {
	yes := true
	status := corev1.ConditionTrue
	if !ready {
		status = corev1.ConditionFalse
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "shop", UID: types.UID(name),
			Labels: map[string]string{"app": "payments"},
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "ReplicaSet", Name: owner, Controller: &yes,
			}},
		},
		Spec: corev1.PodSpec{
			NodeName:   "n1",
			Containers: []corev1.Container{{Name: "app", Image: "app:2.2"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type: corev1.PodReady, Status: status,
				LastTransitionTime: metav1.NewTime(since),
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "app", Ready: ready, Image: "app:2.2",
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
			}},
		},
	}
}

func crashingPod(name, owner string, since time.Time) *corev1.Pod {
	p := pod(name, owner, false, since)
	p.Spec.Containers[0].Image = "app:2.3"
	p.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "app", Image: "app:2.3", RestartCount: 4,
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
			Reason: "CrashLoopBackOff",
		}},
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				Reason: "Error", ExitCode: 1,
				Message: "panic: missing key DB_PASSWORD_V2",
			},
		},
	}}
	return p
}
