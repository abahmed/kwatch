package problem

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func announced(t *testing.T, r *rig, s signal.Signal) {
	t.Helper()
	r.raise(at(0), s)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
}

func TestManagerDigestIgnoresCountersAndSummary(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)

	changed := web
	changed.Summary = "restarted 9 times"
	r.apply(at(2*time.Minute), signal.Changed, changed)

	wantNone(t, r.tick(at(3*time.Minute)))
}

func TestManagerDigestUpdatesOnNewRootReason(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)

	r.raise(at(2*time.Minute), sig(web.Entity,
		constant.ReasonOOMKilled, signal.Warning))
	ds := r.tick(at(3 * time.Minute))
	wantAction(t, ds, Update, "material change")
	if ds[0].Problem.Revision != 2 {
		t.Fatalf("revision = %d, want 2", ds[0].Problem.Revision)
	}
	wantNone(t, r.tick(at(4*time.Minute)))
}

func TestManagerDigestUpdatesWhenCauseChanges(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	pod := podSig("web")
	r.relate(pod.Entity, knowledge.RunsOn, node)
	r.cause(pod.Entity, node, "node is out of memory")
	announced(t, r, pod)

	r.cause(pod.Entity, node, "node lost its network")
	r.apply(at(2*time.Minute), signal.Changed, pod)
	wantAction(t, r.tick(at(3*time.Minute)), Update, "material change")
}

func TestManagerDigestUpdatesWhenImpactCrossesBucket(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	first := podSig("web-1")
	r.cause(first.Entity, node, "node down")
	announced(t, r, first)

	for _, name := range []string{"api", "db"} {
		id := entity(kube.KindDeployment, name)
		pod := entity(kube.KindPod, name+"-0")
		r.relate(pod, knowledge.OwnedBy, id)
		s := sig(pod, constant.ReasonCrashLoopBackOff, signal.Warning)
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

func TestDigestUniqDropsAdjacentDuplicates(t *testing.T) {
	got := uniq([]string{"a", "a", "b", "b", "b", "c"})
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("uniq = %v", got)
	}
	if len(uniq(nil)) != 0 {
		t.Fatal("uniq(nil) should be empty")
	}
}

func TestDigestRecentKeepsTimesInsideWindow(t *testing.T) {
	times := []time.Time{at(0), at(20 * time.Minute), at(40 * time.Minute)}
	got := recent(times, at(45*time.Minute), 30*time.Minute)
	if len(got) != 2 || !got[0].Equal(at(20*time.Minute)) {
		t.Fatalf("recent = %v", got)
	}
}

func TestProblemDigestIgnoresSymptomsOfOtherEntities(t *testing.T) {
	root := entity(kube.KindNode, "n1")
	base := &Problem{
		Root: root, Tier: Notify, State: Open,
		Members: map[signal.Key]signal.Signal{},
	}
	before := digest(base)
	pod := podSig("web")
	pod.Symptom = true
	base.Members[pod.Key()] = pod
	if digest(base) != before {
		t.Fatal("symptom of another entity must not change digest")
	}
}
