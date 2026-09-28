package slack

import (
	"context"
	"errors"
	"sync"
	"testing"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

func TestStructuredNotificationRetryDoesNotDuplicateRoot(t *testing.T) {
	s := &Slack{clockSource: clock.RealClock{}}
	posts := 0
	failDetails := true
	s.postBlocksFn = func(
		_ *slackClient.Blocks, threadTS string,
	) (string, error) {
		posts++
		if threadTS != "" && failDetails {
			failDetails = false
			return "", errors.New("detail post failed")
		}
		return "123.456", nil
	}
	n := &message.Notification{
		DeliveryID:      "incident:1:create",
		ConversationKey: "incident",
		Action:          model.ActionCreate,
		Summary: message.NotificationSummary{
			Emoji: "🔴", Title: "Container keeps crashing",
		},
		Details: []message.NotificationSection{{
			Title: "Events", Lines: []string{"Back-off restarting"},
		}},
	}
	require.Error(t, s.SendNotification(context.Background(), n))
	require.NoError(t, s.SendNotification(context.Background(), n))
	require.Equal(t, 3, posts)
}

func TestStructuredNotificationConcurrentCreatePostsOneRoot(t *testing.T) {
	s := &Slack{}
	var mu sync.Mutex
	posts := 0
	s.postBlocksFn = func(
		_ *slackClient.Blocks, threadTS string,
	) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		posts++
		if threadTS == "" {
			return "123.456", nil
		}
		return "", nil
	}
	n := &message.Notification{
		DeliveryID: "incident:1:create", ConversationKey: "incident",
		Action:  model.ActionCreate,
		Summary: message.NotificationSummary{Emoji: "🔴", Title: "Failure"},
	}
	start := make(chan struct{})
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			if err := s.SendNotification(context.Background(), n); err != nil {
				t.Errorf("SendNotification() error = %v", err)
			}
		}()
	}
	close(start)
	wait.Wait()
	mu.Lock()
	defer mu.Unlock()
	if posts != 1 {
		t.Fatalf("post count = %d, want one root", posts)
	}
}

func TestSlackStructuredConversationStateRoundTrips(t *testing.T) {
	original := &Slack{conversations: map[string]conversationState{
		"incident": {
			ThreadTS: "123.456", RootDeliveryID: "incident:1:create",
			LastDeliveryID: "incident:2:update", LastDetailHash: "details",
		},
	}}
	saved := original.SnapshotThreads()
	restored := &Slack{}
	restored.RestoreThreads(saved)

	restored.mu.Lock()
	state := restored.conversations["incident"]
	restored.mu.Unlock()
	require.Equal(t, conversationState{
		ThreadTS: "123.456", RootDeliveryID: "incident:1:create",
		LastDeliveryID: "incident:2:update", LastDetailHash: "details",
	}, state)
}

func TestPostWithThreadFallbackReturnsThreadTSOnSuccess(t *testing.T) {
	blocks := &slackClient.Blocks{}
	post := func(
		_ context.Context, _ *slackClient.Blocks, _ string,
	) (string, error) {
		return "999.888", nil
	}
	ts, err := postWithThreadFallback(context.Background(), post,
		blocks, "123.456")
	require.NoError(t, err)
	require.Equal(t, "123.456", ts)
}

func TestPostWithThreadFallbackReturnsErrorOnNonStaleThreadError(t *testing.T) {
	blocks := &slackClient.Blocks{}
	expectedErr := errors.New("some other error")
	post := func(
		_ context.Context, _ *slackClient.Blocks, _ string,
	) (string, error) {
		return "", expectedErr
	}
	ts, err := postWithThreadFallback(context.Background(), post,
		blocks, "123.456")
	require.Equal(t, expectedErr, err)
	require.Equal(t, "123.456", ts)
}

func TestPostWithThreadFallbackRetriesToTopLevelOnStaleThread(t *testing.T) {
	blocks := &slackClient.Blocks{}
	calls := 0
	post := func(
		_ context.Context, _ *slackClient.Blocks, threadTS string,
	) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("error: invalid_thread_ts")
		}
		if threadTS == "" {
			return "new-ts", nil
		}
		return "", errors.New("unexpected threadTS in fallback")
	}
	ts, err := postWithThreadFallback(context.Background(), post,
		blocks, "old-ts")
	require.NoError(t, err)
	require.Equal(t, "new-ts", ts)
	require.Equal(t, 2, calls)
}

func TestPostWithThreadFallbackDoesNotRetryWhenThreadTSEmpty(t *testing.T) {
	blocks := &slackClient.Blocks{}
	calls := 0
	post := func(
		_ context.Context, _ *slackClient.Blocks, threadTS string,
	) (string, error) {
		calls++
		return "", errors.New("error: invalid_thread_ts")
	}
	ts, err := postWithThreadFallback(context.Background(), post,
		blocks, "")
	require.Error(t, err)
	require.Equal(t, "", ts)
	require.Equal(t, 1, calls)
}

func TestPostWithThreadFallbackFallbackError(t *testing.T) {
	blocks := &slackClient.Blocks{}
	calls := 0
	post := func(
		_ context.Context, _ *slackClient.Blocks, threadTS string,
	) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("error: thread_not_found")
		}
		return "", errors.New("fallback also failed")
	}
	ts, err := postWithThreadFallback(context.Background(), post,
		blocks, "old-ts")
	require.Error(t, err)
	require.Equal(t, "old-ts", ts)
	require.Equal(t, 2, calls)
}
