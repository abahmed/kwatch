package slack

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
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

func testIncident() *model.Incident {
	return &model.Incident{
		Key:       "default:deploy-1:CrashLoopBackOff",
		Name:      "deploy-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
		Resource:  "pod",
		Count:     1,
		FirstSeen: time.Now().Add(-5 * time.Minute),
		LastSeen:  time.Now(),
		Resources: map[string]bool{"pod-1": true, "pod-2": true},
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
	assert.Contains(lastMsg, "Incident")
	assert.Contains(lastMsg, "deploy-1")
	assert.Contains(lastMsg, "CrashLoopBackOff")
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
	assert.Contains(lastMsg, "Update")
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
	assert.Contains(lastText, "Incident")
	assert.Contains(lastText, "deploy-1")
	assert.Contains(lastText, "CrashLoopBackOff")
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

func TestSendIncidentTokenCreate(t *testing.T) {
	assert := assert.New(t)

	s := &Slack{
		channel: "#alerts",
		appCfg:  &config.App{ClusterName: "dev"},
	}

	var capturedBlocks *slackClient.Blocks
	var capturedThreadTS string
	s.postBlocksFn = func(blocks *slackClient.Blocks, threadTS string) (string, error) {
		capturedBlocks = blocks
		capturedThreadTS = threadTS
		return "12345.67890", nil
	}

	err := s.SendIncident(testIncident(), model.ActionCreate)
	assert.Nil(err)
	assert.NotNil(capturedBlocks)
	assert.Empty(capturedThreadTS)

	// verify threadMap was populated
	s.mu.Lock()
	ts, ok := s.threadMap["default:deploy-1:CrashLoopBackOff"]
	s.mu.Unlock()
	assert.True(ok)
	assert.Equal("12345.67890", ts)
}

func TestSendIncidentTokenUpdate(t *testing.T) {
	assert := assert.New(t)

	s := &Slack{
		channel:   "#alerts",
		appCfg:    &config.App{ClusterName: "dev"},
		threadMap: map[string]string{"default:deploy-1:CrashLoopBackOff": "12345.67890"},
	}

	var capturedBlocks *slackClient.Blocks
	var capturedThreadTS string
	s.postBlocksFn = func(blocks *slackClient.Blocks, threadTS string) (string, error) {
		capturedBlocks = blocks
		capturedThreadTS = threadTS
		return "12345.67891", nil
	}

	err := s.SendIncident(testIncident(), model.ActionUpdate)
	assert.Nil(err)
	assert.NotNil(capturedBlocks)
	assert.Equal("12345.67890", capturedThreadTS)
}

func TestSendIncidentTokenUpdateNoThread(t *testing.T) {
	assert := assert.New(t)

	s := &Slack{
		channel: "#alerts",
		appCfg:  &config.App{ClusterName: "dev"},
		// no threadMap set — first update should still work (no thread)
	}

	var capturedThreadTS string
	s.postBlocksFn = func(_ *slackClient.Blocks, threadTS string) (string, error) {
		capturedThreadTS = threadTS
		return "12345.67890", nil
	}

	err := s.SendIncident(testIncident(), model.ActionUpdate)
	assert.Nil(err)
	assert.Empty(capturedThreadTS)
}

func TestSendIncidentTokenSkip(t *testing.T) {
	assert := assert.New(t)

	s := &Slack{
		channel: "#alerts",
		appCfg:  &config.App{ClusterName: "dev"},
	}

	called := false
	s.postBlocksFn = func(_ *slackClient.Blocks, _ string) (string, error) {
		called = true
		return "", nil
	}

	err := s.SendIncident(testIncident(), model.ActionSkip)
	assert.Nil(err)
	assert.False(called)
}

// --- buildIncidentBlocks ---

func TestBuildIncidentBlocks(t *testing.T) {
	assert := assert.New(t)

	inc := testIncident()
	blocks := buildIncidentBlocks(inc, &config.App{ClusterName: "prod-cluster"})

	assert.NotNil(blocks)
	assert.Greater(len(blocks.BlockSet), 0)
}

func TestBuildIncidentUpdateBlocks(t *testing.T) {
	assert := assert.New(t)

	inc := testIncident()
	blocks := buildIncidentUpdateBlocks(inc)

	assert.NotNil(blocks)
	assert.Equal(1, len(blocks.BlockSet))
}

func TestFormatIncidentText(t *testing.T) {
	assert := assert.New(t)

	inc := testIncident()
	text := formatIncidentText(inc, model.ActionCreate)
	assert.Contains(text, "Incident")
	assert.Contains(text, "deploy-1")

	textUpdate := formatIncidentText(inc, model.ActionUpdate)
	assert.Contains(textUpdate, "Update")
}

func TestBuildIncidentBlocksWithLogsEvents(t *testing.T) {
	assert := assert.New(t)

	inc := testIncident()
	inc.Events = "Warning Unhealthy pod-1 liveness probe failed"
	inc.Logs = "Error: connection refused"
	inc.IncludeEvents = true
	inc.IncludeLogs = true

	blocks := buildIncidentBlocks(inc, &config.App{ClusterName: "prod-cluster"})

	assert.NotNil(blocks)
	foundEvents := false
	foundLogs := false
	for _, b := range blocks.BlockSet {
		if s, ok := b.(slackClient.SectionBlock); ok && s.Text != nil {
			if s.Text.Text == ":mag: *Events*" {
				foundEvents = true
			}
			if s.Text.Text == ":memo: *Logs*" {
				foundLogs = true
			}
		}
	}
	assert.True(foundEvents, "Events block should be present")
	assert.True(foundLogs, "Logs block should be present")
}

func TestBuildIncidentUpdateBlocksWithLogsEvents(t *testing.T) {
	assert := assert.New(t)

	inc := testIncident()
	inc.Events = "Warning BackOff restarting container"
	inc.Logs = "Error: server closed connection"
	inc.IncludeEvents = true
	inc.IncludeLogs = true

	blocks := buildIncidentUpdateBlocks(inc)

	assert.NotNil(blocks)
	assert.Greater(len(blocks.BlockSet), 1, "update blocks should include Logs/Events sections")
}

func TestFormatIncidentTextWithLogsEvents(t *testing.T) {
	assert := assert.New(t)

	inc := testIncident()
	inc.Events = "Warning Unhealthy"
	inc.Logs = "Error: timeout"
	inc.IncludeEvents = true
	inc.IncludeLogs = true

	text := formatIncidentText(inc, model.ActionCreate)
	assert.Contains(text, "Events:")
	assert.Contains(text, "Warning Unhealthy")
	assert.Contains(text, "Logs:")
	assert.Contains(text, "Error: timeout")
}

func TestFormatIncidentTextUpdateWithLogsEvents(t *testing.T) {
	assert := assert.New(t)

	inc := testIncident()
	inc.Events = "Warning BackOff"
	inc.Logs = "Error: crash"
	inc.IncludeEvents = true
	inc.IncludeLogs = true

	text := formatIncidentText(inc, model.ActionUpdate)
	assert.Contains(text, "Events:")
	assert.Contains(text, "Warning BackOff")
	assert.Contains(text, "Logs:")
	assert.Contains(text, "Error: crash")
}
