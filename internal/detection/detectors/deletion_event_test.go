package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A stuck deletion whose controller lives outside kwatch's view still
// has a reason: the latest Warning event on the object, quoted as the
// controller wrote it.
func TestStuckDeletionQuotesTheLatestWarningEvent(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindService, "kube-addons", "ingress")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrDeleting:      inventory.Bool(true),
		kube.AttrDeletingSince: inventory.Time(t0),
		kube.AttrFinalizers:    inventory.Text("service.k8s.aws/resources"),
	})
	now := t0.Add(time.Hour)
	noteEntity(m, id, "FailedDeployModel",
		"DeleteLoadBalancer: ResourceInUse: older", 1, now.Add(-20*time.Minute))
	noteEntity(m, id, "FailedBuildModel",
		"DeleteLoadBalancer: ResourceInUse: newer", 1, now.Add(-5*time.Minute))

	got := evaluate(Generic{}, m, now, id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, "DeleteLoadBalancer: ResourceInUse: newer",
		evidenceValue(got[0], detection.EvidenceLastWarning))
}

// An event too old to be current is not the reason it is stuck now.
func TestStuckDeletionIgnoresAnOldWarningEvent(t *testing.T) {
	m := newTestModel()
	id := stuckSecret(m)
	now := t0.Add(5 * time.Hour)
	noteEntity(m, id, "Failed", "long ago", 1, now.Add(-3*time.Hour))

	got := evaluate(Generic{}, m, now, id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, "", evidenceValue(got[0], detection.EvidenceLastWarning))
}

func evidenceValue(f detection.Finding, label string) string {
	for _, e := range f.Evidence {
		if e.Label == label {
			return e.Value
		}
	}
	return ""
}
