package message

// SlackRenderer produces Slack mrkdwn text from a Report, for webhook-mode
// Slack. Token-mode Slack renders the same Report as Block Kit blocks.
type SlackRenderer struct{ textRenderer }

// NewSlackRenderer returns a SlackRenderer.
func NewSlackRenderer() *SlackRenderer {
	return &SlackRenderer{textRenderer{m: slackMarkup}}
}
