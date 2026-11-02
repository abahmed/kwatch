package signal

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A message far over Signal's size limit is cut to the limit, so the
// service accepts it instead of rejecting it for good.
func TestSignalCutsOverlongIncident(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewSignal(map[string]interface{}{
		"number": "+12025550199", "to": "+12025550100", "url": rec.URL(),
	}, "", rec.Dependencies())
	if err := c.SendIncident(
		context.Background(), providertest.Oversized(300000),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	body := rec.Last(t)
	providertest.AssertWithin(t, "text", body.JSON(t)["message"].(string),
		signalTextLimit)
}
