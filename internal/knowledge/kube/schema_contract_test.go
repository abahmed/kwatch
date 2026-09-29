package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func allSchemas() []kube.Schema {
	return []kube.Schema{
		kube.PodSchema{}, kube.NodeSchema{}, kube.DeploymentSchema(),
		kube.ReplicaSetSchema(), kube.StatefulSetSchema(),
		kube.DaemonSetSchema(), kube.JobSchema(), kube.CronJobSchema{},
		kube.HPASchema{}, kube.ServiceSchema{},
		kube.EndpointSliceSchema{}, kube.IngressSchema{},
		kube.SecretSchema{}, kube.ConfigMapSchema{},
		kube.ServiceAccountSchema{}, kube.PVCSchema{}, kube.PVSchema{},
		kube.StorageClassSchema{}, kube.NamespaceSchema{},
		kube.LimitRangeSchema{}, kube.PDBSchema{}, kube.QuotaSchema{},
		kube.NetworkPolicySchema{}, kube.VolumeAttachmentSchema{},
		kube.WebhookSchema{}, kube.WebhookSchema{Mutating: true},
		kube.NewUnstructuredSchema("Widget"),
	}
}

func TestSchemasRejectForeignObjects(t *testing.T) {
	for _, s := range allSchemas() {
		t.Run(string(s.Kind()), func(t *testing.T) {
			assert.NotEmpty(t, s.Kind())
			_ = s.RelationTypes()
			_, ok := s.Describe("not a kubernetes object")
			assert.False(t, ok)
			assert.Empty(t, s.Diff("a", "b"))
		})
	}
}
