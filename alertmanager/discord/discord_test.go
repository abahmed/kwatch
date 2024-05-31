package discord

import (
	"testing"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
	discordgo "github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
)

func mockedSend(
	webhookID,
	token string,
	wait bool,
	data *discordgo.WebhookParams,
	options ...discordgo.RequestOption) (st *discordgo.Message, err error) {
	return nil, nil
}

func TestDiscordEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewDiscord(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestDiscordInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "testtest",
	}
	c := NewDiscord(configMap, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestDiscord(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "test/test",
	}
	c := NewDiscord(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	assert.Equal(c.Name(), "Discord")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "test/test",
	}
	c := NewDiscord(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	c.send = mockedSend
	assert.Nil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "test/test",
	}
	c := NewDiscord(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	c.send = mockedSend

	ev := event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs: "Nam quis nulla. Integer malesuada. In in enim a arcu " +
			"imperdiet malesuada. Sed vel lectus. Donec odio urna, tempus " +
			"molestie, porttitor ut, iaculis quis, sem. Phasellus rhoncus.\n" +
			"Nam quis nulla. Integer malesuada. In in enim a arcu " +
			"imperdiet malesuada. Sed vel lectus. Donec odio urna, tempus " +
			"molestie, porttitor ut, iaculis quis, sem. Phasellus rhoncus.\n" +
			"Nam quis nulla. Integer malesuada. In in enim a arcu " +
			"imperdiet malesuada. Sed vel lectus. Donec odio urna, tempus " +
			"molestie, porttitor ut, iaculis quis, sem. Phasellus rhoncus.\n" +
			"Nam quis nulla. Integer malesuada. In in enim a arcu " +
			"imperdiet malesuada. Sed vel lectus. Donec odio urna, tempus " +
			"molestie, porttitor ut, iaculis quis, sem. Phasellus rhoncus.\n" +
			"Nam quis nulla. Integer malesuada. In in enim a arcu " +
			"imperdiet malesuada. Sed vel lectus. Donec odio urna, tempus " +
			"molestie, porttitor ut, iaculis quis, sem. Phasellus rhoncus.\n" +
			"Nam quis nulla. Integer malesuada. In in enim a arcu " +
			"imperdiet malesuada. Sed vel lectus. Donec odio urna, tempus " +
			"molestie, porttitor ut, iaculis quis, sem. Phasellus rhoncus.\n",
		Events: "BackOff Back-off restarting failed container\n" +
			"event3\nevent5\nevent6-event8-event11-event12",
	}
	assert.Nil(c.SendEvent(&ev))
}
