package discord

import (
	"context"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
)

func TestDiscordMessagesDisableMentions(t *testing.T) {
	var captured *discordgo.WebhookParams
	d := &Discord{send: func(
		_, _ string, _ bool, data *discordgo.WebhookParams,
		_ ...discordgo.RequestOption,
	) (*discordgo.Message, error) {
		captured = data
		return nil, nil
	}}
	assert.NoError(t, d.SendMessage(context.Background(), "@everyone boom"))
	if assert.NotNil(t, captured.AllowedMentions) {
		assert.Empty(t, captured.AllowedMentions.Parse)
		assert.NotNil(t, captured.AllowedMentions.Parse)
	}
}
