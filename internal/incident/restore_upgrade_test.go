package incident

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fieldsAddedAfterRC16 are the persisted Record fields that rc.17 added.
// A state file saved by rc.16 has none of them.
var fieldsAddedAfterRC16 = []string{
	"FormerRoot", "RerootedAt", "RejectionSeenAt",
}

// rc16Records exports an announced incident and strips the fields rc.16
// never wrote, through JSON, the way the state file stores it.
func rc16Records(t *testing.T) []Record {
	t.Helper()
	r := newRig(t, Config{})
	announced(t, r, podSig("web"))
	var out []Record
	for _, rec := range r.m.Export() {
		raw, err := json.Marshal(rec)
		require.NoError(t, err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &fields))
		for _, name := range fieldsAddedAfterRC16 {
			delete(fields, name)
		}
		raw, err = json.Marshal(fields)
		require.NoError(t, err)
		var old Record
		require.NoError(t, json.Unmarshal(raw, &old))
		out = append(out, old)
	}
	require.NotEmpty(t, out)
	return out
}

// A record without the fields rc.17 added restores as an incident with
// no former root and no refused request, and resolves like any other.
func TestRC16RecordRestoresWithoutNewFields(t *testing.T) {
	recs := rc16Records(t)
	assert.True(t, recs[0].FormerRoot.IsZero())
	assert.True(t, recs[0].RerootedAt.IsZero())
	assert.True(t, recs[0].RejectionSeenAt.IsZero())

	fresh := newRig(t, Config{})
	fresh.m.Restore(recs, at(8*time.Hour+10*time.Minute))
	p := fresh.only()
	assert.True(t, p.formerRoot.IsZero())
	assert.True(t, p.rerootedAt.IsZero())
	assert.False(t, p.rejectionSeen)
	assert.True(t, p.rejectionAt.IsZero())

	got := restoreAfterGap(t, func(r *Record) {
		*r = recs[0]
	}, 8*time.Hour, 10*time.Minute)
	assert.False(t, got.IsZero(), "never resolved")
}
