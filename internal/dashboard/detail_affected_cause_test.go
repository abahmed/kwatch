package dashboard

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// rootStatus is the Affected status of p's root object.
func rootStatus(t *testing.T, d IncidentDetail) string {
	t.Helper()
	for _, o := range d.Affected {
		if o.Root {
			return o.Status
		}
	}
	t.Fatalf("no root in %+v", d.Affected)
	return ""
}

// Without a cause the page says "Cause not found yet", so the issue's
// own object is not labelled the likely cause; with a cause that names
// the root, it is.
func TestDetailAffectedRootIsCauseOnlyWithACause(t *testing.T) {
	root := inventory.CoreID(kube.KindDeployment, "shop", "web")
	pod := kube.ContainerID("shop", "web-1", "web")
	p := failingAt("c", pod, testEpoch)
	p.Root = root
	if got := rootStatus(t, toDetail(p)); got != statusAffected {
		t.Errorf("root without a cause is %q, want %q", got,
			statusAffected)
	}
	p.Cause = &rootcause.CauseRecord{Root: root,
		Summary: "the new version can't start"}
	if got := rootStatus(t, toDetail(p)); got != statusCause {
		t.Errorf("root with a cause is %q, want %q", got, statusCause)
	}
}
