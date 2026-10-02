package slack

import (
	"context"
	"net/http"
	"testing"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func mockedSend(url string, msg *slackClient.WebhookMessage) error {
	return nil
}

func newTestSlack(
	values map[string]interface{}, clusterName string,
) *Slack {
	return NewSlack(values, clusterName, transport.Dependencies{
		HTTPClient: http.DefaultClient,
		Clock:      clock.RealClock{},
	})
}

// --- webhook mode tests ---

func TestSlackEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	s := newTestSlack(map[string]interface{}{}, "dev")
	assert.Nil(s)
}

func TestSlackWebhook(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "https://example.test/hook",
	}
	s := newTestSlack(configMap, "dev")
	assert.NotNil(s)
	assert.Equal("Slack", s.Name())
}

func TestSlackWebhookWithChannel(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "https://example.test/hook",
		"channel": "#alerts",
	}
	s := newTestSlack(configMap, "dev")
	assert.NotNil(s)
	assert.Equal("#alerts", s.channel)
}

func TestSendMessageWebhook(t *testing.T) {
	assert := assert.New(t)

	s := newTestSlack(map[string]interface{}{
		"webhook": "https://example.test/hook",
		"channel": "test",
	}, "dev")
	assert.NotNil(s)

	s.send = mockedSend
	assert.Nil(s.SendMessage(context.Background(), "test"))
}

func TestSlackWebhookCompactFalse(t *testing.T) {
	assert := assert.New(t)

	s := newTestSlack(map[string]interface{}{
		"webhook": "https://example.test/hook",
		"compact": false,
	}, "dev")
	assert.NotNil(s)
	assert.False(s.compact)
}

// --- token mode tests ---

func TestSlackTokenMode(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token":   "xoxb-test-token",
		"channel": "#alerts",
	}
	s := newTestSlack(configMap, "dev")
	assert.NotNil(s)
	assert.Equal("Slack", s.Name())
	assert.Equal("#alerts", s.channel)
	assert.NotNil(s.apiClient)
	assert.Empty(s.webhook)
}

func TestSlackTokenMissingChannel(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token": "xoxb-test-token",
	}
	s := newTestSlack(configMap, "dev")
	assert.Nil(s)
}

func TestSlackTokenEmptyChannel(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token":   "xoxb-test-token",
		"channel": "",
	}
	s := newTestSlack(configMap, "dev")
	assert.Nil(s)
}

func TestSlackWebhookPreferWebhookOverToken(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "https://hooks.slack.com/test",
		"token":   "",
		"channel": "#alerts",
	}
	s := newTestSlack(configMap, "dev")
	assert.NotNil(s)
	// Empty token should fall through to webhook mode
	assert.Equal("https://hooks.slack.com/test", s.webhook)
	assert.Nil(s.apiClient)
}

func TestSendMessageTokenMode(t *testing.T) {
	assert := assert.New(t)

	s := newTestSlack(map[string]interface{}{
		"token":   "xoxb-test-token",
		"channel": "#alerts",
	}, "dev")
	assert.NotNil(s)

	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_auth"}`))
	}
	s.apiClient = slackClient.New("xoxb-test-token",
		slackClient.OptionHTTPClient(rec.Server.Client()),
		slackClient.OptionAPIURL(rec.URL()+"/"))
	err := s.SendMessage(context.Background(), "test message")
	assert.Error(err)
	assert.Len(rec.Requests(), 1)
}

func TestSendMessageWebhookMode(t *testing.T) {
	assert := assert.New(t)

	s := newTestSlack(map[string]interface{}{
		"webhook": "https://example.test/hook",
	}, "dev")
	assert.NotNil(s)

	s.send = mockedSend
	assert.Nil(s.SendMessage(context.Background(), "test message"))
}

// --- helper tests ---

func TestMarkdownSection(t *testing.T) {
	block := markdownSection("test")
	assert.Equal(t, slackClient.MBTSection, block.Type)
}
