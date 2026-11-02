package announce

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListingNoteResolvedRecordsListedIncident(t *testing.T) {
	l := Listing{Incidents: []string{"a", "b"}}

	assert.True(t, l.noteResolved("a"))
	assert.Equal(t, []string{"a"}, l.Resolved)
}

func TestListingNoteResolvedIgnoresUnlistedIncident(t *testing.T) {
	l := Listing{Incidents: []string{"a"}}

	assert.False(t, l.noteResolved("z"))
	assert.Empty(t, l.Resolved)
}

func TestListingNoteResolvedRecordsOnce(t *testing.T) {
	l := Listing{Incidents: []string{"a"}}

	assert.True(t, l.noteResolved("a"))
	assert.True(t, l.noteResolved("a"))
	assert.Equal(t, []string{"a"}, l.Resolved)
}

func TestListingNoteResolvedOnEmptyListing(t *testing.T) {
	var l Listing

	assert.False(t, l.noteResolved("a"))
	assert.Empty(t, l.Resolved)
}

func TestCollectorSpokeOnItsOwnFollowsStartupSummary(t *testing.T) {
	c := New(Env{})
	c.Startup.Summary.Listing = Listing{
		Incidents: []string{"a", "b"}, Followed: []string{"a"}}

	assert.True(t, c.SpokeOnItsOwn("a"))
	assert.False(t, c.SpokeOnItsOwn("b"), "listed but never spoke")
	assert.False(t, c.SpokeOnItsOwn("z"), "not listed at all")
}

func TestCollectorSpokeOnItsOwnFollowsRollups(t *testing.T) {
	c := New(Env{})
	c.Startup.Summary.Rollups = []Listing{
		{Incidents: []string{"a"}},
		{Incidents: []string{"b", "c"}, Followed: []string{"c"}},
	}

	assert.False(t, c.SpokeOnItsOwn("a"))
	assert.False(t, c.SpokeOnItsOwn("b"))
	assert.True(t, c.SpokeOnItsOwn("c"))
}

func TestCollectorSpokeOnItsOwnEmptyState(t *testing.T) {
	assert.False(t, New(Env{}).SpokeOnItsOwn("a"))
}
