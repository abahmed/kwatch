package teams

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func decodeFlow(t *testing.T, body []byte) (teamsFlowPayload, string) {
	t.Helper()
	var payload teamsFlowPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode flow payload: %v", err)
	}
	if len(payload.Attachment) != 1 {
		t.Fatalf("want one card, got %s", body)
	}
	content, _ := payload.Attachment[0]["content"].(map[string]any)
	blocks, _ := content["body"].([]any)
	if len(blocks) != 2 {
		t.Fatalf("want title and text blocks, got %s", body)
	}
	text, _ := blocks[1].(map[string]any)["text"].(string)
	return payload, text
}

func TestTeamsSendIncidentPostsNote(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewTeams(map[string]interface{}{"webhook": rec.URL()}, "dev",
		rec.Dependencies())
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := c.SendIncident(
				context.Background(), tc.Message,
			); err != nil {
				t.Fatalf("SendIncident: %v", err)
			}
			payload, card := decodeFlow(t, rec.Last(t).Body)
			if payload.Title != tc.Message.Title {
				t.Fatalf("title = %q", payload.Title)
			}
			if providertest.CountEmoji(payload.Title) != 0 {
				t.Fatalf("title carries an emoji: %q", payload.Title)
			}
			if !strings.HasPrefix(payload.Text, tc.Message.Note) ||
				card != payload.Text {
				t.Fatalf("text = %q, card = %q", payload.Text, card)
			}
			providertest.AssertOneLeadingEmoji(t, payload.Text)
		})
	}
}

func TestTeamsSendIncidentUsesConfiguredTitle(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewTeams(map[string]interface{}{
		"webhook": rec.URL(), "title": "Prod alerts",
	}, "dev", rec.Dependencies())
	if err := c.SendIncident(
		context.Background(), providertest.Hostile(),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	payload, _ := decodeFlow(t, rec.Last(t).Body)
	if payload.Title != "Prod alerts" {
		t.Fatalf("title = %q", payload.Title)
	}
	if strings.Contains(payload.Text, "@channel") {
		t.Fatalf("mention was not neutralized: %q", payload.Text)
	}
}

// The title of an incident is text the cluster supplied: the card shows
// it as text, never as a link, a heading or a mention.
func TestTeamsCardTitleIsEscaped(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewTeams(map[string]interface{}{"webhook": rec.URL()}, "dev",
		rec.Dependencies())
	m := providertest.Announce()
	m.Title = "[pay](https://evil.example) **now** @channel <b>x</b>"
	if err := c.SendIncident(context.Background(), m); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	var payload teamsFlowPayload
	if err := json.Unmarshal(rec.Last(t).Body, &payload); err != nil {
		t.Fatal(err)
	}
	content := payload.Attachment[0]["content"].(map[string]any)
	title := content["body"].([]any)[0].(map[string]any)["text"].(string)
	for _, bad := range []string{"[pay](", "**now**", "@channel", "<b>"} {
		if strings.Contains(title, bad) {
			t.Errorf("title block holds markup %q: %q", bad, title)
		}
	}
	if !strings.Contains(title, `\[pay\]`) {
		t.Errorf("title block was not escaped: %q", title)
	}
}
