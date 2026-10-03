//go:build e2e

package scenarios

import (
	"testing"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

// TestScenarioExtendedClusterDNSDown scales CoreDNS to zero and expects a
// cluster-dns page rather than blame on nodes or consumers. CoreDNS is
// restored before the scenario ends.
func TestScenarioExtendedClusterDNSDown(t *testing.T) {
	inExtendedCluster(t, "integration.cluster-dns", func(s *Scenario) {
		s.StopCoreDNS()
		s.ExpectRoot(harness.RootExpectation{
			Root:         "cluster-dns//cluster-dns",
			Tier:         "page",
			MaxMessages:  2,
			MustNotBlame: s.NodeRoots(),
		})
	})
}
