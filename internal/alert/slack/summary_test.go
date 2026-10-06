package slack

import (
	"context"
	"testing"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notification"
)

func summaryMessage(
	key string, status notification.Status,
) notification.Message {
	return notification.Message{
		Key: key, Status: status,
		Short: "short lead", Note: "full note",
	}
}

func TestSlackSummaryIsOneTopLevelMessageWithTheNote(t *testing.T) {
	for _, key := range []string{
		"startup/x", "rollup/x", "digest/x",
	} {
		s, recorder := threadedSlack(t)
		m := summaryMessage(key, notification.StatusCritical)

		require.NoError(t, s.SendIncident(context.Background(), m))

		require.Len(t, recorder.posts, 1, key)
		require.Empty(t, recorder.posts[0].threadTS, key)
		require.Equal(t, "full note", firstText(recorder.posts[0].blocks))
		require.Empty(t, s.SnapshotThreads(), "never updated: no thread")
	}
}

func TestSlackSummaryResolveIsOneShortStandaloneLine(t *testing.T) {
	s, recorder := threadedSlack(t)
	m := summaryMessage("rollup/x", notification.StatusResolved)

	require.NoError(t, s.SendIncident(context.Background(), m))

	require.Len(t, recorder.posts, 1)
	require.Empty(t, recorder.posts[0].threadTS)
	require.Equal(t, "short lead", firstText(recorder.posts[0].blocks))
}

func TestSlackOutputIsLabelledAndSeparateFromTheNote(t *testing.T) {
	m := notification.Message{
		Key: "inc-1", Note: "run kubectl describe pod x -n staging",
		Output: []string{"line one"},
	}

	blocks := noteBlocks(m).BlockSet

	require.Len(t, blocks, 3)
	require.Equal(t, m.Note, firstText(noteBlocks(m)))
	require.Contains(t, blockText(blocks[1]), "recent output")
	require.Equal(t, "```line one```", blockText(blocks[2]))
}

func blockText(b slackClient.Block) string {
	return firstText(&slackClient.Blocks{BlockSet: []slackClient.Block{b}})
}
