package slack

import (
	"context"
	"sync"
	"testing"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

type recordedPost struct {
	blocks   *slackClient.Blocks
	threadTS string
}

type postRecorder struct {
	mu    sync.Mutex
	posts []recordedPost
}

func (r *postRecorder) post(
	blocks *slackClient.Blocks, threadTS string,
) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.posts = append(r.posts, recordedPost{blocks, threadTS})
	if threadTS == "" {
		return "root-ts", nil
	}
	return threadTS, nil
}

func threadedSlack(t *testing.T) (*Slack, *postRecorder) {
	t.Helper()
	s := newTestSlack(map[string]interface{}{
		"token": "xoxb-test", "channel": "#alerts",
	}, "dev")
	require.NotNil(t, s)
	s.apiClient = nil
	recorder := &postRecorder{}
	s.postBlocksFn = recorder.post
	return s, recorder
}

// firstText reads the first section's text; decoded JSON yields pointers.
func firstText(b *slackClient.Blocks) string {
	switch block := b.BlockSet[0].(type) {
	case slackClient.SectionBlock:
		return block.Text.Text
	case *slackClient.SectionBlock:
		return block.Text.Text
	}
	return ""
}

func TestSlackIncidentPostsShortRootThenNoteInThread(t *testing.T) {
	s, recorder := threadedSlack(t)
	m := providertest.Announce()

	require.NoError(t, s.SendIncident(context.Background(), m))

	require.Len(t, recorder.posts, 2)
	require.Empty(t, recorder.posts[0].threadTS)
	require.Equal(t, m.Short, firstText(recorder.posts[0].blocks))
	require.Equal(t, "root-ts", recorder.posts[1].threadTS)
	require.Equal(t, m.Note, firstText(recorder.posts[1].blocks))
	require.Len(t, recorder.posts[1].blocks.BlockSet, 2, "output block")
	require.Equal(t, "root-ts", s.SnapshotThreads()[m.Key])
}

func TestSlackIncidentUpdateStaysInThread(t *testing.T) {
	s, recorder := threadedSlack(t)
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))

	require.NoError(t, s.SendIncident(ctx, providertest.Update()))

	require.Len(t, recorder.posts, 3)
	require.Equal(t, "root-ts", recorder.posts[2].threadTS)
	require.Equal(t, providertest.Update().Note,
		firstText(recorder.posts[2].blocks))
}

func TestSlackIncidentResolvedForgetsThread(t *testing.T) {
	s, _ := threadedSlack(t)
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))

	require.NoError(t, s.SendIncident(ctx, providertest.Resolve()))

	require.Empty(t, s.SnapshotThreads())
}

func TestSlackRestoredThreadIsReused(t *testing.T) {
	s, recorder := threadedSlack(t)
	s.RestoreThreads(map[string]string{
		providertest.Key: "saved-ts", "x": "",
	})

	require.NoError(t, s.SendIncident(
		context.Background(), providertest.Update()))

	require.Len(t, recorder.posts, 1)
	require.Equal(t, "saved-ts", recorder.posts[0].threadTS)
	require.NotContains(t, s.SnapshotThreads(), "x")
}

func TestSlackRestoreKeepsLiveThread(t *testing.T) {
	s, _ := threadedSlack(t)
	require.NoError(t, s.SendIncident(
		context.Background(), providertest.Announce()))

	s.RestoreThreads(map[string]string{providertest.Key: "stale-ts"})

	require.Equal(t, "root-ts", s.SnapshotThreads()[providertest.Key])
}

func TestSlackResolveAsFirstMessageKeepsNoConversation(t *testing.T) {
	s, recorder := threadedSlack(t)

	require.NoError(t, s.SendIncident(
		context.Background(), providertest.Resolve()))

	require.Len(t, recorder.posts, 2, "root and note are still posted")
	require.Empty(t, s.SnapshotThreads(),
		"a resolved conversation is not persisted")
}
