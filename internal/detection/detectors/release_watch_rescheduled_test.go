package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// rescheduledRelease is a release whose new ReplicaSet was created
// rsAge ago and whose pods a node rotation recreated ten minutes ago.
func rescheduledRelease(
	t *testing.T, rsAge time.Duration,
) (*inventory.Model, inventory.EntityID) {
	steady := make([]float64, 12)
	steady[3] = 2
	m, dep := buildRelease(t, releaseCase{history: steady,
		restarts: [2]float64{3, 4}, age: 10 * time.Minute})
	rs := newID(kube.KindReplicaSet, "shop", "api-new")
	put(m, rs, t0.Add(-2*time.Hour), map[string]inventory.Value{
		kube.AttrRevision: inventory.Text("14"),
		kube.AttrCreated:  inventory.Time(t0.Add(-rsAge)),
	})
	return m, dep
}

func TestReleaseWatchIgnoresPodsRecreatedForAnOldRevision(t *testing.T) {
	m, dep := rescheduledRelease(t, 21*24*time.Hour)

	assert.Empty(t, evaluate(ReleaseWatch{}, m, t0, dep, nil).Findings,
		"a three-week-old revision is not a new rollout")
}

func TestReleaseWatchJudgesARevisionCreatedRecently(t *testing.T) {
	m, dep := rescheduledRelease(t, 10*time.Minute)

	got := evaluate(ReleaseWatch{}, m, t0, dep, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.ReleaseRegression, got[0].Reason)
}

func TestReleaseWatchCountsARollbackFromItsRevisionChange(t *testing.T) {
	m, dep := rescheduledRelease(t, 21*24*time.Hour)
	rs := newID(kube.KindReplicaSet, "shop", "api-new")
	put(m, rs, t0.Add(-10*time.Minute), map[string]inventory.Value{
		kube.AttrRevision: inventory.Text("15"),
		kube.AttrCreated:  inventory.Time(t0.Add(-21 * 24 * time.Hour)),
	})

	got := evaluate(ReleaseWatch{}, m, t0, dep, nil).Findings
	require.Len(t, got, 1, "a rollback makes an old ReplicaSet new")
}
