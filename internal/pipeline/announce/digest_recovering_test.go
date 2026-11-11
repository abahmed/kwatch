package announce

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// A listed problem that recovers (its hold can last hours) or that
// flaps and is healthy now is not "still failing": the digest leaves it
// out. An open one stays.
func TestRecoveringProblemIsNotListedAsStillFailing(t *testing.T) {
	recovering := demotedRecord("a")
	recovering.State = incident.Recovering
	recovering.RecoveringSince = foldNow.Add(-time.Minute)
	healthyFlapper := demotedRecord("b")
	healthyFlapper.State = incident.Flapping
	c, _ := foldHarness(recovering, healthyFlapper, demotedRecord("c"))

	items := c.ongoingProblems(foldNow)

	require.Len(t, items, 1)
	assert.Equal(t, "c", items[0].p.ID)
}

func TestStillFailing(t *testing.T) {
	failing := incident.Incident{Root: demotedRecord("a").Root}
	failing.Members = failingMember(failing)
	for state, want := range map[incident.State]bool{
		incident.Open: true, incident.Flapping: true,
		incident.Recovering: true, incident.Resolved: false,
		incident.Settling: false,
	} {
		failing.State = state
		assert.Equal(t, want, stillFailing(failing), state)
	}
}

// A digest that panics while it is written is due again after
// digestRetry. Before, it stayed due: the engine's timer then never
// waited, and every loop iteration panicked and logged again.
func TestDigestThatPanicsBacksOff(t *testing.T) {
	writer := compose.Writer{}
	c := New(Env{
		Incidents: incident.NewManager(incident.Config{}, nil),
		Messages:  writer, Write: writer.Write,
		SaveStartup: func(StartupState) {},
		Sink: func(
			context.Context, incident.Decision, notification.Message,
		) {
			panic("a bad entry")
		},
	})
	c.Low.Since = foldNow.Add(-DigestWindow)
	c.Low.Opened = []incident.Decision{causeRevised(c, "a")}

	assert.Panics(t, func() {
		c.flushDigest(context.Background(), foldNow)
	}, "the caller contains, counts and logs it")

	assert.Equal(t, foldNow.Add(digestRetry), c.NextDigest())
	assert.Len(t, c.Low.Opened, 1, "the entry is kept for the retry")
}

// A panic after the digest was handed to the sink does not send it
// again: the digest is done, and the retry finds nothing to send.
func TestDigestThatPanicsAfterItWasSentIsNotResent(t *testing.T) {
	writer := compose.Writer{}
	sent := 0
	var c *Collector
	c = New(Env{
		Incidents: incident.NewManager(incident.Config{}, nil),
		Messages:  writer, Write: writer.Write,
		SaveStartup: func(StartupState) {},
		Sink: func(
			context.Context, incident.Decision, notification.Message,
		) {
			sent++
			c.env.Incidents = nil // the bookkeeping after it panics
		},
	})
	c.Low.Since = foldNow.Add(-DigestWindow)
	c.Low.Opened = []incident.Decision{causeRevised(c, "a")}

	assert.Panics(t, func() {
		c.flushDigest(context.Background(), foldNow)
	})

	assert.Equal(t, 1, sent)
	assert.True(t, c.NextDigest().IsZero(), "nothing is due again")
	assert.Empty(t, c.Low.Opened)
}
