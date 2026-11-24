package incident

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An incident is introduced at most once in its whole life: the mark is
// saved with its record, so neither a restart nor a reopening under
// the same ID introduces it again.
func TestIntroductionIsOnceForTheIncidentsLife(t *testing.T) {
	r, id, _ := rolledUpRig(t)
	_, ok := r.m.Introduce(id)
	require.True(t, ok)
	_, ok = r.m.Introduce(id)
	assert.False(t, ok, "introduced twice in one run")

	records := r.m.Export()
	require.Len(t, records, 1)
	assert.True(t, records[0].Introduced, "the mark is not saved")

	again := newRig(t, Config{})
	again.m.Restore(records, time.Time{})
	_, ok = again.m.Introduce(id)
	assert.False(t, ok, "introduced again after a restart")
}

// An introduction never repeats what the incident's own last message
// said: with the same fingerprint it would be the same message again.
func TestNoIntroductionOfWhatWasAlreadySaid(t *testing.T) {
	r, id, _ := rolledUpRig(t)
	p := r.m.incidents[id]
	r.m.NoteSaid(id, fingerprint(p))
	_, ok := r.m.Introduce(id)
	assert.False(t, ok, "introduced with the fingerprint already said")

	r.m.NoteSaid(id, "an older message")
	_, ok = r.m.Introduce(id)
	assert.True(t, ok, "a new content may be introduced")
}

// A restart rebuilds the members of a restored incident. The former
// member that comes back with the same reason joined long ago: its
// joining is not written on the timeline again. A new finding is.
func TestRestoredMemberRejoiningWritesNoNote(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	records := r.m.Export()
	joined := func(p Incident, name string) int {
		n := 0
		for _, e := range p.Timeline {
			if strings.Contains(e.Text, "on "+name) {
				n++
			}
		}
		return n
	}
	before := joined(r.only(), "web")
	require.Equal(t, 1, before)

	again := newRig(t, Config{})
	restart := at(2 * time.Hour)
	again.m.Restore(records, restart.Add(10*time.Minute))
	again.raise(restart, web)
	assert.Equal(t, before, joined(again.only(), "web"),
		"the restart wrote the join note again")

	api := podSig("api")
	again.raise(restart.Add(time.Minute), api)
	if p := again.of(api.Entity); joined(p, "api") != 1 {
		t.Fatalf("a new member wrote no note: %+v", p.Timeline)
	}
}
