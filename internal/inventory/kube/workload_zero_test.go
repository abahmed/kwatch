package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestDeploymentSchemaRecordsItsPodLabels(t *testing.T) {
	d := deployment("api")
	d.Spec.Template.Labels = map[string]string{"app": "api", "tier": "web"}

	desc, ok := kube.DeploymentSchema().Describe(d)

	require.True(t, ok)
	assert.Equal(t, "app=api,tier=web",
		desc.Attributes[kube.AttrTemplateLabels].AsText())
}

func TestJobSchemaRecordsCompletionAndDeadline(t *testing.T) {
	j := job("j1")
	done := metav1.NewTime(fixedTime())
	deadline := int64(3600)
	j.Status.CompletionTime = &done
	j.Spec.ActiveDeadlineSeconds = &deadline

	desc, ok := kube.JobSchema().Describe(j)

	require.True(t, ok)
	assert.True(t, fixedTime().Equal(
		desc.Attributes[kube.AttrCompletionTime].AsTime()))
	deadlineSeconds, _ := desc.Attributes[kube.AttrActiveDeadline].AsNumber()
	assert.Equal(t, 3600.0, deadlineSeconds)
}

func TestServicesSelectingFindsAServiceOfAWorkloadWithoutPods(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	apply := func(id inventory.EntityID, attrs map[string]inventory.Value) {
		_, err := model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: kube.ObservationSource,
			At: fixedTime(), Entity: id, Attributes: attrs})
		require.NoError(t, err)
	}
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	apply(api, map[string]inventory.Value{
		kube.AttrTemplateLabels: inventory.Text("app=api,tier=web")})
	apply(inventory.CoreID(kube.KindService, "shop", "api"),
		map[string]inventory.Value{
			kube.AttrSelector: inventory.Text("app=api")})
	apply(inventory.CoreID(kube.KindService, "shop", "other"),
		map[string]inventory.Value{
			kube.AttrSelector: inventory.Text("app=other")})
	apply(inventory.CoreID(kube.KindService, "shop", "headless"),
		map[string]inventory.Value{})
	apply(inventory.CoreID(kube.KindService, "elsewhere", "api"),
		map[string]inventory.Value{
			kube.AttrSelector: inventory.Text("app=api")})
	workload, _ := model.Entity(api)

	got := kube.ServicesSelecting(model, workload)

	assert.Equal(t, []inventory.EntityID{
		inventory.CoreID(kube.KindService, "shop", "api")}, got)
}
