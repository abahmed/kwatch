package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func entry(
	manager string, op metav1.ManagedFieldsOperationType,
	subresource string, offset time.Duration,
) metav1.ManagedFieldsEntry {
	return metav1.ManagedFieldsEntry{
		Manager: manager, Operation: op, Subresource: subresource,
		Time: timePtr(fixedTime().Add(offset)),
	}
}

func TestSpecManagerSkipsStatusWrites(t *testing.T) {
	apply, update := metav1.ManagedFieldsOperationApply,
		metav1.ManagedFieldsOperationUpdate
	tt := []struct {
		name    string
		entries []metav1.ManagedFieldsEntry
		want    string
	}{
		{"latest_spec_write", []metav1.ManagedFieldsEntry{
			entry("helm", update, "", 0),
			entry("argocd", apply, "", time.Minute),
			entry("operator", update, "status", 2*time.Minute),
		}, "argocd"},
		{"status_only", []metav1.ManagedFieldsEntry{
			entry("operator", update, "status", 0),
		}, ""},
		{"unknown_operation", []metav1.ManagedFieldsEntry{
			{Manager: "kubectl", Time: timePtr(fixedTime())},
		}, ""},
		{"none", nil, ""},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			p := pod("p")
			p.ManagedFields = tc.entries
			assert.Equal(t, tc.want, kube.SpecManager(p))
		})
	}
}

func TestTrimToLatestManagerKeepsSpecManager(t *testing.T) {
	p := pod("p")
	p.ManagedFields = []metav1.ManagedFieldsEntry{
		entry("operator", metav1.ManagedFieldsOperationUpdate, "", 0),
		entry("kubelet", metav1.ManagedFieldsOperationUpdate, "status",
			time.Minute),
	}
	_, err := kube.TrimToLatestManager(p)
	require.NoError(t, err)
	assert.Len(t, p.ManagedFields, 2)
	assert.Equal(t, "kubelet", kube.Actor(p))
	assert.Equal(t, "operator", kube.SpecManager(p))
}

func managedWidget(manager string) *unstructured.Unstructured {
	w := widget(nil)
	w.SetManagedFields([]metav1.ManagedFieldsEntry{
		entry(manager, metav1.ManagedFieldsOperationApply, "", 0),
	})
	return w
}

func TestLinksManagedByResolvesControllerWorkload(t *testing.T) {
	tt := []struct {
		name        string
		deployments [][2]string
		want        []inventory.EntityID
	}{
		{"same_namespace_preferred", [][2]string{
			{"ops", "widget-operator"}, {testNamespace, "widget-operator"},
		}, []inventory.EntityID{inventory.CoreID(
			kube.KindDeployment, testNamespace, "widget-operator")}},
		{"single_other_namespace", [][2]string{{"ops", "widget-operator"}},
			[]inventory.EntityID{inventory.CoreID(
				kube.KindDeployment, "ops", "widget-operator")}},
		{"ambiguous_elsewhere", [][2]string{
			{"a", "widget-operator"}, {"b", "widget-operator"},
		}, nil},
		{"missing", [][2]string{{testNamespace, "other"}}, nil},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			model := inventory.NewModel(inventory.Options{})
			for _, d := range tc.deployments {
				feed(t, model, kube.DeploymentSchema(),
					deploymentIn(d[0], d[1]))
			}
			feed(t, model, widgetSchema(),
				managedWidget("widget-operator"))
			assert.Equal(t, tc.want,
				linksOf(model, widgetID(), inventory.ManagedBy))
		})
	}
}

func TestLinksManagedByIsInferred(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	feed(t, model, kube.DeploymentSchema(),
		deploymentIn(testNamespace, "op"))
	feed(t, model, widgetSchema(), managedWidget("op"))
	links := kube.Links(model, widgetID())
	require.Len(t, links, 1)
	assert.True(t, links[0].Inferred)
}
