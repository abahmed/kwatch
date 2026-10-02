package messagebird

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSendIncidentSendsShortAsSMS(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewMessagebird(map[string]interface{}{
		"accessKey": "test", "from": "kwatch", "to": "+12025550100",
	}, "dev", rec.Dependencies())
	c.url = rec.URL()

	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := c.SendIncident(
				context.Background(), tc.Message,
			); err != nil {
				t.Fatalf("SendIncident() error = %v", err)
			}
			body, _ := rec.Last(t).JSON(t)["body"].(string)
			if body != tc.Message.Short {
				t.Fatalf("body = %q, want %q", body, tc.Message.Short)
			}
			providertest.AssertOneLeadingEmoji(t, body)
		})
	}
}
