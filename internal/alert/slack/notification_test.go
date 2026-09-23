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
