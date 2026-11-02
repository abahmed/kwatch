package zulip

import (
	"context"
	"net/url"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A message far over Zulip's size limit is cut to the limit, so the
// service accepts it instead of rejecting it for good.
func TestZulipCutsOverlongIncident(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewZulip(map[string]interface{}{
		"email": "kwatch@example.com", "token": "zulip-secret",
		"channel": "alerts", "url": rec.URL(),
	}, "", rec.Dependencies())
	if err := c.SendIncident(
		context.Background(), providertest.Oversized(300000),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	body := rec.Last(t)
	providertest.AssertWithin(t, "text", formValue(t, body, "content"),
		zulipTextLimit)
}

func formValue(
	t *testing.T, r providertest.Request, name string,
) string {
	t.Helper()
	form, err := url.ParseQuery(string(r.Body))
	if err != nil {
		t.Fatalf("form body: %v", err)
	}
	return form.Get(name)
}
