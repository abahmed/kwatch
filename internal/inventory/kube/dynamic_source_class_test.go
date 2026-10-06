package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/abahmed/kwatch/internal/inventory"
)

// A cluster-scoped class watched for metadata only is announced under
// the same id the detectors and links look it up by.
func TestDynamicSourceAnnouncesMetadataClassUnderCoreID(t *testing.T) {
	scheme := runtime.NewScheme()
	metav1.AddMetaToScheme(scheme)
	class := &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.k8s.io/v1", Kind: "IngressClass",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "alb"},
	}
	r := runDynamicSource(t, DynamicConfig{
		Client:   dynamicClient(),
		Metadata: metadatafake.NewSimpleMetadataClient(scheme, class),
		Discovery: newLockedDiscovery(apiList("networking.k8s.io/v1",
			servedResource("ingressclasses", "IngressClass", false))),
	})

	r.pass(t)

	r.observed(t, inventory.Observed,
		entityIs(inventory.CoreID(KindIngressClass, "", "alb")))
	state, found := r.src.KindState(KindIngressClass)
	assert.True(t, found)
	assert.True(t, state.Synced)
}
