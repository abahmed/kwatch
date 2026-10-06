package slack

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

// threadTS is the Slack timestamp of the persisted thread for key.
func threadTS(s *Slack, key string) string {
	return decodeThread(s.SnapshotThreads()[key]).ThreadTS
}

// editedBlocks is the text of every block of a chat.update request.
func editedBlocks(t *testing.T, form string) []string {
	t.Helper()
	var blocks []struct {
		Text struct {
			Text string `json:"text"`
		} `json:"text"`
	}
	require.NoError(t, json.Unmarshal([]byte(form), &blocks))
	var out []string
	for _, b := range blocks {
		out = append(out, b.Text.Text)
	}
	return out
}

func TestSlackResolveAfterRestartEditsRootWithAnnouncement(t *testing.T) {
	before, _ := namedChannelSlack(t, func(string) string { return okPost })
	ctx := context.Background()
	require.NoError(t, before.SendIncident(ctx, providertest.Announce()))
	saved := before.SnapshotThreads()

	after, rec := namedChannelSlack(t, func(string) string { return okPost })
	after.RestoreThreads(saved)
	require.NoError(t, after.SendIncident(ctx, providertest.Resolve()))

	requests := rec.Requests()
	require.Len(t, requests, 2, "thread reply, root edit")
	edit := requests[1]
	require.Equal(t, "/chat.update", edit.Path)
	require.Equal(t, "111.1", formOf(t, edit).Get("ts"))
	blocks := editedBlocks(t, formOf(t, edit).Get("blocks"))
	require.Equal(t,
		swapMarker(providertest.Announce().Note,
			notification.MarkerResolved), blocks[0])
	require.True(t, strings.HasPrefix(blocks[0], "✅"))
	require.Contains(t, blocks[2], "panic: out of memory")
	require.Empty(t, after.SnapshotThreads(), "resolve drops the thread")
}

// A conversation restored from a bare timestamp has no stored announcement,
// so its root must not be edited: only the thread grows.
func TestSlackLegacyThreadOnlyRepliesInTheThread(t *testing.T) {
	s, rec := namedChannelSlack(t, func(string) string { return okPost })
	s.RestoreThreads(map[string]string{providertest.Key: "saved-ts"})

	require.NoError(t, s.SendIncident(
		context.Background(), providertest.Resolve()))

	requests := rec.Requests()
	require.Len(t, requests, 1, "thread reply only, no root edit")
	require.Equal(t, "/chat.postMessage", requests[0].Path)
	require.Equal(t, "saved-ts", formOf(t, requests[0]).Get("thread_ts"))
}

func TestSlackLegacyThreadUpdateDoesNotTouchTheRoot(t *testing.T) {
	s, rec := namedChannelSlack(t, func(string) string { return okPost })
	s.RestoreThreads(map[string]string{providertest.Key: "saved-ts"})

	require.NoError(t, s.SendIncident(context.Background(), downgrade()))

	for _, request := range rec.Requests() {
		require.NotEqual(t, "/chat.update", request.Path)
	}
	require.Equal(t, "saved-ts", threadTS(s, providertest.Key))
}

func TestSlackUnreadableStoredRootFallsBackToTimestamp(t *testing.T) {
	state := decodeThread("111.1\n{not json")
	require.Equal(t, "111.1", state.ThreadTS)
	require.Nil(t, state.Root)
}

func TestSlackStoredRootIsBounded(t *testing.T) {
	m := providertest.Announce()
	m.Note += strings.Repeat(" é", maxSectionTextChars)
	m.Output = []string{strings.Repeat("x", maxSectionTextChars*4)}

	value := encodeThread(conversationState{ThreadTS: "1.1", Root: &m})

	require.Less(t, len(value), 4*maxSectionTextChars+2000)
	state := decodeThread(value)
	require.Equal(t, "1.1", state.ThreadTS)
	require.LessOrEqual(t,
		len([]rune(state.Root.NoteText())), maxSectionTextChars)
}

func TestSlackRestoredThreadsRespectSizeCap(t *testing.T) {
	s, _ := threadedSlack(t)
	s.maxThreadMapSize = 2
	root := providertest.Announce()
	saved := map[string]string{}
	for _, key := range []string{"a", "b", "c"} {
		saved[key] = encodeThread(
			conversationState{ThreadTS: key + "-ts", Root: &root})
	}

	s.RestoreThreads(saved)

	snapshot := s.SnapshotThreads()
	require.Len(t, snapshot, 2)
	require.NotContains(t, snapshot, "a", "oldest is evicted")
	require.Equal(t, "c-ts", threadTS(s, "c"))
}

func TestSlackThreadCapEvictsKeptAfterResolveBeforeOpenOnes(t *testing.T) {
	s, _ := threadedSlack(t)
	s.maxThreadMapSize = 2
	later := s.clockSource.Now().Add(time.Hour)
	s.saveConversation("open", conversationState{ThreadTS: "1"})
	s.saveConversation("resolved",
		conversationState{ThreadTS: "2", ReopenUntil: later})

	s.saveConversation("new", conversationState{ThreadTS: "3"})

	require.Equal(t, "1", threadTS(s, "open"), "open thread survives")
	require.Equal(t, "3", threadTS(s, "new"))
	require.Empty(t, threadTS(s, "resolved"))
}
