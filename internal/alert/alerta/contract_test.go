package alerta

import (
	"encoding/json"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Alerta API request for the canonical
// messages. Alerta deduplicates on environment, resource and event, so the
// resource is the key that must not change across the lifecycle.
func TestContract(t *testing.T) {
	c, rec := newTestAlerta(t)
	c.apiKey = "alerta-api-secret"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers: []string{"Authorization"},
		Secrets: []string{"alerta-api-secret"},
		KeyOf: func(t *testing.T, r providertest.Request) string {
			p := r.JSON(t)
			return p["environment"].(string) + "|" +
				p["resource"].(string) + "|" + p["event"].(string)
		},
		Check: checkAlertShape,
	})
}

func checkAlertShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if name == "digest" {
		if len(reqs) != 0 {
			t.Fatal("a digest must not raise an alert")
		}
		return
	}
	if len(reqs) != 1 || reqs[0].Method != "POST" ||
		reqs[0].Path != alertaAPIPath {
		t.Fatalf("got %d requests", len(reqs))
	}
	if reqs[0].Header.Get("Authorization") != "Key alerta-api-secret" {
		t.Fatal("missing Key authorization")
	}
	var p alertaPayload
	if err := json.Unmarshal(reqs[0].Body, &p); err != nil {
		t.Fatal(err)
	}
	if p.Resource == "" || p.Event != incidentEvent ||
		p.Environment != "Staging" || len(p.Service) != 1 {
		t.Fatalf("alert lacks required fields: %+v", p)
	}
	wantSeverity, wantStatus := "critical", ""
	if name == "resolve" {
		wantSeverity, wantStatus = "normal", "closed"
	}
	if p.Severity != wantSeverity || p.Status != wantStatus {
		t.Fatalf("severity/status = %q/%q, want %q/%q", p.Severity,
			p.Status, wantSeverity, wantStatus)
	}
	providertest.AssertFencesBalanced(t, p.Text)
}
