package ifttt

import (
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Maker webhook request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 go test -run TestContract
// -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewIfttt(map[string]interface{}{
		"key": "abc123", "event": "kw",
	}, "dev", rec.Dependencies())
	c.url = rec.URL() + "/trigger/kw/with/key/abc123"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Secrets: []string{"abc123"}, LimitBytes: 3000,
		Check: checkShape,
	})
}

func checkShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("%s: requests = %+v", name, reqs)
	}
	body := reqs[0].JSON(t)
	for _, k := range []string{"value1", "value2", "value3"} {
		if s, _ := body[k].(string); s == "" {
			t.Fatalf("%s: %s is empty", name, k)
		}
	}
	if len(body) != 3 {
		t.Fatalf("%s: unexpected fields %v", name, body)
	}
	if (name == "resolve") != (body["value3"] == "resolved") {
		t.Fatalf("%s: value3 = %v", name, body["value3"])
	}
}
