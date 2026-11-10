package announce

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
)

// A held member that paged and then eases to notify (the previous
// version serves again) tells the paging tools so: the outage message
// carries it to chat, but the pager would keep a critical alert.
func TestHeldOutageMemberLoweringReachesThePagers(t *testing.T) {
	paged := shopRecord(0, incident.Page, incident.Open)
	paged.Paged = true
	c, sent := foldHarness(append(settlingRecords(4), paged)...)
	ctx := context.Background()
	var first []incident.Decision
	for i := range 5 {
		first = append(first,
			outageDecision("shop", i, foldNow.Add(-8*time.Minute)))
	}
	first[0].Incident.Tier = incident.Page
	rest, _ := c.CollectOutages(ctx, foldNow, first)
	require.Empty(t, rest, "the outage is forming: all held")
	require.True(t, c.env.Incidents.PageOpen("shop-0"))
	before := pagesIn(*sent)

	lowered := incident.Decision{Action: incident.Update,
		Reason:   incident.ReasonTierLowered,
		Incident: first[0].Incident}
	lowered.Incident.Tier = incident.Notify
	rest, _ = c.CollectOutages(ctx, foldNow.Add(5*time.Second),
		[]incident.Decision{lowered})

	assert.Empty(t, rest, "held: the outage carries it to chat")
	assert.Equal(t, before+1, pagesIn(*sent),
		"the lowering reaches the paging tools")
}
