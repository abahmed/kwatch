package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// keptMember runs a finding that turns into a crash loop, with the cause
// swinging back (swing) so the incident keeps it (keepsMember), and
// returns the incident's mode and stage.
func keptMember(t *testing.T, swing bool) (detection.Mode, stage) {
	t.Helper()
	r := newRig(t, Config{})
	web := podSig("web")
	first := entity(kube.KindConfigMap, "cfg-a")
	second := entity(kube.KindConfigMap, "cfg-b")
	r.cause(web.Entity, first, "cfg-a changed")
	r.raise(at(0), web)
	r.tick(at(DefaultSettle))
	r.cause(web.Entity, second, "cfg-b changed")
	r.apply(at(2*time.Minute), detection.Changed, web)
	r.tick(at(2*time.Minute + time.Second))
	if swing {
		r.cause(web.Entity, first, "cfg-a changed")
	}
	worse := web
	worse.Severity = detection.Critical
	worse.Mode = detection.ModeCrashLoop
	r.apply(at(2*time.Minute+20*time.Second), detection.Changed, worse)
	r.tick(at(10 * time.Minute))
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	p := r.m.incidents[r.m.byMember[worse.Key()]]
	return p.Mode, p.stagePeak
}

// A member the incident keeps (its cause swung back) still refreshes the
// incident: its crash loop counts like any other member's.
func TestKeptMemberRefreshesTheIncident(t *testing.T) {
	wantMode, wantStage := keptMember(t, false)
	mode, st := keptMember(t, true)
	if mode != wantMode || st != wantStage {
		t.Fatalf("kept member: mode %q stage %v, want %q %v",
			mode, st, wantMode, wantStage)
	}
}

// A member leaving refreshes the incident: the tier follows the members
// that are left, not the one that cleared.
func TestDetachedMemberRefreshesTheTier(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	node.Namespace = ""
	crit := sig(node, reasons.NodeNotReady, detection.Critical)
	warn := sig(entity(kube.KindPod, "web"), reasons.CrashLoopBackOff,
		detection.Warning)
	r.cause(warn.Entity, node, "node n1")
	r.raise(at(0), crit, warn)
	r.clear(at(10*time.Second), crit)
	after := r.of(node)
	r.m.mu.Lock()
	fresh := *r.m.incidents[after.ID]
	r.m.mu.Unlock()
	if want, _ := tierWhy(&fresh); want != after.Tier {
		t.Fatalf("tier %v after the node cleared, members call for %v",
			after.Tier, want)
	}
}

// A flapper that then stays down without a break is no longer flapping:
// it returns to open and tells what it owes (here, a revised cause),
// instead of staying silent until it resolves.
func TestSteadyFlapperReturnsToOpen(t *testing.T) {
	r := newRig(t, Config{})
	runCycles(r, 2*time.Minute, 4*time.Minute, 19*time.Minute)
	if r.only().State != Flapping {
		t.Fatalf("not flapping: %v", r.only().State)
	}
	web := podSig("web")
	r.cause(web.Entity, entity(kube.KindNode, "n1"),
		"node n1 is out of memory")
	worse := web
	worse.Mode = detection.ModeCrashLoop
	r.apply(at(19*time.Minute+10*time.Second), detection.Changed, worse)
	var ds []Decision
	for now := 20 * time.Minute; now < 2*time.Hour; now += 30 *
		time.Second {
		ds = append(ds, r.tick(at(now))...)
	}
	p := r.only()
	if p.State != Open || len(ds) == 0 || p.Pending.RevisedOwed() {
		t.Fatalf("state %v, %d decisions, revised owed %v",
			p.State, len(ds), p.Pending.RevisedOwed())
	}
}
