package explain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// stuckNamespace adds a Terminating namespace whose discovery failure
// names the group version.
func (f *fixture) stuckNamespace(name, groupVersion string) inventory.EntityID {
	ns := inventory.CoreID(kube.KindNamespace, "", name)
	f.add(ns)
	f.findings[ns] = []detection.Finding{{
		Entity: ns, Reason: "Terminating", Mode: detection.ModeTerminating,
		Health: degradedH, Since: t0.Add(-2 * time.Hour),
		Summary: "Discovery failed for some groups: " + groupVersion +
			": stale GroupVersion discovery",
	}}
	return ns
}

// A namespace stuck on a group version whose APIService is unavailable is
// explained by that APIService.
func TestNamespaceStuckOnAnUnavailableAPIServiceIsExplainedByIt(t *testing.T) {
	f := newFixture(t)
	api := inventory.CoreID(kube.KindAPIService, "", "v1beta1.metrics.k8s.io")
	f.add(api)
	f.findings[api] = []detection.Finding{{
		Entity: api, Reason: "APIServiceFailure",
		Mode: detection.ModeAPIServiceUnavailable, Health: failingH,
		Since: t0.Add(-3 * time.Hour), Summary: "Available=False",
	}}
	ns := f.stuckNamespace("old-shop", "metrics.k8s.io/v1beta1")

	c, ok := rootOf(f, ns)

	assert.True(t, ok)
	assert.Equal(t, api, c.Root)
}

// A group version whose APIService is healthy is not blamed.
func TestNamespaceStuckOnAHealthyAPIServiceIsNotBlamedOnIt(t *testing.T) {
	f := newFixture(t)
	api := inventory.CoreID(kube.KindAPIService, "", "v1beta1.metrics.k8s.io")
	f.add(api)
	ns := f.stuckNamespace("old-shop", "metrics.k8s.io/v1beta1")

	c, ok := rootOf(f, ns)

	assert.False(t, ok && c.Root == api)
}
