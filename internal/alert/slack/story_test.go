package slack

import (
	"context"
	"sync"
	"testing"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notice"
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

func story(status notice.Status) notice.Message {
	return notice.Message{
		Key: "problem-1", Status: status, Title: "api is crashing",
		Lines: []string{"api restarted 5 times", "after rollout 12"},
	}
}

func TestSlackStoryPostsRootThenDetailInThread(t *testing.T) {
	s, recorder := threadedSlack(t)

	require.NoError(t, s.SendStory(
		context.Background(), story(notice.StatusCritical)))

	require.Len(t, recorder.posts, 2)
	require.Empty(t, recorder.posts[0].threadTS)
	require.Equal(t, "root-ts", recorder.posts[1].threadTS)
	require.Equal(t, "root-ts", s.SnapshotThreads()["problem-1"])
}

func TestSlackStoryUpdateStaysInThread(t *testing.T) {
	s, recorder := threadedSlack(t)
	ctx := context.Background()
	require.NoError(t, s.SendStory(ctx, story(notice.StatusCritical)))

	require.NoError(t, s.SendStory(ctx, story(notice.StatusWarning)))

	require.Len(t, recorder.posts, 3)
	require.Equal(t, "root-ts", recorder.posts[2].threadTS)
}

func TestSlackStoryResolvedForgetsThread(t *testing.T) {
	s, _ := threadedSlack(t)
	ctx := context.Background()
	require.NoError(t, s.SendStory(ctx, story(notice.StatusCritical)))

	require.NoError(t, s.SendStory(ctx, story(notice.StatusResolved)))

	require.Empty(t, s.SnapshotThreads())
}

func TestSlackRestoredThreadIsReused(t *testing.T) {
	s, recorder := threadedSlack(t)
	s.RestoreThreads(map[string]string{"problem-1": "saved-ts", "x": ""})

	require.NoError(t, s.SendStory(
		context.Background(), story(notice.StatusWarning)))

	require.Len(t, recorder.posts, 1)
	require.Equal(t, "saved-ts", recorder.posts[0].threadTS)
	require.NotContains(t, s.SnapshotThreads(), "x")
}

func TestSlackRestoreKeepsLiveThread(t *testing.T) {
	s, _ := threadedSlack(t)
	require.NoError(t, s.SendStory(
		context.Background(), story(notice.StatusCritical)))

	s.RestoreThreads(map[string]string{"problem-1": "stale-ts"})

	require.Equal(t, "root-ts", s.SnapshotThreads()["problem-1"])
}

func TestSlackCompactStorySendsText(t *testing.T) {
	s := newTestSlack(map[string]interface{}{
		"webhook": "https://hooks.example.test/x", "compact": true,
	}, "dev")
	var text string
	s.send = func(_ string, msg *slackClient.WebhookMessage) error {
		text = msg.Text
		return nil
	}

	require.NoError(t, s.SendStory(
		context.Background(), story(notice.StatusCritical)))

	require.Contains(t, text, "api is crashing")
}
