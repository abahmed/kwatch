package slack

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSlackWithoutThreadsPostsEveryMessageAtTopLevel(t *testing.T) {
	s, recorder := threadedSlack(t)
	s.DisableThreads()
	ctx := context.Background()

	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	require.NoError(t, s.SendIncident(ctx, providertest.Update()))
	require.NoError(t, s.SendIncident(ctx, providertest.Resolve()))

	require.Len(t, recorder.posts, 3, "no edit, one post per message")
	for _, post := range recorder.posts {
		require.Empty(t, post.threadTS)
	}
	require.Equal(t, providertest.Update().Note,
		firstText(recorder.posts[1].blocks))
	require.Empty(t, s.SnapshotThreads(), "no thread is kept")
}

func TestSlackWithThreadsKeepsUpdatesInTheThread(t *testing.T) {
	s, recorder := threadedSlack(t)
	ctx := context.Background()

	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	require.NoError(t, s.SendIncident(ctx, providertest.Update()))

	require.Equal(t, "root-ts", recorder.posts[1].threadTS)
}
