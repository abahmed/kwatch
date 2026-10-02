package homeassistant

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSendIncidentSendsShortAsMessage(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewHomeAssistant(map[string]interface{}{"token": "test"}, "dev",
		rec.Dependencies())
	c.url = rec.URL()

	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := c.SendIncident(
				context.Background(), tc.Message,
			); err != nil {
				t.Fatalf("SendIncident() error = %v", err)
			}
			req := rec.Last(t)
			body := req.JSON(t)
			message, _ := body["message"].(string)
			if message != tc.Message.Short || body["title"] != "kwatch alert" {
				t.Fatalf("payload = %v", body)
			}
			providertest.AssertOneLeadingEmoji(t, message)
			if req.Header.Get("Authorization") != "Bearer test" {
				t.Fatal("missing bearer token")
			}
		})
	}
}
