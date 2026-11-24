package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// lateWithdrawModel has a Lease that a watch and a prober describe,
// which the watch removed.
func lateWithdrawModel(t *testing.T) (*Model, EntityID) {
	t.Helper()
	m := NewModel(Options{})
	lease := CoreID("lease", "shop", "leader")
	at := testTime
	for _, o := range []Observation{
		{Kind: Observed, Source: "kubernetes", At: at, Entity: lease,
			UID: "u1", Attributes: map[string]Value{"a": Text("x")}},
		{Kind: Observed, Source: "probe", At: at, Entity: lease,
			Attributes: map[string]Value{"holder": Text("p")}},
		{Kind: Gone, Source: "kubernetes", At: at.Add(time.Second),
			Entity: lease},
	} {
		_, err := m.Apply(o)
		assert.NoError(t, err)
	}
	return m, lease
}

// The prober withdraws what it said after the watch's Gone: the Lease
// stays gone, and its tombstone is pruned like any other.
func TestLateWithdrawDoesNotBringBackAGoneEntity(t *testing.T) {
	m, lease := lateWithdrawModel(t)
	update, err := m.Apply(Observation{Kind: Observed, Source: "probe",
		At: testTime.Add(2 * time.Second), Entity: lease,
		Attributes: map[string]Value{}})
	assert.NoError(t, err)
	assert.False(t, update.Appeared)
	assert.False(t, m.Exists(lease))

	m.Prune(testTime.Add(time.Hour))
	assert.Equal(t, 0, m.Stats().Tombstones)
}

// A prober that still reports the Lease, and the watch that removed it
// observing it again, bring it back: that is news, not a withdraw.
func TestObservationsAfterGoneStillBringTheEntityBack(t *testing.T) {
	m, lease := lateWithdrawModel(t)
	_, _ = m.Apply(Observation{Kind: Observed, Source: "probe",
		At: testTime.Add(2 * time.Second), Entity: lease,
		Attributes: map[string]Value{"holder": Text("q")}})
	assert.True(t, m.Exists(lease))

	m, lease = lateWithdrawModel(t)
	_, _ = m.Apply(Observation{Kind: Observed, Source: "kubernetes",
		At: testTime.Add(2 * time.Second), Entity: lease})
	assert.True(t, m.Exists(lease))
}
