package controller

import (
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

func TestBuildGraphAndPruneUseInformerSnapshots(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "api"}},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api", Namespace: "apps", Labels: map[string]string{"app": "api"},
		},
		Spec: corev1.PodSpec{NodeName: "node-a"},
	}
	volume := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-api"},
		Spec: corev1.PersistentVolumeSpec{ClaimRef: &corev1.ObjectReference{
			Namespace: "apps", Name: "claim",
		}},
	}
	claim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "claim", Namespace: "apps"},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "pv-api"},
	}
	selector := metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}
	replicas := int32(1)
	replicaset := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "apps",
	}, Spec: appsv1.ReplicaSetSpec{Replicas: &replicas, Selector: &selector}}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "apps",
	}}
	ingress := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "apps",
	}}
	hpa := &autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "apps",
	}}
	netpol := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "apps",
	}, Spec: networkingv1.NetworkPolicySpec{PodSelector: selector}}
	pdb := &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "apps",
	}, Spec: policyv1.PodDisruptionBudgetSpec{Selector: &selector}}
	eps := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "apps", Labels: map[string]string{
			endpointSliceServiceLabel: "api",
		},
	}}
	controller, cancel := newGraphTestGraph(
		pod, service, volume, claim, replicaset, job, ingress, hpa,
		netpol, pdb, eps,
	)
	defer cancel()
	controller.buildGraph()
	controller.rebuildService(service)
	controller.rebuildJob(job)
	controller.rebuildHorizontalPodAutoscaler(hpa)
	controller.rebuildNetworkPolicy(netpol)
	controller.rebuildPodDisruptionBudget(pdb)
	controller.rebuildPersistentVolume(volume)
	if err := controller.rebuildPersistentVolumeChecked(volume); err != nil {
		t.Fatalf("rebuildPersistentVolumeChecked() error = %v", err)
	}
	if deps := controller.graph.DependenciesOf(
		"pod", "apps", "api",
	); len(deps) == 0 {
		t.Fatal("graph build produced no pod dependencies")
	}
	controller.graph.AddEdge(
		"service", "apps", "stale", "networktarget", "", "stale", "test",
	)
	controller.pruneGraph()
	if len(controller.graph.DependenciesOf("service", "apps", "stale")) != 0 {
		t.Fatal("pruneGraph retained stale service edges")
	}
}

func TestGraphBuildHelperErrorWrapping(t *testing.T) {
	if err := rebuildFrom([]int(nil), errors.New("list failed"),
		"items", func(interface{}) {}); err == nil {
		t.Fatal("rebuildFrom hid list failure")
	}
	if err := rebuildCheckedFrom([]int{1}, nil, "items",
		func(int) error { return errors.New("build failed") }); err == nil {
		t.Fatal("rebuildCheckedFrom hid build failure")
	}
	if err := rebuildFrom(
		[]int{1, 2}, nil, "items", func(interface{}) {},
	); err != nil {
		t.Fatalf("rebuildFrom() error = %v", err)
	}
	if err := rebuildCheckedFrom([]int{1, 2}, nil, "items",
		func(int) error { return nil }); err != nil {
		t.Fatalf("rebuildCheckedFrom() error = %v", err)
	}
	if err := (&graphBuilder{
		graph: kwcontext.NewResourceGraph(),
	}).buildResourceGraph(); err != nil {
		t.Fatalf("empty resource graph build = %v", err)
	}
}
