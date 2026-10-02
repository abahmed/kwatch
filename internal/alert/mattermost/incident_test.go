package mattermost

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestMattermostSendIncidentPostsNote(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewMattermost(map[string]interface{}{"webhook": rec.URL()},
		"dev", rec.Dependencies())
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := c.SendIncident(
				context.Background(), tc.Message,
			); err != nil {
				t.Fatalf("SendIncident: %v", err)
			}
			body := rec.Last(t).JSON(t)
			text, _ := body["text"].(string)
			if !strings.HasPrefix(text, tc.Message.Note) {
				t.Fatalf("text = %q, want the Note first", text)
			}
			providertest.AssertOneLeadingEmoji(t, text)
			attachments, _ := body["attachments"].([]any)
			if len(attachments) != 1 ||
				!strings.Contains(string(rec.Last(t).Body), `"dev"`) {
				t.Fatalf("cluster field missing: %s", rec.Last(t).Body)
			}
		})
	}
}

func TestMattermostSendIncidentNeutralizesMentions(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewMattermost(map[string]interface{}{"webhook": rec.URL()},
		"", rec.Dependencies())
	if err := c.SendIncident(
		context.Background(), providertest.Hostile(),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	body := rec.Last(t).JSON(t)
	text, _ := body["text"].(string)
	if strings.Contains(text, "@channel") {
		t.Fatalf("mention was not neutralized: %q", text)
	}
	if _, ok := body["attachments"]; ok {
		t.Fatal("no cluster means no attachment")
	}
}
