package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const sliceAddressLine = "dial tcp 10.244.0.10:8080: connect: no route to host"

// An address an EndpointSlice lists belongs to the cluster, even when no
// pod holds it: the failing call is not a call to an outside endpoint.
func TestSliceAddressIsNotAnExternalEndpoint(t *testing.T) {
	for _, listed := range []bool{false, true} {
		f := newFixture(t)
		slice := inventory.CoreID(kube.KindEndpointSlice, "shop", "pay-x")
		attrs := map[string]inventory.Value{}
		if listed {
			attrs[kube.AttrEndpointAddresses] = inventory.Text(
				"10.244.0.10,10.244.0.11")
		}
		f.observe(slice, attrs)
		effect := crashOn(f, "shop", "api", sliceAddressLine)
		crashOn(f, "billing", "web", sliceAddressLine)
		c, ok := f.explain().CauseOf(effect)
		external := ok && c.Root.Kind == KindExternalEndpoint
		if external == listed {
			t.Fatalf("listed %v: cause %v (ok %v)", listed, c.Root, ok)
		}
	}
}
