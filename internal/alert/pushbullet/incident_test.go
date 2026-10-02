package pushbullet

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSendIncidentSendsShortAsNote(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewPushbullet(map[string]interface{}{"accessToken": "o.test"},
		"dev", rec.Dependencies())
	c.url = rec.URL()

	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := c.SendIncident(
				context.Background(), tc.Message,
			); err != nil {
				t.Fatalf("SendIncident() error = %v", err)
			}
			body := rec.Last(t).JSON(t)
			text, _ := body["body"].(string)
			title, _ := body["title"].(string)
			if text != tc.Message.Short || title != "kwatch alert: dev" {
				t.Fatalf("payload = %v", body)
			}
			providertest.AssertOneLeadingEmoji(t, text)
			if providertest.CountEmoji(title) != 0 {
				t.Fatalf("title has an emoji: %q", title)
			}
		})
	}
}
