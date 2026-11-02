package sns

import (
	"context"
	"net/url"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A message far over the SNS Publish limit is cut to it, so SNS accepts
// it instead of rejecting it for good.
func TestSnsCutsOverlongIncident(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewSns(map[string]interface{}{
		"accessKeyId": testKeyID, "secretAccessKey": testSecret,
		"topicArn": topic,
	}, "dev", rec.Dependencies())
	c.url = rec.URL() + "/"
	if err := c.SendIncident(
		context.Background(), providertest.Oversized(300000),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	form, err := url.ParseQuery(string(rec.Last(t).Body))
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	providertest.AssertWithin(t, "Message", form.Get("Message"),
		maxMessageBytes)
}
