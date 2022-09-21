package mattermost

import (
	"testing"

	"github.com/abahmed/kwatch/event"
	"github.com/stretchr/testify/assert"
)

func mockedSend(
	content []byte) error {
	return nil
}

func TestMattermostEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewMattermost(map[string]string{})
	assert.Nil(c)
}

func TestMattermostInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	config := map[string]string{
		"webhook": "testtest",
	}
	c := NewMattermost(config)
	assert.Nil(c)
}

func TestMattermost(t *testing.T) {
	assert := assert.New(t)

	config := map[string]string{
		"webhook": "testtest",
	}
	c := NewMattermost(config)
	assert.NotNil(c)

	assert.Equal(c.Name(), "Mattermost")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	config := map[string]string{
		"webhook": "testtest",
	}
	c := NewMattermost(config)
	assert.NotNil(c)

	//c.sendAPI = mockedSend
	assert.Nil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	config := map[string]string{
		"webhook": "testtest",
	}
	c := NewMattermost(config)
	assert.NotNil(c)

	c.send = mockedSend

	ev := event.Event{
		Name:      "test-pod",
		Container: "test-container",
		Namespace: "default",
		Reason:    "OOMKILLED",
		Logs:      "test\ntestlogs",
		Events: "event1-event2-event3-event1-event2-event3-event1-event2-" +
			"event3\nevent5\nevent6-event8-event11-event12",
	}
	assert.Nil(c.SendEvent(&ev))
}
