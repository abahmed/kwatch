package slack

import (
	"testing"
	"time"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
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
		Events: "BackOff Back-off restarting failed " +
			"container\nevent3\nevent5",
	}
	assert.Nil(s.SendEvent(ev))
}

func TestSendEventWebhookCompact(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]interface{}{
		"webhook": "testtest",
		"compact": true,
	}, &config.App{ClusterName: "dev"})
	assert.NotNil(s)
	assert.True(s.compact)

	var lastText string
	s.send = func(_ string, msg *slackClient.WebhookMessage) error {
		lastText = msg.Text
		return nil
	}

	ev := &event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
	}
	assert.Nil(s.SendEvent(ev))
	assert.Equal("K8s Alert: test-pod - OOMKILLED (default)", lastText)
}

func TestSendEventWebhookCompactFalse(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]interface{}{
		"webhook": "testtest",
		"compact": false,
	}, &config.App{ClusterName: "dev"})
	assert.NotNil(s)
	assert.False(s.compact)
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
		longLog += "Nam quis nulla. Integer malesuada. In in enim a arcu " +
			"imperdiet.\n"
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

	result := util.Chunks("abc", 5)
	assert.Equal([]string{"abc"}, result)

	result = util.Chunks("abcdef", 3)
	assert.Equal([]string{"abc", "def"}, result)

	result = util.Chunks("abcdefg", 3)
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

func testIncident() *model.Incident {
	return &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy-1:CrashLoopBackOff",
			Name:      "deploy-1",
			Namespace: "default",
			Reason:    "CrashLoopBackOff",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:     1,
			FirstSeen: time.Now().Add(-5 * time.Minute),
			LastSeen:  time.Now(),
			Resources: map[string]bool{"pod-1": true, "pod-2": true},
		},
	}

}

// --- SendIncident: webhook fallback ---

func TestSendIncidentWebhookCreate(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]interface{}{
		"webhook": "testtest",
	}, &config.App{ClusterName: "dev"})
	assert.NotNil(s)

	var lastMsg string
	s.send = func(_ string, msg *slackClient.WebhookMessage) error {
		lastMsg = msg.Text
		return nil
	}

	err := s.SendIncident(testIncident(), model.ActionCreate)
	assert.Nil(err)
	assert.Contains(lastMsg, "CrashLoopBackOff")
	assert.Contains(lastMsg, "deploy-1")
}

func TestSendIncidentWebhookUpdate(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]interface{}{
		"webhook": "testtest",
	}, &config.App{ClusterName: "dev"})
	assert.NotNil(s)

	var lastMsg string
	s.send = func(_ string, msg *slackClient.WebhookMessage) error {
		lastMsg = msg.Text
		return nil
	}

	err := s.SendIncident(testIncident(), model.ActionUpdate)
	assert.Nil(err)
	assert.Contains(lastMsg, "CrashLoopBackOff")
}

func TestSendIncidentWebhookCompact(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]interface{}{
		"webhook": "testtest",
		"compact": true,
	}, &config.App{ClusterName: "dev"})
	assert.NotNil(s)
	assert.True(s.compact)

	var lastText string
	s.send = func(_ string, msg *slackClient.WebhookMessage) error {
		lastText = msg.Text
		return nil
	}

	err := s.SendIncident(testIncident(), model.ActionCreate)
	assert.Nil(err)
	assert.Contains(lastText, "CrashLoopBackOff")
	assert.Contains(lastText, "deploy-1")
}

func TestSendIncidentWebhookSkip(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]interface{}{
		"webhook": "testtest",
	}, &config.App{ClusterName: "dev"})
	assert.NotNil(s)

	called := false
	s.send = func(_ string, _ *slackClient.WebhookMessage) error {
		called = true
		return nil
	}

	err := s.SendIncident(testIncident(), model.ActionSkip)
	assert.Nil(err)
	assert.False(called)
}

// --- SendIncident: token mode with mocked postBlocksFn ---
