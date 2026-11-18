package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// markedDeployment is Deployment payments in shop with the given labels
// and annotations.
func markedDeployment(
	labels, annotations map[string]string, version string,
) *appsv1.Deployment {
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Namespace: "shop", Name: "payments", UID: "d1",
		ResourceVersion: version, Labels: labels,
		Annotations: annotations,
	}}
}

// deployedBy is what the observations say the object is deployed by;
// told is false when they say nothing about it.
func deployedBy(
	observations []inventory.Observation,
) (targets []inventory.EntityID, told bool) {
	for _, o := range observations {
		if o.Kind == inventory.Related &&
			o.Relation == inventory.DeployedBy {
			return o.Targets, true
		}
	}
	return nil, false
}

func TestDeployedByNamesTheHelmReleaseOfTheNamespace(t *testing.T) {
	translator := kube.NewTranslator(kube.DeploymentSchema())
	d := markedDeployment(map[string]string{
		"app.kubernetes.io/instance": "payments",
		"app.kubernetes.io/part-of":  "store"},
		map[string]string{"meta.helm.sh/release-name": "payments"}, "1")

	targets, told := deployedBy(translator.Added(d, true, ackAt))

	assert.True(t, told)
	assert.Equal(t, []inventory.EntityID{
		kube.HelmReleaseID("shop", "payments")}, targets)
}

func TestDeployedByLeavesOutAReleaseOfAnotherNamespace(t *testing.T) {
	translator := kube.NewTranslator(kube.DeploymentSchema())
	d := markedDeployment(nil, map[string]string{
		"meta.helm.sh/release-name":      "platform",
		"meta.helm.sh/release-namespace": "ops"}, "1")

	_, told := deployedBy(translator.Added(d, true, ackAt))

	assert.False(t, told)
}

func TestDeployedByIsToldOnlyForMarkedWorkloads(t *testing.T) {
	deployment := kube.NewTranslator(kube.DeploymentSchema())
	plain := markedDeployment(map[string]string{"app": "payments"},
		nil, "1")
	_, told := deployedBy(deployment.Added(plain, true, ackAt))
	assert.False(t, told, "an unmarked workload says nothing")

	configs := kube.NewTranslator(kube.ConfigMapSchema{})
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Namespace: "shop", Name: "payments", Labels: map[string]string{
			"app.kubernetes.io/instance": "payments"}}}
	_, told = deployedBy(configs.Added(cm, true, ackAt))
	assert.False(t, told, "only workloads are deployed by a release")
}

func TestDeployedByClearsWhenTheMarksGo(t *testing.T) {
	translator := kube.NewTranslator(kube.DeploymentSchema())
	marked := markedDeployment(map[string]string{
		"app.kubernetes.io/instance": "payments"}, nil, "1")
	plain := markedDeployment(nil, nil, "2")

	targets, told := deployedBy(translator.Updated(marked, plain, ackAt))

	assert.True(t, told, "the old edge is cleared")
	assert.Empty(t, targets)
	_, told = deployedBy(translator.Updated(plain,
		markedDeployment(nil, nil, "3"), ackAt))
	assert.False(t, told, "nothing to clear once it is gone")
}
