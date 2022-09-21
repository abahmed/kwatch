package discord

import (
	"testing"

	"github.com/abahmed/kwatch/event"
	discordgo "github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
)

func mockedSend(
	webhookID,
	token string,
	wait bool,
	data *discordgo.WebhookParams) (st *discordgo.Message, err error) {
	return nil, nil
}

func TestDiscordEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewDiscord(map[string]string{})
	assert.Nil(c)
}

func TestDiscordInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	config := map[string]string{
		"webhook": "testtest",
	}
	c := NewDiscord(config)
	assert.Nil(c)
}

func TestDiscord(t *testing.T) {
	assert := assert.New(t)

	config := map[string]string{
		"webhook": "test/test",
	}
	c := NewDiscord(config)
	assert.NotNil(c)

	assert.Equal(c.Name(), "Discord")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	config := map[string]string{
		"webhook": "test/test",
	}
	c := NewDiscord(config)
	assert.NotNil(c)

	c.send = mockedSend
	assert.Nil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	config := map[string]string{
		"webhook": "test/test",
	}
	c := NewDiscord(config)
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
