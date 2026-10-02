package discord

import (
	"context"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

// capturingDiscord records the webhook parameters instead of calling the
// Discord API.
func capturingDiscord(captured *[]*discordgo.WebhookParams) *Discord {
	return &Discord{send: func(
		_, _ string, _ bool, data *discordgo.WebhookParams,
		_ ...discordgo.RequestOption,
	) (*discordgo.Message, error) {
		*captured = append(*captured, data)
		return nil, nil
	}}
}

func TestDiscordSendIncidentPostsNote(t *testing.T) {
	var captured []*discordgo.WebhookParams
	d := capturingDiscord(&captured)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := d.SendIncident(
				context.Background(), tc.Message,
			); err != nil {
				t.Fatalf("SendIncident: %v", err)
			}
			params := captured[len(captured)-1]
			if !strings.HasPrefix(params.Content, tc.Message.Note) {
				t.Fatalf("content = %q", params.Content)
			}
			providertest.AssertOneLeadingEmoji(t, params.Content)
			if len(params.Embeds) != 0 || params.AllowedMentions == nil {
				t.Fatalf("unexpected params: %+v", params)
			}
		})
	}
}

func TestDiscordSendIncidentNeutralizesMentions(t *testing.T) {
	var captured []*discordgo.WebhookParams
	d := capturingDiscord(&captured)
	if err := d.SendIncident(
		context.Background(), providertest.Hostile(),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	if strings.Contains(captured[0].Content, "@channel") {
		t.Fatalf("mention was not neutralized: %q", captured[0].Content)
	}
}

func TestDiscordSendIncidentTruncatesContent(t *testing.T) {
	var captured []*discordgo.WebhookParams
	d := capturingDiscord(&captured)
	m := providertest.Announce()
	m.Note = notification.MarkerPage + " " + strings.Repeat("é", 3000)
	if err := d.SendIncident(context.Background(), m); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	content := captured[0].Content
	if len(content) > maxContent || !strings.HasSuffix(content, "…") {
		t.Fatalf("content is %d bytes", len(content))
	}
	if again := incidentContent(m); again != content {
		t.Fatal("truncation is not deterministic")
	}
	providertest.AssertOneLeadingEmoji(t, content)
}
