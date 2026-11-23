package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// A node lost the same way soon after the outage was announced joins
// it: the page already told, and the new node is an update, not a page.
func TestNodeLostSoonAfterTheAnnouncementJoinsTheOutage(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), lostNode("n1"))
	wantAction(t, r.tick(at(DefaultPageSettle)), Announce, ReasonSettled)

	r.raise(at(time.Minute), lostNode("n2"))

	assert.Len(t, r.m.Export(), 1)
	assert.True(t, r.only().Grouped())
	assert.Len(t, r.only().Merged, 1)
}
