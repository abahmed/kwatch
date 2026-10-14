package n8n

import (
	"encoding/json"
	"testing"

	"github.com/abahmed/kwatch/internal/alert/structured"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact structured JSON request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 go test -run TestContract
// -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewN8n(map[string]interface{}{
		"url": rec.URL(), "title": "custom", "token": "n8n-secret",
	}, "dev", rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers: []string{"Authorization"}, Secrets: []string{"n8n-secret"},
		LimitBytes: 3000,
		KeyOf: func(t *testing.T, r providertest.Request) string {
			return r.JSON(t)["alertKey"].(string)
		},
		Check: checkShape,
	})
}

func checkShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("%s: requests = %+v", name, reqs)
	}
	if ct := reqs[0].Header.Get("Content-Type"); ct !=
		"application/json" {
		t.Fatalf("content type = %q", ct)
	}
	var in structured.Incident
	if err := json.Unmarshal(reqs[0].Body, &in); err != nil {
		t.Fatal(err)
	}
	if in.Cluster != "dev" || in.Key == "" || in.AlertKey == "" ||
		in.Status == "" || in.Short == "" {
		t.Fatalf("%s lacks required fields: %+v", name, in)
	}
	if (name == "resolve") != in.Resolved ||
		(name == "resolve") != (in.Status == "resolved") {
		t.Fatalf("%s: status %q resolved %v", name, in.Status,
			in.Resolved)
	}
}
