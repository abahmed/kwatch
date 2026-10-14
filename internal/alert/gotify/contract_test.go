package gotify

import (
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact /message request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 go test -run TestContract
// -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewGotify(map[string]interface{}{
		"url": rec.URL(), "token": "gt-secret", "title": "kwatch",
		"priority": 8,
	}, "dev", rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers: []string{"X-Gotify-Key"},
		Secrets: []string{"gt-secret"}, LimitBytes: 3000,
		Check: checkShape,
	})
}

func checkShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" ||
		reqs[0].Path != "/message" {
		t.Fatalf("%s: requests = %+v", name, reqs)
	}
	if reqs[0].Header.Get("X-Gotify-Key") != "gt-secret" {
		t.Fatalf("%s: app token header missing", name)
	}
	body := reqs[0].JSON(t)
	if s, _ := body["message"].(string); s == "" {
		t.Fatalf("%s: message is empty", name)
	}
	// Resolves use the server default (priority omitted).
	_, has := body["priority"]
	if (name == "resolve") == has {
		t.Fatalf("%s: priority = %v", name, body["priority"])
	}
}
