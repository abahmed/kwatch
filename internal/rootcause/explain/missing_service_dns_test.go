package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A resolver that answers "temporary failure" (EAI_AGAIN) did not say
// the name does not exist: the Service named in the configuration is not
// the cause of such a lookup.
func TestMissingServiceNeedsMoreThanATemporaryLookupFailure(t *testing.T) {
	f := newFixture(t)
	api, missing := missingFixture(f, "shop/paymnts:8080")
	failAPI(f, api, "getaddrinfo EAI_AGAIN paymnts.shop.svc.cluster.local")

	for _, root := range causes(f.explain()) {
		if root == missing.String() {
			t.Fatalf("a missing Service was blamed for a temporary "+
				"lookup failure: %v", root)
		}
	}
}

// With cluster DNS failing, every lookup fails whether the Service
// exists or not: a name in the configuration is not proof it is missing.
func TestMissingServiceYieldsToFailingClusterDNS(t *testing.T) {
	f := newFixture(t)
	api, missing := missingFixture(f, "shop/paymnts:8080")
	failAPI(f, api, "getaddrinfo ENOTFOUND paymnts.shop.svc.cluster.local")
	f.fail(kube.ClusterDNS, detection.ModeDNSServfail, failingH, 2, "")

	for _, root := range causes(f.explain()) {
		if root == missing.String() {
			t.Fatalf("a missing Service was blamed while cluster DNS "+
				"fails: %v", root)
		}
	}
}
