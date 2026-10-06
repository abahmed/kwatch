package slack

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

// reopenWindow stands in for incident.RepageWindow, which compose puts
// on the resolve message.
const reopenWindow = 2 * time.Hour

// reopenable is a resolve of an incident that may reopen.
func reopenable() notification.Message {
	m := providertest.Resolve()
	m.ReopenWithin = reopenWindow
	return m
}

// steppedClock is a clock the test moves by hand.
type steppedClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *steppedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *steppedClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func clockedSlack(t *testing.T) (*Slack, *postRecorder, *steppedClock) {
	t.Helper()
	s, recorder := threadedSlack(t)
	c := &steppedClock{now: time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)}
	s.clockSource = clock.Clock(c)
	return s, recorder, c
}

// A resolve that may reopen keeps its thread, and the "failing again"
// update replies in it instead of starting a new top-level message.
func TestSlackReopenRepliesInTheOldThread(t *testing.T) {
	s, recorder, clk := clockedSlack(t)
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	require.NoError(t, s.SendIncident(ctx, reopenable()))
	require.Equal(t, "root-ts", threadTS(s, providertest.Key))

	clk.advance(20 * time.Minute)
	require.NoError(t, s.SendIncident(ctx, providertest.Update()))

	last := recorder.posts[len(recorder.posts)-1]
	require.Equal(t, "root-ts", last.threadTS, "a reply, not a new root")
	require.Len(t, recorder.posts, 3)
}

// After the reopen window the thread is dropped, so a late failure opens
// a new conversation, and the old record is not persisted.
func TestSlackThreadIsDroppedAfterTheReopenWindow(t *testing.T) {
	s, recorder, clk := clockedSlack(t)
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	require.NoError(t, s.SendIncident(ctx, reopenable()))

	clk.advance(reopenWindow + time.Minute)
	require.Empty(t, s.SnapshotThreads(), "expired threads are not saved")
	require.NoError(t, s.SendIncident(ctx, providertest.Update()))

	last := recorder.posts[len(recorder.posts)-1]
	require.Equal(t, "", last.threadTS, "a new top-level message")
}

// A resolve that cannot reopen still forgets its thread at once.
func TestSlackResolveWithoutReopenForgetsThread(t *testing.T) {
	s, _, _ := clockedSlack(t)
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	require.NoError(t, s.SendIncident(ctx, providertest.Resolve()))
	require.Empty(t, s.SnapshotThreads())
}

// The kept thread and its window survive a restart.
func TestSlackKeptThreadSurvivesRestart(t *testing.T) {
	s, _, clk := clockedSlack(t)
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	require.NoError(t, s.SendIncident(ctx, reopenable()))
	saved := s.SnapshotThreads()

	again, recorder, clk2 := clockedSlack(t)
	clk2.now = clk.Now().Add(30 * time.Minute)
	again.RestoreThreads(saved)
	require.NoError(t, again.SendIncident(ctx, providertest.Update()))
	require.Equal(t, "root-ts", recorder.posts[0].threadTS)

	late, recorder, clk3 := clockedSlack(t)
	clk3.now = clk.Now().Add(reopenWindow + time.Minute)
	late.RestoreThreads(saved)
	require.NoError(t, late.SendIncident(ctx, providertest.Update()))
	require.Equal(t, "", recorder.posts[0].threadTS)
}

// The root shows the failing marker again on a reopen, and the resolved
// one after the next resolve.
func TestSlackRootMarkerFollowsResolveReopenResolve(t *testing.T) {
	s, rec := namedChannelSlack(t, func(string) string { return okPost })
	ctx := context.Background()
	steps := []notification.Message{
		providertest.Announce(), reopenable(), downgrade(), reopenable(),
	}
	for _, m := range steps {
		require.NoError(t, s.SendIncident(ctx, m))
	}
	var edits []string
	for _, req := range rec.Requests() {
		if req.Path == "/chat.update" {
			edits = append(edits, formOf(t, req).Get("text"))
		}
	}
	require.Len(t, edits, 3, "resolve, reopen, resolve")
	require.True(t, strings.HasPrefix(edits[0], "✅"), edits[0])
	require.True(t, strings.HasPrefix(edits[1], "🟠"), edits[1])
	require.True(t, strings.HasPrefix(edits[2], "✅"), edits[2])
}
