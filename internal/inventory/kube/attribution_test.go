package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var attributionTime = time.Date(2026, 9, 29, 14, 2, 0, 0, time.UTC)

func managed(manager string, at time.Time) metav1.ManagedFieldsEntry {
	stamp := metav1.NewTime(at)
	return metav1.ManagedFieldsEntry{
		Manager: manager, Operation: metav1.ManagedFieldsOperationUpdate,
		Time: &stamp,
	}
}

func TestGitOpsApp(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		want        string
	}{
		{"argocd instance label",
			map[string]string{argoInstanceLabel: "shop"}, nil,
			"argocd/shop"},
		{"argocd tracking id", nil,
			map[string]string{argoTrackingID: "shop:apps/Deployment:a/b"},
			"argocd/shop"},
		{"flux kustomization",
			map[string]string{fluxKustomizeLabel: "apps"}, nil, "flux/apps"},
		{"helm release", map[string]string{helmChartLabel: "api-1.2.0"},
			map[string]string{helmReleaseName: "api"}, "helm/api"},
		{"helm chart only",
			map[string]string{helmChartLabel: "api-1.2.0"}, nil,
			"helm/api-1.2.0"},
		{"argocd renders helm", map[string]string{
			argoInstanceLabel: "shop", helmChartLabel: "api-1.2.0",
		}, nil, "argocd/shop"},
		{"none", nil, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := &metav1.ObjectMeta{
				Labels: tt.labels, Annotations: tt.annotations,
			}
			assert.Equal(t, tt.want, GitOpsApp(meta))
		})
	}
}

func TestRevision(t *testing.T) {
	annotated := func(key, value string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Annotations: map[string]string{key: value}}
	}
	tests := []struct {
		name string
		obj  any
		want string
	}{
		{"deployment revision", &appsv1.Deployment{
			ObjectMeta: annotated(deploymentRevision, "7")}, "7"},
		{"replicaset revision", &appsv1.ReplicaSet{
			ObjectMeta: annotated(deploymentRevision, "6")}, "6"},
		{"statefulset update revision", &appsv1.StatefulSet{
			Status: appsv1.StatefulSetStatus{UpdateRevision: "db-5c9"}},
			"db-5c9"},
		{"controller revision",
			&appsv1.ControllerRevision{Revision: 3}, "3"},
		{"daemonset generation", &appsv1.DaemonSet{
			ObjectMeta: annotated(daemonSetGeneration, "4")}, "4"},
		{"generation", &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Generation: 2}}, "generation 2"},
		{"none", &corev1.Service{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Revision(tt.obj))
		})
	}
}

func TestRevisionHashesConfigData(t *testing.T) {
	before := Revision(&corev1.ConfigMap{Data: map[string]string{"a": "1"}})
	after := Revision(&corev1.ConfigMap{Data: map[string]string{"a": "2"}})
	back := Revision(&corev1.ConfigMap{Data: map[string]string{"a": "1"}})
	assert.NotEqual(t, before, after)
	assert.Equal(t, before, back)
	secret := Revision(&corev1.Secret{Data: map[string][]byte{"a": []byte("1")}})
	assert.NotEmpty(t, secret)
}

func TestAttributeUsesLatestSpecWriter(t *testing.T) {
	status := managed("kubelet", attributionTime.Add(time.Minute))
	status.Subresource = "status"
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		ManagedFields: []metav1.ManagedFieldsEntry{
			managed("helm", attributionTime.Add(-time.Hour)),
			managed("argocd-controller", attributionTime),
			status,
		},
	}}
	who := Attribute(deployment, false)
	assert.Equal(t, "argocd-controller", who.Actor)
	assert.Equal(t, attributionTime, who.At)
}

func TestAttributeCreatedUsesCreationTime(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		CreationTimestamp: metav1.NewTime(attributionTime),
	}}
	assert.Equal(t, attributionTime, Attribute(node, true).At)
}

func TestChangeTimePrefersPlausibleAPITime(t *testing.T) {
	received := attributionTime.Add(3 * time.Second)
	tests := []struct {
		name string
		api  time.Time
		want time.Time
	}{
		{"api time", attributionTime, attributionTime},
		{"unknown", time.Time{}, received},
		{"within skew ahead", received.Add(time.Second),
			received.Add(time.Second)},
		{"beyond skew ahead", received.Add(time.Minute), received},
		{"stale write", received.Add(-time.Hour), received},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, changeTime(tt.api, received))
		})
	}
}

func TestTranslatorChangeCarriesAttribution(t *testing.T) {
	old := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "shop",
	}}
	next := old.DeepCopy()
	replicas := int32(3)
	next.Spec.Replicas = &replicas
	next.Labels = map[string]string{argoInstanceLabel: "shop"}
	next.Annotations = map[string]string{deploymentRevision: "8"}
	next.ManagedFields = []metav1.ManagedFieldsEntry{
		managed("argocd-controller", attributionTime),
	}
	received := attributionTime.Add(time.Second)
	observations := NewTranslator(DeploymentSchema()).
		Updated(old, next, received)
	change := observations[len(observations)-1].Change
	assert.Equal(t, attributionTime, change.At)
	assert.Equal(t, received, change.Observed)
	assert.Equal(t, "argocd-controller", change.Actor)
	assert.Equal(t, "argocd/shop", change.App)
	assert.Equal(t, "8", change.Revision)
}
