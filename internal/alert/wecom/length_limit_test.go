package wecom

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A message far over Wecom's size limit is cut to the limit, so the
// service accepts it instead of rejecting it for good.
func TestWecomCutsOverlongIncident(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewWecom(map[string]interface{}{
		"webhook": rec.URL() + "/cgi-bin/webhook/send?key=hk",
	}, "", rec.Dependencies())
	if err := c.SendIncident(
		context.Background(), providertest.Oversized(300000),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	body := rec.Last(t)
	content := body.JSON(t)["markdown"].(map[string]any)["content"]
	providertest.AssertWithin(t, "content", content.(string),
		wecomTextLimit)
}
