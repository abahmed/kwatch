package pushbullet

import (
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact note push request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 go test -run TestContract
// -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewPushbullet(map[string]interface{}{
		"accessToken": "o.pb-secret",
	}, "dev", rec.Dependencies())
	c.url = rec.URL() + "/v2/pushes"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers: []string{"Access-Token"},
		Secrets: []string{"o.pb-secret"}, LimitBytes: 3000,
		Check: checkShape,
	})
}

func checkShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" ||
		reqs[0].Path != "/v2/pushes" {
		t.Fatalf("%s: requests = %+v", name, reqs)
	}
	if reqs[0].Header.Get("Access-Token") != "o.pb-secret" {
		t.Fatalf("%s: access token header missing", name)
	}
	body := reqs[0].JSON(t)
	if body["type"] != "note" || body["title"] == "" ||
		body["body"] == "" {
		t.Fatalf("%s: note lacks type, title or body: %v", name, body)
	}
}
