package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestStatefulSetSchemaRecordsRollout(t *testing.T) {
	ss := statefulSet("web")
	partition := int32(2)
	ss.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
		RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
			Partition: &partition,
		},
	}
	ss.Status.CurrentRevision = "web-1"
	ss.Status.UpdateRevision = "web-2"
	ss.Status.CurrentReplicas = 2

	desc, ok := kube.StatefulSetSchema().Describe(ss)

	assert.True(t, ok)
	attrs := desc.Attributes
	assert.Equal(t, "web-1", attrs[kube.AttrCurrentRevision].AsText())
	assert.Equal(t, "web-2", attrs[kube.AttrRevision].AsText())
	current, _ := attrs[kube.AttrCurrentReplicas].AsNumber()
	assert.Equal(t, 2.0, current)
	held, _ := attrs[kube.AttrPartition].AsNumber()
	assert.Equal(t, 2.0, held)
	assert.Equal(t, "RollingUpdate", attrs[kube.AttrUpdateStrategy].AsText())
	assert.Equal(t, "OrderedReady", attrs[kube.AttrPodManagement].AsText())
}

func TestStatefulSetSchemaDefaultsRolloutStrategy(t *testing.T) {
	ss := statefulSet("db")
	ss.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{}
	ss.Spec.PodManagementPolicy = appsv1.ParallelPodManagement

	desc, _ := kube.StatefulSetSchema().Describe(ss)

	held, _ := desc.Attributes[kube.AttrPartition].AsNumber()
	assert.Zero(t, held)
	assert.Equal(t, "RollingUpdate",
		desc.Attributes[kube.AttrUpdateStrategy].AsText())
	assert.Equal(t, "Parallel",
		desc.Attributes[kube.AttrPodManagement].AsText())
}
