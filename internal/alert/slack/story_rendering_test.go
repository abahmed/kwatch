package slack

import (
	"context"
	"strings"
	"testing"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

func TestStructuredSlackNotificationRendersStoryAcrossModes(t *testing.T) {
	tests := []struct {
		name  string
		build func(*testing.T) (*Slack, *string)
	}{
		{
			name: "token",
			build: func(t *testing.T) (*Slack, *string) {
				s := &Slack{}
				var text string
				s.postBlocksFn = func(
					blocks *slackClient.Blocks, _ string,
				) (string, error) {
					text = flattenSlackBlocks(blocks)
					return "123.456", nil
				}
				return s, &text
			},
		},
		{
			name: "webhook",
			build: func(t *testing.T) (*Slack, *string) {
				s := newTestSlack(map[string]interface{}{
					"webhook": "test-webhook",
				}, "dev")
				var text string
				s.send = func(
					_ string, msg *slackClient.WebhookMessage,
				) error {
					text = msg.Text
					return nil
				}
				return s, &text
			},
		},
		{
			name: "compact",
			build: func(t *testing.T) (*Slack, *string) {
				s := newTestSlack(map[string]interface{}{
					"webhook": "test-webhook", "compact": true,
				}, "dev")
				var text string
				s.send = func(
					_ string, msg *slackClient.WebhookMessage,
				) error {
					text = msg.Text
					return nil
				}
				return s, &text
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, text := test.build(t)
			require.NoError(t, s.SendNotification(
				context.Background(), storyNotification(),
			))
			require.Contains(t, *text, "Node worker-a is unhealthy.")
			require.Contains(t, *text, "This affects checkout.")
			require.NotContains(t, *text, "dev/dev/api")
			require.NotContains(t, *text, "Cause:")
			require.NotContains(t, *text, "Impact:")
			require.NotContains(t, *text, "Location:")
		})
	}
}

func TestSlackFallbackPreservesInsightStory(t *testing.T) {
	s := newTestSlack(map[string]interface{}{
		"webhook": "test-webhook",
	}, "dev")
	var text string
	s.send = func(_ string, msg *slackClient.WebhookMessage) error {
		text = msg.Text
		return nil
	}

	insight := &insight.Insight{
		Cause:      "node worker-a is unhealthy",
		CauseState: insight.CauseConfirmed,
		Confidence: 0.9,
		Evidence:   []string{"node reports NotReady"},
		Impact:     "affects checkout",
	}
	require.NoError(t, s.SendIncidentWithInsight(
		context.Background(), testIncident(), model.ActionCreate, insight,
	))
	require.Contains(t, text, "Node worker-a is unhealthy.")
	require.Contains(t, text, "This affects checkout.")
}

func storyNotification() *message.Notification {
	return &message.Notification{
		DeliveryID: "incident:1:create", ConversationKey: "incident:1",
		Action: model.ActionCreate,
		Summary: message.NotificationSummary{
			Emoji: "🔴", Title: "Pod is unavailable",
			Location: message.Location{
				Namespace: "dev", Resource: "pod", Name: "dev/api",
			},
			Story: "Node worker-a is unhealthy. This affects checkout.",
		},
	}
}

func flattenSlackBlocks(blocks *slackClient.Blocks) string {
	var text strings.Builder
	for _, block := range blocks.BlockSet {
		section, ok := block.(slackClient.SectionBlock)
		if ok && section.Text != nil {
			text.WriteString(section.Text.Text)
			text.WriteByte('\n')
		}
	}
	return text.String()
}
