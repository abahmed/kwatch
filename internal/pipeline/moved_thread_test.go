package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

// movedResolve is the resolve of a roll-up member whose failures another
// incident took over.
func movedResolve(id string) incident.Decision {
	p := incident.Incident{ID: id, Tier: incident.Notify,
		SupersededBy: "taker",
		Root:         inventory.CoreID(kube.KindDeployment, "shop", id)}
	p.Delivery.MarkRolledUp(1)
	return incident.Decision{Action: incident.Resolve,
		Reason: incident.ReasonSuperseded, Incident: p}
}

// sendMoved sends the "Moved" resolve of id, given what the roll-up
// listing knows about it, and returns the message.
func sendMoved(t *testing.T, listing announce.Listing) notification.Message {
	t.Helper()
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0),
		ch: make(chan time.Time)}
	var sent []notification.Message
	engine := newTestEngine(t, clock,
		func(_ context.Context, _ incident.Decision,
			m notification.Message,
		) {
			sent = append(sent, m)
		}, nil)
	engine.announcer.collect.Startup.Summary.Rollups = []announce.Listing{
		listing}

	engine.announcer.send(context.Background(), movedResolve("api"),
		clock.now)

	require.Len(t, sent, 1)
	return sent[0]
}

// A roll-up member that never spoke on its own has no thread: its "Moved"
// close goes to the alert-tracking providers alone.
func TestMovedRollupMemberWithoutThreadClosesForPagersOnly(t *testing.T) {
	msg := sendMoved(t, announce.Listing{Key: "rollup",
		Incidents: []string{"api"}})

	assert.True(t, msg.PagingOnly)
}

// One that spoke on its own after the roll-up has a chat thread, and the
// thread must be closed: pagers-only left it open for ever.
func TestMovedRollupMemberThatSpokeClosesItsChatThread(t *testing.T) {
	msg := sendMoved(t, announce.Listing{Key: "rollup",
		Incidents: []string{"api"}, Followed: []string{"api"}})

	assert.False(t, msg.PagingOnly, "chat gets the close too")
}
