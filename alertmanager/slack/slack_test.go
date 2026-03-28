package slack

import (
	"testing"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
)

func mockedSend(url string, msg *slackClient.WebhookMessage) error {
	return nil
}

// --- webhook mode tests ---

func TestSlackEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(s)
}

func TestSlackWebhook(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "testtest",
	}
	s := NewSlack(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(s)
	assert.Equal("Slack", s.Name())
}

func TestSlackWebhookWithChannel(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "testtest",
		"channel": "#alerts",
	}
	s := NewSlack(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(s)
	assert.Equal("#alerts", s.channel)
}

func TestSendMessageWebhook(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]interface{}{
		"webhook": "testtest",
		"channel": "test",
	}, &config.App{ClusterName: "dev"})
	assert.NotNil(s)

	s.send = mockedSend
	assert.Nil(s.SendMessage("test"))
}

func TestSendEventWebhook(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]interface{}{
		"webhook": "testtest",
	}, &config.App{ClusterName: "dev"})
	assert.NotNil(s)

	s.send = mockedSend

	ev := &event.Event{
		NodeName:      "test-node",
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "some log line 1\nsome log line 2\nsome log line 3",
		Events:        "BackOff Back-off restarting failed container\nevent3\nevent5",
	}
	assert.Nil(s.SendEvent(ev))
}

func TestSendEventWebhookWithLargeLogs(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]interface{}{
		"webhook": "testtest",
	}, &config.App{ClusterName: "dev"})
	assert.NotNil(s)

	s.send = mockedSend

	// generate logs larger than chunkSize (2000)
	longLog := ""
	for i := 0; i < 500; i++ {
		longLog += "Nam quis nulla. Integer malesuada. In in enim a arcu imperdiet.\n"
	}

	ev := &event.Event{
		NodeName:      "test-node",
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          longLog,
	}
	assert.Nil(s.SendEvent(ev))
}

// --- token mode tests ---

func TestSlackTokenMode(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token":   "xoxb-test-token",
		"channel": "#alerts",
	}
	s := NewSlack(configMap, &config.App{ClusterName: "dev"})
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
	s := NewSlack(configMap, &config.App{ClusterName: "dev"})
	assert.Nil(s)
}

func TestSlackTokenEmptyChannel(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token":   "xoxb-test-token",
		"channel": "",
	}
	s := NewSlack(configMap, &config.App{ClusterName: "dev"})
	assert.Nil(s)
}

func TestSlackWebhookPreferWebhookOverToken(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "https://hooks.slack.com/test",
		"token":   "",
		"channel": "#alerts",
	}
	s := NewSlack(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(s)
	// Empty token should fall through to webhook mode
	assert.Equal("https://hooks.slack.com/test", s.webhook)
	assert.Nil(s.apiClient)
}

func TestSendMessageTokenMode(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]interface{}{
		"token":   "xoxb-test-token",
		"channel": "#alerts",
	}, &config.App{ClusterName: "dev"})
	assert.NotNil(s)

	// sendAPIWithToken will fail because the token is fake,
	// but we're testing the dispatch path
	err := s.SendMessage("test message")
	assert.Error(err) // fake token, API call fails
}

func TestSendMessageWebhookMode(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]interface{}{
		"webhook": "testtest",
	}, &config.App{ClusterName: "dev"})
	assert.NotNil(s)

	s.send = mockedSend
	assert.Nil(s.SendMessage("test message"))
}

// --- helper tests ---

func TestChunks(t *testing.T) {
	assert := assert.New(t)

	result := chunks("abc", 5)
	assert.Equal([]string{"abc"}, result)

	result = chunks("abcdef", 3)
	assert.Equal([]string{"abc", "def"}, result)

	result = chunks("abcdefg", 3)
	assert.Equal([]string{"abc", "def", "g"}, result)
}

func TestMarkdownSection(t *testing.T) {
	block := markdownSection("test")
	assert.Equal(t, slackClient.MBTSection, block.Type)
}

func TestPlainSection(t *testing.T) {
	block := plainSection("test")
	assert.Equal(t, slackClient.MBTSection, block.Type)
}

func TestMarkdownF(t *testing.T) {
	obj := markdownF("*%s*", "test")
	assert.NotNil(t, obj)
}
