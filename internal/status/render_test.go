package status

import (
	"encoding/json"
	"strings"
	"testing"
)

func sampleReport() Report {
	return Report{
		At: testNow, Cluster: "prod-eu-1",
		Problems: Problems{Open: 2, More: 1, Items: []Problem{{
			ID: "i1", Tier: "page", State: "open",
			Root: "zone zone-b", AgeSeconds: 240,
			Cause: "Zone zone-b is failing as a whole."}}},
		ControlPlane: []Component{{Name: "etcd", State: "ok"},
			{Name: "scheduler", State: "problem", Detail: "down"}},
		Upgrade: Readiness{Total: 1, Items: []Blocker{{Class: ClassBudget,
			Text: "PDB shop/api currently allows 0 disruptions"}}},
		Zones: Zones{Assessed: true, Items: []Zone{{Name: "zone-b",
			Nodes: 3, NotReady: 3, FailingPods: 14, Concentrated: true},
			{Name: "zone-a", Nodes: 2}}},
		Gaps: []Gap{{What: "secrets objects", Reason: "forbidden"}},
	}
}

func TestTextLayout(t *testing.T) {
	want := `kwatch status (prod-eu-1), 2026-10-07 10:00:00 UTC

Problems: 2 open
  [page] zone zone-b, 4m: Zone zone-b is failing as a whole.
  and 1 more not shown

Control plane
  ok: etcd
  problem: scheduler, down

Upgrade readiness: 1 blocker
  - PDB shop/api currently allows 0 disruptions

Zones
  Zone zone-b: 3 of 3 nodes not ready; 14 failing pods are all in this ` +
		`zone; the other zones are healthy.

Coverage gaps
  - secrets objects: forbidden
`
	if got := sampleReport().Text(); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestEmptyReportSaysSoPlainly(t *testing.T) {
	text := Report{At: testNow}.Text()
	for _, s := range []string{"Problems: none open",
		"no blockers found",
		"Zones: not assessed", "Coverage gaps: none"} {
		if !strings.Contains(text, s) {
			t.Errorf("missing %q in:\n%s", s, text)
		}
	}
}

func TestJSONShape(t *testing.T) {
	data, err := json.Marshal(sampleReport())
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"at", "problems", "controlPlane",
		"upgradeReadiness", "zones", "coverageGaps"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing key %q", key)
		}
	}
	if _, ok := got["configurationRisks"]; ok {
		t.Error("configuration risks are not reported")
	}
}

func TestLongLinesAreCut(t *testing.T) {
	r := sampleReport()
	r.Problems.Items[0].Cause = strings.Repeat("x", 1000)
	for _, l := range strings.Split(r.Text(), "\n") {
		if len([]rune(l)) > maxLine {
			t.Fatalf("line of %d characters", len([]rune(l)))
		}
	}
}
