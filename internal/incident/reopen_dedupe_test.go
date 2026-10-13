package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A flapping update that already said the incident is back leaves
// nothing for "failing again" to add: it is not recorded a second time.
func TestFailingAgainIsNotRecordedAfterAnIdenticalFlappingUpdate(
	t *testing.T,
) {
	r, _ := pagedRig(t)
	p := r.m.incidents[r.only().ID]
	now := at(1000)
	p.State = Flapping
	p.Digest = fingerprint(p)
	p.Pending.ScheduleReopenUpdate(now)
	revision := p.Revision

	d, ok := r.m.reopenUpdate(p, now.Add(time.Hour))

	assert.False(t, ok, "second update with the same content: %+v", d)
	assert.Equal(t, revision, p.Revision, "no revision spent")
	assert.False(t, p.Pending.ReopenOwed(), "the debt is cleared")

	// With content that changed since, the update is still sent.
	p.Digest = "older"
	p.Pending.ScheduleReopenUpdate(now)
	d, ok = r.m.reopenUpdate(p, now.Add(time.Hour))
	require.True(t, ok)
	assert.Equal(t, ReasonFailingAgain, d.Reason)
}
