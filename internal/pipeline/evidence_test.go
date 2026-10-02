package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The investigation starts when the incident opens, its result arrives
// while the incident settles, and the announcement carries it at once.
func TestEngineInvestigatesAtOpenAndAnnouncesWithEvidence(t *testing.T) {
	started := make(chan string, 8)
	investigator := funcInvestigator(func(
		_ context.Context, p incident.Incident,
	) Result {
		started <- p.ID
		return Result{Output: []string{"panic: boom"},
			Evidence: []incident.Fact{
				{Kind: incident.FactError, Text: "panic: boom"}}}
	})
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	h := newHarnessWith(t, start, func(d *Dependencies) {
		d.Investigator = investigator
	})
	startPool(t, h.engine)
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("reports")
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{}, crashingPod("reports-a", rs.Name, "n1", start))

	h.run(start, time.Second)

	id := waitForID(t, started)
	if len(h.decisions) != 0 {
		t.Fatalf("decisions = %+v, want the investigation before any "+
			"announcement", h.decisions)
	}
	h.engine.announcer.attachOutput(context.Background(), receive(t, h.engine))
	h.run(start.Add(10*time.Minute), 10*time.Second)

	if len(h.decisions) != 1 || h.decisions[0].Incident.ID != id {
		t.Fatalf("decisions = %+v, want the announcement of %s",
			h.decisions, id)
	}
	got := h.decisions[0]
	if len(got.Evidence) != 1 || len(got.Output) != 1 {
		t.Fatalf("announcement evidence = %+v output = %q", got.Evidence,
			got.Output)
	}
	if len(started) != 0 || h.engine.Stats().InvestigationsLate != 0 {
		t.Fatal("the announcement must not start or wait for another " +
			"investigation")
	}
}

// A result that arrives after its announcement went never sends a
// message on its own. The next update carries it once.
func TestEngineLateEvidenceJoinsOnlyTheNextUpdate(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	g := newGatedInvestigator()
	sink := &sinkLog{}
	e := investigatingEngine(t, clock, g, sink)
	startPool(t, e)
	ctx := context.Background()
	if !e.announcer.investigate(incident.Incident{ID: "a"}, clock.now) {
		t.Fatal("the investigation must start")
	}
	e.announcer.deliver(ctx, clock.now, []incident.Decision{announce("a")})
	e.announcer.expireHeld(ctx, clock.now.Add(outputWait))

	close(g.release)
	e.announcer.attachOutput(ctx, receive(t, e))

	if got := sink.all(); len(got) != 1 || got[0].Evidence != nil {
		t.Fatalf("delivered %+v, want only the announcement, without "+
			"evidence", got)
	}
	update := incident.Decision{Action: incident.Update,
		Incident: incident.Incident{ID: "a"}}
	e.announcer.deliver(ctx, clock.now, []incident.Decision{update, update})
	got := sink.all()
	if len(got) != 3 || len(got[1].Evidence) != 1 || got[2].Evidence != nil {
		t.Fatalf("updates = %+v, want the late fact in the first only",
			got[1:])
	}
	if len(got[1].Output) != 1 || got[2].Output != nil {
		t.Fatal("the output joins the update with the new fact, once")
	}
}

func TestEngineReinvestigatesWhenRootKindChanged(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	kinds := make(chan string, 4)
	e := newTestEngine(t, clock, (&sinkLog{}).sink, func(d *Dependencies) {
		d.Investigator = kindInvestigator{started: kinds}
	})
	startPool(t, e)
	ctx := context.Background()
	opened := incident.Incident{ID: "a",
		Root: inventory.CoreID(kube.KindPod, "shop", "api-1")}
	e.announcer.investigate(opened, clock.now)
	e.announcer.attachOutput(ctx, receive(t, e))

	revised := announce("a")
	revised.Incident.Root = inventory.CoreID(kube.KindNode, "", "n1")
	e.announcer.deliver(ctx, clock.now, []incident.Decision{revised})

	if first, second := <-kinds, <-kinds; first != "pod" || second != "node" {
		t.Fatalf("investigated %s then %s, want pod then node", first, second)
	}
	if len(e.announcer.held) != 1 {
		t.Fatal("the announcement must wait for the new investigation")
	}
}

func TestEngineResolveForgetsEvidence(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	g := newGatedInvestigator()
	close(g.release)
	e := investigatingEngine(t, clock, g, &sinkLog{})
	startPool(t, e)
	ctx := context.Background()
	e.announcer.investigate(incident.Incident{ID: "a"}, clock.now)
	e.announcer.attachOutput(ctx, receive(t, e))

	e.announcer.deliver(ctx, clock.now, []incident.Decision{{
		Action: incident.Resolve, Incident: incident.Incident{ID: "a"}}})

	if len(e.announcer.evidence) != 0 {
		t.Fatalf("evidence = %d entries after resolve", len(e.announcer.evidence))
	}
}

func TestEngineForgetsOldEvidence(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, (&sinkLog{}).sink, nil)
	e.announcer.evidence["old"] = &evidenceState{started: now.Add(-time.Hour)}
	e.announcer.evidence["running"] = &evidenceState{started: now.Add(-time.Hour),
		running: true}
	e.announcer.evidence["new"] = &evidenceState{started: now}

	e.announcer.forgetOldEvidence(now)

	if _, ok := e.announcer.evidence["old"]; ok || len(e.announcer.evidence) != 2 {
		t.Fatalf("kept %d, want the running and the recent one",
			len(e.announcer.evidence))
	}
}

// An investigation ends at its own budget; what it found by then is
// kept and the timeout is counted.
func TestEngineInvestigationBudgetBoundsTheRun(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	budget := 20 * time.Millisecond
	e := newTestEngine(t, clock, (&sinkLog{}).sink, func(d *Dependencies) {
		d.Investigator = budgetInvestigator{budget: budget}
	})
	startPool(t, e)

	e.announcer.investigate(incident.Incident{ID: "a"}, clock.now)
	r := receive(t, e)

	if r.result.Evidence[0].Text != "partial" {
		t.Fatalf("result = %+v, want what was found before the deadline",
			r.result)
	}
	if n := e.Stats().InvestigationTimeouts; n != 1 {
		t.Fatalf("timeouts = %d, want 1", n)
	}
	if budgetOf(Investigation{}) != investigationTimeout ||
		budgetOf(Investigation{Budget: time.Hour}) != investigationTimeout ||
		budgetOf(Investigation{Budget: budget}) != budget {
		t.Fatal("a budget must be its own, capped at the pool deadline")
	}
}

type budgetInvestigator struct{ budget time.Duration }

func (b budgetInvestigator) Plan(incident.Incident) (Investigation, bool) {
	return Investigation{Kind: "slow", Budget: b.budget,
		Run: func(ctx context.Context) Result {
			<-ctx.Done()
			return Result{Evidence: []incident.Fact{
				{Kind: incident.FactError, Text: "partial"}}}
		}}, true
}

// kindInvestigator names its plan after the root kind.
type kindInvestigator struct{ started chan string }

func (k kindInvestigator) Plan(p incident.Incident) (Investigation, bool) {
	kind := string(p.Root.Kind)
	return Investigation{Kind: kind, Run: func(context.Context) Result {
		k.started <- kind
		return Result{}
	}}, true
}

func waitForID(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case id := <-ch:
		return id
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for an investigation")
	}
	return ""
}
