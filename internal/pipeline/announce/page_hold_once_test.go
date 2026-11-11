package announce

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An incident that already paged is not paged again when an outage hold
// or the startup summary takes its announcement: the earlier page stays
// the one page.
func TestOutageHoldDoesNotPageAnIncidentThatPaged(t *testing.T) {
	c, sent := foldHarness(pageRecord("a", "shop"))
	d := pageAnnouncement("a", "shop")
	d.PagedAlready = true
	h := c.newOutageHold("shop", foldNow)

	c.absorbAnnounce(context.Background(), foldNow, h, d)

	assert.Zero(t, pagesIn(*sent), "no second page")
	require.True(t, h.Paged, "the earlier page stands for the hold")
	assert.Equal(t, "a", h.PagedID)
}

func TestStartupHoldDoesNotPageAnIncidentThatPaged(t *testing.T) {
	c, sent := foldHarness(pageRecord("a", "shop"))
	c.Startup.ColdStart = true
	c.Startup.AwaitSync = true
	d := pageAnnouncement("a", "shop")
	d.PagedAlready = true

	held := c.holdStartup(context.Background(), foldNow, d)

	assert.True(t, held)
	assert.Zero(t, pagesIn(*sent), "no second page")
}
