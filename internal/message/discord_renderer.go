package message

// DiscordRenderer produces Discord markdown from a Report.
type DiscordRenderer struct{ textRenderer }

// NewDiscordRenderer returns a DiscordRenderer.
func NewDiscordRenderer() *DiscordRenderer {
	return &DiscordRenderer{textRenderer{m: discordMarkup}}
}
