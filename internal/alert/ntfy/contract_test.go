package ntfy

import (
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact JSON publish request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 go test -run TestContract
// -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewNtfy(map[string]interface{}{
		"url": rec.URL(), "topic": "kwatch", "priority": 5,
		"token": "tk-secret", "title": "kwatch",
	}, "dev", rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers: []string{"Authorization"},
		Secrets: []string{"tk-secret"}, LimitBytes: 3000,
		Check: checkShape,
	})
}

func checkShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" ||
		reqs[0].Path != "/" {
		t.Fatalf("%s: requests = %+v", name, reqs)
	}
	body := reqs[0].JSON(t)
	if body["topic"] != "kwatch" {
		t.Fatalf("%s: topic = %v", name, body["topic"])
	}
	if s, _ := body["message"].(string); s == "" {
		t.Fatalf("%s: message is empty", name)
	}
	want := float64(5)
	if name == "resolve" {
		want = ntfyDefaultPriority
	}
	if body["priority"] != want {
		t.Fatalf("%s: priority = %v, want %v", name,
			body["priority"], want)
	}
}
