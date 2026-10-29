package dashboard

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
)

func TestDetailCarriesEvidenceRedacted(t *testing.T) {
	p := testIncident("a", "shop", 0)
	p.Evidence = []incident.Section{{
		Check: incident.CheckLogs, Title: "Logs",
		Status: incident.SectionOK, Subject: "app-a-1/app",
		Run:     "previous",
		Lines:   []string{"connecting", "password=hunter2 refused"},
		Matched: []int{1},
	}, {
		Check: incident.CheckUsage, Title: "Memory use",
		Status: incident.SectionDenied, Reason: incident.ReasonRBAC,
	}}
	p.Correlation = &incident.Correlation{Confirmed: "db has 0 endpoints",
		Facts: []incident.SectionFact{{
			Label: incident.CorrelationSignature,
			Value: "connection refused: password=hunter2"}}}
	var d IncidentDetail
	body := get(t, newTestServer(p), "/api/incidents/a").Body.Bytes()
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Evidence) != 2 || d.Evidence[1].Status != "denied" ||
		d.Evidence[1].Reason != "rbac" || d.Evidence[0].Matched[0] != 1 {
		t.Fatalf("evidence = %+v", d.Evidence)
	}
	if strings.Contains(string(body), "hunter2") {
		t.Fatal("a secret in evidence was served unredacted")
	}
	if d.Correlation == nil || d.Correlation.Confirmed == "" {
		t.Fatalf("correlation = %+v", d.Correlation)
	}
}

func TestDetailWithoutEvidenceOmitsIt(t *testing.T) {
	body := get(t, newTestServer(testIncident("a", "shop", 0)),
		"/api/incidents/a").Body.String()
	if strings.Contains(body, `"evidence"`) ||
		strings.Contains(body, `"correlation"`) {
		t.Fatalf("a Community detail carries evidence: %s", body)
	}
}
