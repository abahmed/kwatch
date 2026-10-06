package ntfy

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSendIncidentSendsShortWithPlainTags(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewNtfy(map[string]interface{}{
		"topic": "kwatch", "priority": 5,
	}, "dev", rec.Dependencies())
	c.url = rec.URL()

	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := c.SendIncident(
				context.Background(), tc.Message,
			); err != nil {
				t.Fatalf("SendIncident() error = %v", err)
			}
			last := rec.Last(t)
			if last.Path != "/" {
				t.Fatalf("published to %q, want the server root", last.Path)
			}
			body := last.JSON(t)
			if body["topic"] != "kwatch" {
				t.Fatalf("topic = %v, want kwatch", body["topic"])
			}
			message, _ := body["message"].(string)
			if message != tc.Message.Short {
				t.Fatalf("message = %q", message)
			}
			providertest.AssertOneLeadingEmoji(t, message)
			tags, _ := body["tags"].([]any)
			want := "status-" + tc.Message.Status.String()
			if len(tags) != 2 || tags[0] != "kwatch" || tags[1] != want {
				t.Fatalf("tags = %v, want [kwatch %s]", tags, want)
			}
			priority := 5.0
			if tc.Message.Resolved() {
				priority = ntfyDefaultPriority
			}
			if body["priority"] != priority {
				t.Fatalf("priority = %v, want %v", body["priority"], priority)
			}
		})
	}
}
