package rocketchat

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestRocketChatSendIncidentPostsNote(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewRocketChat(map[string]interface{}{"webhook": rec.URL()},
		"dev", rec.Dependencies())
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := c.SendIncident(
				context.Background(), tc.Message,
			); err != nil {
				t.Fatalf("SendIncident: %v", err)
			}
			text, _ := rec.Last(t).JSON(t)["text"].(string)
			if !strings.HasPrefix(text, tc.Message.Note) {
				t.Fatalf("text = %q, want the Note first", text)
			}
			providertest.AssertOneLeadingEmoji(t, text)
			hasOutput := strings.Contains(text, "panic: out of memory")
			if hasOutput != (len(tc.Message.Output) > 0) {
				t.Fatalf("output block mismatch: %q", text)
			}
		})
	}
}

func TestRocketChatSendIncidentNeutralizesMentions(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewRocketChat(map[string]interface{}{"webhook": rec.URL()},
		"dev", rec.Dependencies())
	if err := c.SendIncident(
		context.Background(), providertest.Hostile(),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	text, _ := rec.Last(t).JSON(t)["text"].(string)
	if strings.Contains(text, "@channel") {
		t.Fatalf("mention was not neutralized: %q", text)
	}
}
