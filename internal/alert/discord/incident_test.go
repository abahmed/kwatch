package discord

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

var sentAt = time.Date(2026, 10, 29, 13, 44, 0, 0, time.UTC)

// capturingDiscord records the webhook parameters instead of calling the
// Discord API.
func capturingDiscord(captured *[]*discordgo.WebhookParams) *Discord {
	return &Discord{clusterName: "prod-eu-1",
		clockSource: clock.Func(func() time.Time { return sentAt }),
		send: func(
			_, _ string, _ bool, data *discordgo.WebhookParams,
			_ ...discordgo.RequestOption,
		) (*discordgo.Message, error) {
			*captured = append(*captured, data)
			return nil, nil
		}}
}

func sendOne(t *testing.T, m notification.Message) *discordgo.MessageEmbed {
	t.Helper()
	var captured []*discordgo.WebhookParams
	if err := capturingDiscord(&captured).SendIncident(
		context.Background(), m); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	params := captured[0]
	if params.Content != "" || len(params.Embeds) != 1 ||
		params.AllowedMentions == nil {
		t.Fatalf("unexpected params: %+v", params)
	}
	return params.Embeds[0]
}

// embedSize is what Discord counts toward the 6000 limit.
func embedSize(e *discordgo.MessageEmbed) int {
	return len(e.Title) + len(e.Description) + len(e.Footer.Text)
}

func TestDiscordSendIncidentPostsOneEmbed(t *testing.T) {
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			e := sendOne(t, tc.Message)
			if e.Title != tc.Message.ShortText() {
				t.Fatalf("title = %q", e.Title)
			}
			if e.Color != statusColor(tc.Message.Status) ||
				e.Footer.Text != "kwatch · prod-eu-1" ||
				e.Timestamp != "2026-10-29T13:44:00Z" {
				t.Fatalf("embed = %+v", e)
			}
		})
	}
}

func TestDiscordStatusColors(t *testing.T) {
	want := map[notification.Status]int{
		notification.StatusCritical: colorCritical,
		notification.StatusWarning:  colorWarning,
		notification.StatusFlapping: colorWarning,
		notification.StatusResolved: colorResolved,
		notification.StatusLow:      colorInfo,
	}
	for status, color := range want {
		if got := statusColor(status); got != color {
			t.Errorf("%v: color %x, want %x", status, got, color)
		}
	}
}

func TestDiscordSendIncidentNeutralizesMentions(t *testing.T) {
	e := sendOne(t, providertest.Hostile())
	if strings.Contains(e.Title+e.Description, "@channel") ||
		strings.Contains(e.Title+e.Description, "@everyone") {
		t.Fatalf("mention was not neutralized: %q %q", e.Title,
			e.Description)
	}
}

func TestDiscordEmbedStaysWithinLimits(t *testing.T) {
	m := providertest.Announce()
	m.Note = notification.MarkerPage + " " + strings.Repeat("é", 3000)
	m.Output = []string{strings.Repeat("x", 9000)}
	e := sendOne(t, m)
	if len(e.Description) > maxEmbedDescription ||
		embedSize(e) > maxEmbedTotal || len(e.Title) > maxEmbedTitle {
		t.Fatalf("embed sizes: title %d, description %d, total %d",
			len(e.Title), len(e.Description), embedSize(e))
	}
	if again := sendOne(t, m); again.Description != e.Description {
		t.Fatal("truncation is not deterministic")
	}
}

func TestDiscordSendMessageStaysPlainAndBounded(t *testing.T) {
	var captured []*discordgo.WebhookParams
	d := capturingDiscord(&captured)
	if err := d.SendMessage(context.Background(),
		strings.Repeat("digest ", 1000)); err != nil {
		t.Fatal(err)
	}
	if len(captured[0].Embeds) != 0 ||
		len(captured[0].Content) > maxContent {
		t.Fatalf("digest params = %+v", captured[0])
	}
}
