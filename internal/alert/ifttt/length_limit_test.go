package ifttt

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// The ingredients of an over-long incident are cut to iftttValueLimit.
func TestIftttCutsOverlongIncident(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewIfttt(map[string]interface{}{"key": "abc123"}, "",
		rec.Dependencies())
	c.url = rec.URL() + "/trigger/kw/with/key/abc123"
	if err := c.SendIncident(
		context.Background(), providertest.Oversized(300000),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	body := rec.Last(t).JSON(t)
	for _, field := range []string{"value1", "value2"} {
		providertest.AssertWithin(t, field, body[field].(string),
			iftttValueLimit)
	}
}
