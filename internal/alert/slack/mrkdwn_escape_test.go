package slack

import (
	"context"
	"testing"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
)

func TestSlackEscapesControlSequencesInText(t *testing.T) {
	var sent *slackClient.WebhookMessage
	s := &Slack{webhook: "https://hooks.example/x",
		send: func(_ string, msg *slackClient.WebhookMessage) error {
			sent = msg
			return nil
		}}
	assert.NoError(t, s.SendMessage(
		context.Background(), "log <!channel> & <@U1>",
	))
	assert.Equal(t, "log &lt;!channel&gt; &amp; &lt;@U1&gt;", sent.Text)
}

func TestSlackMarkdownBlocksEscapeEventText(t *testing.T) {
	block := markdownSection("pod says <!here>")
	assert.Equal(t, "pod says &lt;!here&gt;", block.Text.Text)
}
