package scenarios

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A kind kwatch cannot observe was never listed: its source did not
// sync, so detectors skip it and an object of that kind is not "missing".
func TestDependenciesModelHiddenKindsAsNeverSynced(t *testing.T) {
	t.Parallel()
	deps := newDependencies(kube.KindSecret)
	if deps.Synced(kube.KindSecret) {
		t.Error("a hidden kind reads as synced: it models visible-then-lost")
	}
	if !deps.Synced(kube.KindPod) {
		t.Error("a kind that is not hidden must stay synced")
	}
}
