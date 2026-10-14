package homeassistant

import (
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact notify service request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 go test -run TestContract
// -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewHomeAssistant(map[string]interface{}{
		"token": "ha-secret", "service": "mobile_app_kwatch",
	}, "dev", rec.Dependencies())
	c.url = rec.URL() + "/api/services/notify/mobile_app_kwatch"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers: []string{"Authorization"},
		Secrets: []string{"ha-secret"}, LimitBytes: 3000,
		Check: checkShape,
	})
}

func checkShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("%s: requests = %+v", name, reqs)
	}
	r := reqs[0]
	if r.Header.Get("Authorization") != "Bearer ha-secret" {
		t.Fatalf("%s: bearer token missing", name)
	}
	if r.Path != "/api/services/notify/mobile_app_kwatch" {
		t.Fatalf("%s: path = %s", name, r.Path)
	}
	body := r.JSON(t)
	if s, _ := body["message"].(string); s == "" {
		t.Fatalf("%s: message is empty", name)
	}
}
