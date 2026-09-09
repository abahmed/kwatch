package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

// The kubelet projects kube-root-ca.crt into every pod. As a dependency it
// carries no signal and made every incident in a namespace read as a shared
// failure of that ConfigMap.
func TestAddPodVolumeToGraphSkipsClusterCABundle(t *testing.T) {
	c := &Controller{graph: kwcontext.NewResourceGraph()}
	c.addPodVolumeToGraph("ns1", "p1", corev1.Volume{
		Name: "kube-api-access-abcde",
		VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{
			Sources: []corev1.VolumeProjection{
				{ConfigMap: &corev1.ConfigMapProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "kube-root-ca.crt",
					},
				}},
				{ConfigMap: &corev1.ConfigMapProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "app-settings",
					},
				}},
			},
		}},
	})
	c.addPodVolumeToGraph("ns1", "p1", corev1.Volume{
		Name: "ca",
		VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "openshift-service-ca.crt",
			},
		}},
	})

	assert.Equal(
		t,
		[]string{"configmap/ns1/app-settings"},
		c.graph.DependenciesOf("pod", "ns1", "p1"),
	)
}
