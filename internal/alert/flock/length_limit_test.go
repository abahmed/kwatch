package flock

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A message far over Flock's size limit is cut to the limit, so the
// service accepts it instead of rejecting it for good.
func TestFlockCutsOverlongIncident(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewFlock(map[string]interface{}{
		"webhook": rec.URL() + "/hooks/hk"}, "", rec.Dependencies())
	if err := c.SendIncident(
		context.Background(), providertest.Oversized(300000),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	body := rec.Last(t)
	providertest.AssertWithin(t, "text", body.JSON(t)["text"].(string),
		flockTextLimit)
}
