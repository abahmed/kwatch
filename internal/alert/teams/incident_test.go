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
