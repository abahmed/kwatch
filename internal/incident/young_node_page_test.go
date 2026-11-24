package incident

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// sandboxIncident is an incident whose critical members are the given
// findings; serving marks its user-facing workload with nothing ready
// (the model of these tests has no Service to say so).
func sandboxIncident(findings ...detection.Finding) *Incident {
	p := &Incident{Members: map[detection.Key]detection.Finding{}}
	for _, s := range findings {
		p.Members[s.Key()] = s
	}
	return p
}

func sandboxFinding(pod, reason string) detection.Finding {
	return detection.Finding{Reason: reason, Severity: detection.Critical,
		Entity: inventory.CoreID(kube.KindPod, "shop", pod),
		Since:  youngNow}
}

// A pod that cannot get a network on a node that joined two minutes
// ago never pages, though its workload has nothing ready.
func TestYoungNodeNetworkNeverPages(t *testing.T) {
	model := youngModel(t, "", "api-1-b")
	p := sandboxIncident(sandboxFinding("api-1-b",
		"FailedCreatePodSandBox"))
	p.noteServing(model, youngNow)
	p.servingDown = true
	if got, why := tierWhy(p); got != Notify || why != levelYoungNetwork {
		t.Fatalf("tier = %v (%s), want notify (%s)", got, why,
			levelYoungNetwork)
	}
}

// The same failure on an old node, or next to another critical
// failure, pages as before.
func TestNetworkFailurePagesOffYoungNodes(t *testing.T) {
	model := youngModel(t, "", "api-1-a", "api-1-b")
	for name, p := range map[string]*Incident{
		"old node": sandboxIncident(sandboxFinding("api-1-a",
			"FailedCreatePodSandBox")),
		"another failure": sandboxIncident(
			sandboxFinding("api-1-b", "NetworkNotReady"),
			sandboxFinding("api-1-b", "CrashLoop")),
	} {
		p.noteServing(model, youngNow)
		p.servingDown = true
		if got, _ := tierWhy(p); got != Page {
			t.Errorf("%s: tier = %v, want page", name, got)
		}
	}
}
