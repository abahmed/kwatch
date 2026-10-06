package slack

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

const rollupKey = notification.RollupKeyPrefix + "20260102T030405.000Z"

func rollupMessage() notification.Message {
	return notification.Message{
		Key: rollupKey, Revision: 1, Status: notification.StatusCritical,
		Opens: true, Marker: notification.MarkerPage,
		Short:   notification.MarkerPage + " 2 new problems.",
		Note:    notification.MarkerPage + " 2 new problems.",
		Members: []string{providertest.Key, "p-43"},
	}
}

func rollupResolved() notification.Message {
	return notification.Message{
		Key: rollupKey, Revision: 2, Status: notification.StatusResolved,
		Marker: notification.MarkerResolved,
		Short:  notification.MarkerResolved + " All 2 resolved.",
		Note:   notification.MarkerResolved + " All 2 resolved.",
	}
}

func TestSlackRollupMemberResolveRepliesInRollupThread(t *testing.T) {
	s, rec := namedChannelSlack(t, func(string) string { return okPost })
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, rollupMessage()))
	require.NoError(t, s.SendIncident(ctx, providertest.Resolve()))

	requests := rec.Requests()
	require.Len(t, requests, 2, "roll-up post, reply; no root edit")
	require.Equal(t, "/chat.postMessage", requests[1].Path)
	require.Equal(t, "111.1", formOf(t, requests[1]).Get("thread_ts"))
	require.Empty(t, formOf(t, requests[0]).Get("thread_ts"))
	require.Empty(t, threadTS(s, providertest.Key), "resolve drops it")
	require.Equal(t, "111.1", threadTS(s, "p-43"))
}

func TestSlackRollupAllResolvedEditsRollupRoot(t *testing.T) {
	s, rec := namedChannelSlack(t, func(string) string { return okPost })
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, rollupMessage()))
	require.NoError(t, s.SendIncident(ctx, rollupResolved()))

	requests := rec.Requests()
	require.Len(t, requests, 2, "roll-up post, root edit")
	edit := requests[1]
	require.Equal(t, "/chat.update", edit.Path)
	require.Equal(t, "111.1", formOf(t, edit).Get("ts"))
	blocks := editedBlocks(t, formOf(t, edit).Get("blocks"))
	require.Equal(t,
		notification.MarkerResolved+" 2 new problems.", blocks[0])
	require.Empty(t, s.SnapshotThreads()[rollupKey])
}

func TestSlackRollupResolveWithoutRollupPostsStandaloneLine(t *testing.T) {
	s, rec := namedChannelSlack(t, func(string) string { return okPost })
	require.NoError(t, s.SendIncident(
		context.Background(), rollupResolved()))
	requests := rec.Requests()
	require.Len(t, requests, 1)
	require.Equal(t, "/chat.postMessage", requests[0].Path)
}

func TestSlackRollupMappingSurvivesRestart(t *testing.T) {
	before, _ := namedChannelSlack(t, func(string) string { return okPost })
	ctx := context.Background()
	require.NoError(t, before.SendIncident(ctx, rollupMessage()))
	saved := before.SnapshotThreads()

	after, rec := namedChannelSlack(t, func(string) string { return okPost })
	after.RestoreThreads(saved)
	require.NoError(t, after.SendIncident(ctx, providertest.Resolve()))
	require.NoError(t, after.SendIncident(ctx, rollupResolved()))

	requests := rec.Requests()
	require.Len(t, requests, 2, "member reply, roll-up edit")
	require.Equal(t, "111.1", formOf(t, requests[0]).Get("thread_ts"))
	require.Equal(t, "/chat.update", requests[1].Path)
}

func TestSlackRollupMembersAreBoundedByThreadCap(t *testing.T) {
	s, _ := namedChannelSlack(t, func(string) string { return okPost })
	s.maxThreadMapSize = 2
	require.NoError(t, s.SendIncident(
		context.Background(), rollupMessage()))
	require.LessOrEqual(t, len(s.SnapshotThreads()), 2)
}

func TestSlackThreadEncodingKeepsRollupMember(t *testing.T) {
	state := conversationState{ThreadTS: "1.2", Rollup: rollupKey}
	require.Equal(t, state, decodeThread(encodeThread(state)))
}

// Two roll-ups that name the same members, sent at once, must not
// deadlock on the member locks they both take.
func TestSlackConcurrentRollupsWithSharedMembersDoNotDeadlock(t *testing.T) {
	s, _ := threadedSlack(t)
	members := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	reversed := make([]string, len(members))
	for i, key := range members {
		reversed[len(members)-1-i] = key
	}
	done := make(chan struct{}, 2)
	for _, list := range [][]string{members, reversed} {
		go func() {
			unlock := s.lockConversations(list)
			unlock()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("roll-up locking deadlocked")
		}
	}
}

func TestSlackRollupHoldsMemberLocksWhileSaving(t *testing.T) {
	s, _ := threadedSlack(t)
	unlock := s.lockConversations([]string{"x", "y"})
	acquired := make(chan struct{})
	go func() {
		lock := s.conversationLock("y")
		lock.Lock()
		close(acquired)
		lock.Unlock()
	}()
	select {
	case <-acquired:
		t.Fatal("a member lock was free while the roll-up held it")
	case <-time.After(50 * time.Millisecond):
	}
	unlock()
	select {
	case <-acquired:
	case <-time.After(10 * time.Second):
		t.Fatal("the member lock was never released")
	}
}
