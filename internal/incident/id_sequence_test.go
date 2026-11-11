package incident

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An incident that is forgotten takes its sequence number out of the
// records. A restart must not give that number out again: the ID was
// announced, and a new incident with it would continue the old thread.
func TestRestoreNeverReusesTheIDOfAForgottenIncident(t *testing.T) {
	src := newRig(t, Config{})
	src.raise(at(0), podSig("web"))
	src.raise(at(time.Second), podSig("api"))
	gone := src.idOf(podSig("api").Entity)
	require.True(t, strings.HasSuffix(gone, "-0002"), gone)
	src.m.mu.Lock()
	src.m.forget(src.m.incidents[gone])
	src.m.mu.Unlock()

	// Through JSON, as the store saves it.
	raw, err := json.Marshal(src.m.Export())
	require.NoError(t, err)
	var records []Record
	require.NoError(t, json.Unmarshal(raw, &records))
	require.Len(t, records, 1)
	dst := newRig(t, Config{IDNonce: "0000"})
	dst.m.Restore(records, time.Time{})
	dst.raise(at(time.Minute), podSig("db"))

	fresh := dst.idOf(podSig("db").Entity)
	assert.NotEqual(t, gone, fresh)
	assert.True(t, strings.HasSuffix(fresh, "-7f3a-0003"), fresh)
}

// The mark is written only when a forgotten ID was higher than every
// saved one, and only on one record: an ordinary save changes nothing.
func TestSequenceMarkIsOnlyWrittenWhenNeeded(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	r.raise(at(time.Second), podSig("api"))

	for _, record := range r.m.Export() {
		assert.Zero(t, record.SeqHigh)
	}
}
