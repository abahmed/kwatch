package teamsworkflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func cardText(t *testing.T, body []byte) string {
	t.Helper()
	var payload teamsPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode card: %v", err)
	}
	if payload.Type != "message" || len(payload.Attachments) != 1 ||
		len(payload.Attachments[0].Content.Body) != 1 {
		t.Fatalf("unexpected card shape: %s", body)
	}
	return payload.Attachments[0].Content.Body[0].Text
}

func TestTeamsWorkflowSendIncidentPostsNote(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewTeamsWorkflow(map[string]interface{}{"webhook": rec.URL()},
		"dev", rec.Dependencies())
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := c.SendIncident(
				context.Background(), tc.Message,
			); err != nil {
				t.Fatalf("SendIncident: %v", err)
			}
			text := cardText(t, rec.Last(t).Body)
			if !strings.HasPrefix(text, tc.Message.Note) {
				t.Fatalf("text = %q, want the Note first", text)
			}
			providertest.AssertOneLeadingEmoji(t, text)
		})
	}
}

func TestTeamsWorkflowSendIncidentNeutralizesMentions(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewTeamsWorkflow(map[string]interface{}{"webhook": rec.URL()},
		"dev", rec.Dependencies())
	if err := c.SendIncident(
		context.Background(), providertest.Hostile(),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	if text := cardText(t, rec.Last(t).Body); strings.Contains(
		text, "@channel",
	) {
		t.Fatalf("mention was not neutralized: %q", text)
	}
}
