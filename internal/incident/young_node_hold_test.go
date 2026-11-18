package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var youngNow = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

// youngModel: replica set rs owns pod a on node old (a day old) and pod
// b on node new (joined two minutes ago). failing lists the pods that
// are not ready; line is each container's last error line.
func youngModel(
	t *testing.T, line string, failing ...string,
) *inventory.Model {
	t.Helper()
	model := inventory.NewModel(inventory.Options{})
	apply := func(o inventory.Observation) {
		o.Source, o.At = "test", youngNow
		if _, err := model.Apply(o); err != nil {
			t.Fatal(err)
		}
	}
	rs := inventory.CoreID(kube.KindReplicaSet, "shop", "api-1")
	apply(inventory.Observation{Kind: inventory.Observed, Entity: rs,
		Attributes: map[string]inventory.Value{}})
	created := map[string]time.Time{"old": youngNow.Add(-24 * time.Hour),
		"new": youngNow.Add(-2 * time.Minute)}
	for pod, node := range map[string]string{"api-1-a": "old",
		"api-1-b": "new"} {
		n := inventory.CoreID(kube.KindNode, "", node)
		apply(inventory.Observation{Kind: inventory.Observed, Entity: n,
			Attributes: map[string]inventory.Value{
				kube.AttrCreated: inventory.Time(created[node])}})
		p := inventory.CoreID(kube.KindPod, "shop", pod)
		ready := true
		for _, f := range failing {
			ready = ready && f != pod
		}
		apply(inventory.Observation{Kind: inventory.Observed, Entity: p,
			Attributes: map[string]inventory.Value{
				kube.AttrReady: inventory.Bool(ready)}})
		c := kube.ContainerID("shop", pod, "app")
		apply(inventory.Observation{Kind: inventory.Observed, Entity: c,
			Attributes: map[string]inventory.Value{
				kube.AttrLastErrorLine: inventory.Text(line)}})
		apply(inventory.Observation{Kind: inventory.Related, Entity: c,
			Relation: inventory.PartOf, Targets: []inventory.EntityID{p}})
		apply(inventory.Observation{Kind: inventory.Related, Entity: p,
			Relation: inventory.RunsOn, Targets: []inventory.EntityID{n}})
		apply(inventory.Observation{Kind: inventory.Related, Entity: p,
			Relation: inventory.OwnedBy, Targets: []inventory.EntityID{rs}})
	}
	return model
}

const refusedLine = "dial tcp 10.0.7.20:6379: connect: connection refused"

func youngFinding(pod string) detection.Finding {
	return detection.Finding{Reason: "CrashLoop",
		Entity: kube.ContainerID("shop", pod, "app"),
		Since:  youngNow.Add(-time.Minute)}
}

func TestYoungHoldOnlyOnTheNewNode(t *testing.T) {
	model := youngModel(t, refusedLine, "api-1-b")
	until := youngHoldUntil(model, youngNow, youngFinding("api-1-b").Entity)
	if want := youngNow.Add(kube.YoungNodeWindow - 2*time.Minute); !until.Equal(
		want) {
		t.Fatalf("until = %v, want %v", until, want)
	}
	if !failsToConnect(model, youngFinding("api-1-b")) {
		t.Fatal("the connection failure was not read")
	}
}

// A replica of the same workload failing on the old node: not the new
// node's failure, nothing is held.
func TestYoungHoldNotWhenAnOldReplicaFails(t *testing.T) {
	model := youngModel(t, refusedLine, "api-1-a", "api-1-b")
	if until := youngHoldUntil(model, youngNow,
		youngFinding("api-1-b").Entity); !until.IsZero() {
		t.Fatalf("held until %v while the old replica fails", until)
	}
	if until := youngHoldUntil(model, youngNow,
		youngFinding("api-1-a").Entity); !until.IsZero() {
		t.Fatalf("a failure on the old node was held until %v", until)
	}
}

// A failure that names no connection (a missing file) is the app's,
// wherever it runs.
func TestYoungHoldNeedsAConnectionFailure(t *testing.T) {
	model := youngModel(t, "FileNotFoundException: Could not load file",
		"api-1-b")
	if failsToConnect(model, youngFinding("api-1-b")) {
		t.Fatal("a missing file read as a connection failure")
	}
	probe := youngFinding("api-1-b")
	probe.Evidence = []detection.Evidence{{Label: "probe",
		Value: "dial tcp 10.1.0.5:8080: connect: connection refused"}}
	if failsToConnect(youngModel(t, "boom", "api-1-b"), probe) {
		t.Fatal("the kubelet's probe message counted as the pod's call")
	}
}

func TestWithNodeAgeAddsTheFactOnce(t *testing.T) {
	model := youngModel(t, refusedLine, "api-1-b")
	s := withNodeAge(model, youngFinding("api-1-b"))
	if got := evidenceOf(s, detection.EvidenceYoungNode); got != "new 1" {
		t.Fatalf("node age = %q", got)
	}
	if again := withNodeAge(model, s); len(again.Evidence) != 1 {
		t.Fatalf("added twice: %+v", again.Evidence)
	}
	old := withNodeAge(model, youngFinding("api-1-a"))
	if got := evidenceOf(old, detection.EvidenceYoungNode); got != "" {
		t.Fatalf("an old node got an age: %q", got)
	}
}
