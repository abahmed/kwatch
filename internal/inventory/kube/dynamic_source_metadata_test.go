package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestDynamicSourceWatchesMetadataKinds(t *testing.T) {
	scheme := runtime.NewScheme()
	metav1.AddMetaToScheme(scheme)
	lease := &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "coordination.k8s.io/v1", Kind: "Lease",
		},
		ObjectMeta: deletingMeta("n1"),
	}
	r := runDynamicSource(t, DynamicConfig{
		Client:   dynamicClient(),
		Metadata: metadatafake.NewSimpleMetadataClient(scheme, lease),
		Discovery: newLockedDiscovery(apiList("coordination.k8s.io/v1",
			servedResource("leases", "Lease", false))),
	})

	status := r.pass(t)
	assert.Equal(t, 1, status.Watched[WatchMetadata])
	o := r.observed(t, inventory.Observed,
		entityIs(inventory.NewEntityID("", "lease", "ns", "n1")))
	assert.Equal(t, map[string]inventory.Value{
		AttrWatchMode:     inventory.Text("metadata"),
		AttrGeneration:    inventory.Number(4),
		AttrDeleting:      inventory.Bool(true),
		AttrDeletingSince: inventory.Time(deletedAt),
		AttrFinalizers:    inventory.Text("a,b"),
	}, o.Attributes)
}
