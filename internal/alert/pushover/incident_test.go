package pushover

import (
	"context"
	"net/url"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSendIncidentSendsShortWithStatusPriority(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewPushover(map[string]interface{}{
		"token": "test", "user": "user123", "title": "kwatch",
		"priority": 2, "retry": 60, "expire": 3600,
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
			if form.Get("message") != tc.Message.Short {
				t.Fatalf("message = %q", form.Get("message"))
			}
			providertest.AssertOneLeadingEmoji(t, form.Get("message"))
			assertPushoverPriority(t, form, tc.Message.Resolved())
		})
	}
}

func assertPushoverPriority(t *testing.T, form url.Values, resolved bool) {
	t.Helper()
	if resolved {
		if form.Has("priority") || form.Has("retry") {
			t.Fatalf("resolve must use normal priority: %v", form)
		}
		return
	}
	if form.Get("priority") != "2" || form.Get("retry") != "60" ||
		form.Get("expire") != "3600" {
		t.Fatalf("emergency priority needs retry and expire: %v", form)
	}
}

func TestSendIncidentHighPriorityNeedsNoRetry(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewPushover(map[string]interface{}{
		"token": "test", "user": "user123", "priority": 1,
	}, "dev", rec.Dependencies())
	c.url = rec.URL()

	err := c.SendIncident(context.Background(), providertest.Announce())
	if err != nil {
		t.Fatalf("SendIncident() error = %v", err)
	}
	form, _ := url.ParseQuery(string(rec.Last(t).Body))
	if form.Get("priority") != "1" || form.Has("retry") ||
		form.Has("expire") {
		t.Fatalf("priority 1 form = %v", form)
	}
}
