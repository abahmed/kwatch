package slack

import (
	"testing"

	"github.com/abahmed/kwatch/event"
	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
)

func mockedSend(url string, msg *slackClient.WebhookMessage) error {
	return nil
}
func TestSlackEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]string{})
	assert.Nil(s)
}

func TestSlack(t *testing.T) {
	assert := assert.New(t)

	config := map[string]string{
		"webhook": "testtest",
	}
	s := NewSlack(config)
	assert.NotNil(s)

	assert.Equal(s.Name(), "Slack")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]string{
		"webhook": "testtest",
	})
	assert.NotNil(s)

	s.send = mockedSend
	assert.Nil(s.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := NewSlack(map[string]string{
		"webhook": "testtest",
	})
	assert.NotNil(s)

	s.send = mockedSend

	ev := event.Event{
		Name:      "test-pod",
		Container: "test-container",
		Namespace: "default",
		Reason:    "OOMKILLED",
		Logs:      "test\ntestlogs",
		Events: "event1-event2-event3-event1-event2-event3-event1-event2-" +
			"event3\nevent5\nevent6-event8-event11-event12",
	}
	assert.Nil(s.SendEvent(&ev))
}
