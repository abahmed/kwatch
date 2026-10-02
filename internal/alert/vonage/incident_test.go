package vonage

import (
	"context"
	"net/url"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSendIncidentSendsShortAsSMS(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewVonage(map[string]interface{}{
		"apiKey": "test", "apiSecret": "secret",
		"from": "kwatch", "to": "+12025550100",
	}, "dev", rec.Dependencies())
	c.url = rec.URL()

	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := c.SendIncident(
				context.Background(), tc.Message,
			); err != nil {
				t.Fatalf("SendIncident() error = %v", err)
			}
			form, err := url.ParseQuery(string(rec.Last(t).Body))
			if err != nil {
				t.Fatalf("body is not a form: %v", err)
			}
			text := form.Get("text")
			if text != tc.Message.Short {
				t.Fatalf("text = %q, want %q", text, tc.Message.Short)
			}
			providertest.AssertOneLeadingEmoji(t, text)
		})
	}
}
