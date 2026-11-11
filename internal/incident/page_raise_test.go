package incident

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The page raise survives a restart through the saved record (and its
// JSON), so the restored incident keeps sending to the page key and its
// resolve closes both keys.
func TestPageRaiseSurvivesARestart(t *testing.T) {
	r, _ := pagedRig(t)
	id := r.only().ID
	r.m.RecordPagerTier(id, false)
	raises, keyed := r.m.RecordPagerTier(id, true)
	require.True(t, raises && keyed)

	body, err := json.Marshal(r.m.Export())
	require.NoError(t, err)
	var recs []Record
	require.NoError(t, json.Unmarshal(body, &recs))
	require.True(t, recs[0].PagedBelowPage && recs[0].PageRaised)

	fresh := newRig(t, Config{})
	fresh.m.Restore(recs, at(time.Minute))
	raises, keyed = fresh.m.RecordPagerTier(id, true)
	assert.False(t, raises, "raised once only")
	assert.True(t, keyed)
	assert.True(t, fresh.m.ClosePageKeys(id))
	assert.False(t, fresh.m.ClosePageKeys(id), "the resolve forgot it")
}

// Half the raise survives too: heard below Page before the restart, the
// first page after it raises.
func TestHeardBelowPageSurvivesARestart(t *testing.T) {
	r, _ := pagedRig(t)
	id := r.only().ID
	r.m.RecordPagerTier(id, false)

	fresh := newRig(t, Config{})
	fresh.m.Restore(r.m.Export(), at(time.Minute))
	raises, keyed := fresh.m.RecordPagerTier(id, true)
	assert.True(t, raises && keyed)
}

// A record saved before the raise existed restores without it: the
// incident pages as it did then, under its one key.
func TestOldRecordRestoresWithoutRaise(t *testing.T) {
	r, _ := pagedRig(t)
	id := r.only().ID
	recs := r.m.Export()
	recs[0].Marks = Marks{}

	fresh := newRig(t, Config{})
	fresh.m.Restore(recs, at(time.Minute))
	raises, keyed := fresh.m.RecordPagerTier(id, true)
	assert.False(t, raises || keyed)
}
