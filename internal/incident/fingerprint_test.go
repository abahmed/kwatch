package incident

import (
	"strconv"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func announced(t *testing.T, r *rig, s detection.Finding) {
	t.Helper()
	r.raise(at(0), s)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
}

func TestManagerFingerprintIgnoresCountersAndSummary(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)

	changed := web
	changed.Summary = "restarted 9 times"
	r.apply(at(2*time.Minute), detection.Changed, changed)

	wantNone(t, r.tick(at(3*time.Minute)))
}

func TestManagerFingerprintUpdatesOnNewRootReason(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)

	r.raise(at(2*time.Minute), sig(web.Entity,
		reasons.OOMKilled, detection.Warning))
	ds := r.tick(at(3 * time.Minute))
	wantAction(t, ds, Update, "material change")
	if ds[0].Incident.Revision != 2 {
		t.Fatalf("revision = %d, want 2", ds[0].Incident.Revision)
	}
	wantNone(t, r.tick(at(4*time.Minute)))
}

func TestManagerFingerprintUpdatesWhenCauseChanges(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	pod := podSig("web")
	r.relate(pod.Entity, inventory.RunsOn, node)
	r.cause(pod.Entity, node, "node is out of memory")
	announced(t, r, pod)

	r.cause(pod.Entity, node, "node lost its network")
	network := sig(node, "NodeNetworkUnavailable", detection.Critical)
	network.Health = detection.Failing
	r.sigs[node] = []detection.Finding{network}
	r.apply(at(2*time.Minute), detection.Changed, pod)
	wantAction(t, r.tick(at(3*time.Minute)), Update, "material change")
}

func TestManagerFingerprintIgnoresLiveNumbersInCauseSummary(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	pod := podSig("web")
	r.relate(pod.Entity, inventory.RunsOn, node)
	r.cause(pod.Entity, node, "node memory at 91%")
	announced(t, r, pod)

	summaries := []string{
		"node memory at 93%",
		"node memory at 97%",
	}
	for i, summary := range summaries {
		r.cause(pod.Entity, node, summary)
		changed := pod
		changed.Summary = "restarted " + strconv.Itoa(5+i) + " times"
		now := at(time.Duration(2+i) * time.Minute)
		r.apply(now, detection.Changed, changed)
		wantNone(t, r.tick(now.Add(time.Second)))
	}
	if got := r.only().Cause.Summary; got != summaries[1] {
		t.Fatalf("cause summary = %q, want the latest", got)
	}
}

func TestManagerFingerprintUpdatesWhenImpactCrossesBucket(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	first := podSig("web-1")
	r.cause(first.Entity, node, "node down")
	announced(t, r, first)

	for _, name := range []string{"api", "db"} {
		id := entity(kube.KindDeployment, name)
		pod := entity(kube.KindPod, name+"-0")
		r.relate(pod, inventory.OwnedBy, id)
		s := sig(pod, reasons.CrashLoopBackOff, detection.Warning)
		r.cause(pod, node, "node down")
		r.raise(at(2*time.Minute), s)
	}
	wantAction(t, r.tick(at(3*time.Minute)), Update, "material change")
}

func TestImpactBucketBoundaries(t *testing.T) {
	cases := map[int]string{
		0: "1", 1: "1", 2: "2-3", 3: "2-3", 4: "4-7", 7: "4-7",
		8: "8-15", 15: "8-15", 16: "16+", 100: "16+",
	}
	for n, want := range cases {
		if got := impactBucket(n); got != want {
			t.Errorf("impactBucket(%d) = %s, want %s", n, got, want)
		}
	}
}

func TestFingerprintUniqDropsAdjacentDuplicates(t *testing.T) {
	got := uniq([]string{"a", "a", "b", "b", "b", "c"})
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("uniq = %v", got)
	}
	if len(uniq(nil)) != 0 {
		t.Fatal("uniq(nil) should be empty")
	}
}

func TestFingerprintRecentKeepsTimesInsideWindow(t *testing.T) {
	times := []time.Time{at(0), at(20 * time.Minute), at(40 * time.Minute)}
	got := recent(times, at(45*time.Minute), 30*time.Minute)
	if len(got) != 2 || !got[0].Equal(at(20*time.Minute)) {
		t.Fatalf("recent = %v", got)
	}
}

func TestIncidentFingerprintIgnoresSymptomsOfOtherEntities(t *testing.T) {
	root := entity(kube.KindNode, "n1")
	base := &Incident{
		Root: root, Tier: Notify, State: Open,
		Members: map[detection.Key]detection.Finding{},
	}
	before := fingerprint(base)
	pod := podSig("web")
	pod.Symptom = true
	base.Members[pod.Key()] = pod
	if fingerprint(base) != before {
		t.Fatal("symptom of another entity must not change the fingerprint")
	}
}

// A cause that blames a change names its root findings. One that ages
// out, such as an event leaving its window, is not news: the
// fingerprint keeps what the cause once had.
func TestFingerprintIgnoresRootFindingsThatAgeOut(t *testing.T) {
	cr := entity(kube.KindDeployment, "operator")
	events := sig(cr, reasons.CrashLoopBackOff, detection.Warning)
	ready := sig(cr, reasons.DeploymentUnavailable, detection.Warning)
	p := incidentOf(entity(kube.KindDeployment, "web"), nil)
	p.Cause = &rootcause.CauseRecord{
		Rule: "own-change", Root: cr,
		Change:       &inventory.Change{Entity: cr, Revision: "7"},
		RootFindings: []detection.Finding{events, ready},
	}
	p.rememberRootReasons()
	before := fingerprint(p)

	p.Cause.RootFindings = []detection.Finding{ready}
	p.rememberRootReasons()

	if fingerprint(p) != before {
		t.Fatal("a finding that aged out changed the fingerprint")
	}
}

// A workload behind a Service with no endpoints, already reported,
// gains "pods run but never become ready": a later stage of the same
// story, not news. After plain unavailability it is a milestone.
func TestFingerprintIgnoresNeverReadyAfterNoEndpoints(t *testing.T) {
	root := entity(kube.KindDeployment, "api")
	p := incidentOf(root, nil)
	first := sig(root, reasons.ServiceNoEndpoints, detection.Warning)
	first.Symptom = true
	p.Members[first.Key()] = first
	before := fingerprint(p)
	never := sig(root, reasons.WorkloadNeverReady, detection.Warning)
	p.Members[never.Key()] = never
	p.rememberRootReasons()
	if fingerprint(p) != before {
		t.Fatal("never ready after no endpoints changed the fingerprint")
	}

	// Unavailable alone keeps its "never healthy" milestone.
	waiting := incidentOf(root, nil)
	down := sig(root, reasons.DeploymentUnavailable, detection.Warning)
	down.Symptom = true
	waiting.Members[down.Key()] = down
	before = fingerprint(waiting)
	waiting.Members[never.Key()] = never
	if fingerprint(waiting) == before {
		t.Fatal("never ready after unavailable is a milestone")
	}

	alone := incidentOf(root, nil)
	before = fingerprint(alone)
	alone.Members[never.Key()] = never
	if fingerprint(alone) == before {
		t.Fatal("never ready on its own must change the fingerprint")
	}
}
