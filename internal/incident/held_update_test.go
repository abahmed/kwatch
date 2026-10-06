package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// heldUpdateRecords exports an incident whose crash-loop update was
// decided and then held for its investigation.
func heldUpdateRecords(t *testing.T, hold bool) []Record {
	t.Helper()
	r := newRig(t, Config{})
	_, pod := r.workloadRig(t, 2, 1, 1)
	r.raise(at(0), notReadySig(pod))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	r.raise(at(2*time.Minute), crashSig(pod))
	ds := r.tick(at(2 * time.Minute))
	wantAction(t, ds, Update, ReasonMaterialChange)
	if hold {
		r.m.HoldUpdate(ds[0].Incident.ID)
	}
	return r.m.Export()
}

// restoredUpdates restores the records and returns the updates decided
// once the failures are back.
func restoredUpdates(t *testing.T, recs []Record) []Decision {
	t.Helper()
	fresh := newRig(t, Config{})
	_, pod := fresh.workloadRig(t, 2, 1, 1)
	fresh.m.Restore(recs, at(20*time.Minute))
	fresh.raise(at(10*time.Minute), notReadySig(pod))
	fresh.raise(at(10*time.Minute), crashSig(pod))
	var got []Decision
	for now := 10 * time.Minute; now <= 35*time.Minute; now += 10 *
		time.Second {
		got = append(got, fresh.tick(at(now))...)
	}
	return got
}

// An update that was held for its investigation when kwatch stopped is
// decided again after the restart: the saved fingerprint is the one
// people were last told, and the restore does not adopt a newer one.
func TestHeldUpdateIsDecidedAgainAfterARestart(t *testing.T) {
	recs := heldUpdateRecords(t, true)
	require.True(t, recs[0].HeldUpdate)

	got := restoredUpdates(t, recs)

	require.NotEmpty(t, got)
	assert.Equal(t, Update, got[0].Action)
	assert.Equal(t, ReasonMaterialChange, got[0].Reason)
}

// An update that was delivered is not repeated: the record carries the
// new fingerprint and the restore adopts the current one as before.
func TestDeliveredUpdateIsNotRepeatedAfterARestart(t *testing.T) {
	recs := heldUpdateRecords(t, false)
	require.False(t, recs[0].HeldUpdate)

	assert.Empty(t, restoredUpdates(t, recs))
}

func TestReleasedUpdateSavesTheNewFingerprint(t *testing.T) {
	r := newRig(t, Config{})
	_, pod := r.workloadRig(t, 2, 1, 1)
	r.raise(at(0), notReadySig(pod))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	r.raise(at(2*time.Minute), crashSig(pod))
	id := r.tick(at(2 * time.Minute))[0].Incident.ID
	r.m.HoldUpdate(id)
	held := r.m.Export()[0].Digest

	r.m.ReleaseUpdate(id)

	assert.NotEqual(t, held, r.m.Export()[0].Digest)
	assert.False(t, r.m.Export()[0].HeldUpdate)
}
