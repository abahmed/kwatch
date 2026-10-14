package datadog

import (
	"encoding/json"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Datadog events API request for the
// canonical messages. A resolve is an event with the same aggregation key
// and alert_type success.
func TestContract(t *testing.T) {
	c, rec := newTestDatadog(t, nil)
	c.apiKey, c.appKey = "dd-api-secret", "dd-app-secret"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers:    []string{"DD-API-KEY", "DD-APPLICATION-KEY"},
		Secrets:    []string{"dd-api-secret", "dd-app-secret"},
		LimitBytes: maxTextBytes,
		KeyOf: func(t *testing.T, r providertest.Request) string {
			return r.JSON(t)["aggregation_key"].(string)
		},
		Check: checkEventShape,
	})
}

func checkEventShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if name == "digest" {
		if len(reqs) != 0 {
			t.Fatal("a digest must not post an event")
		}
		return
	}
	if len(reqs) != 1 || reqs[0].Method != "POST" ||
		reqs[0].Path != "/api/v1/events" {
		t.Fatalf("got %d requests", len(reqs))
	}
	if reqs[0].Header.Get("DD-API-KEY") != "dd-api-secret" {
		t.Fatal("missing DD-API-KEY header")
	}
	var p datadogPayload
	if err := json.Unmarshal(reqs[0].Body, &p); err != nil {
		t.Fatal(err)
	}
	want := "error"
	if name == "resolve" {
		want = "success"
	}
	if p.AlertType != want || p.Title == "" || p.AggregationKey == "" {
		t.Fatalf("event = %+v, want alert_type %s", p, want)
	}
	if len(p.Title) > maxTitleBytes || len(p.Text) > maxTextBytes {
		t.Fatalf("limits exceeded: %d/%d", len(p.Title), len(p.Text))
	}
	providertest.AssertFencesBalanced(t, p.Text)
	if len(p.Tags) == 0 || p.Tags[len(p.Tags)-1] != "cluster:dev" {
		t.Fatalf("tags = %v, want the cluster tag", p.Tags)
	}
}
