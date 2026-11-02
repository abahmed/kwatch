package alerta

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// The text fields of an over-long incident are cut to alertaTextLimit.
func TestAlertaCutsOverlongIncident(t *testing.T) {
	c, rec := newTestAlerta(t)
	if err := c.SendIncident(
		context.Background(), providertest.Oversized(300000),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	body := rec.Last(t).JSON(t)
	for _, field := range []string{"text", "value"} {
		providertest.AssertWithin(t, field, body[field].(string),
			alertaTextLimit)
	}
}
