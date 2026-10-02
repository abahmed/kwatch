package gotify

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSendIncidentSendsShortWithStatusPriority(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewGotify(map[string]interface{}{
		"url": rec.URL(), "token": "test", "title": "kwatch",
		"priority": 8,
	}, "dev", rec.Dependencies())
	c.url = rec.URL()

	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := c.SendIncident(
				context.Background(), tc.Message,
			); err != nil {
				t.Fatalf("SendIncident() error = %v", err)
			}
			body := rec.Last(t).JSON(t)
			message, _ := body["message"].(string)
			if message != tc.Message.Short || body["title"] != "kwatch" {
				t.Fatalf("payload = %v", body)
			}
			providertest.AssertOneLeadingEmoji(t, message)
			_, hasPriority := body["priority"]
			if tc.Message.Resolved() == hasPriority {
				t.Fatalf("priority = %v, resolved = %v",
					body["priority"], tc.Message.Resolved())
			}
		})
	}
}
