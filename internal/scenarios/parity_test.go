package scenarios

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
)

// replayHiding runs a log through a fresh engine that cannot observe
// the kinds in hidden.
func replayHiding(
	t *testing.T, log replay.Log, opts replay.Options,
	hidden ...inventory.Kind,
) replay.Result {
	t.Helper()
	result, err := replay.Run(context.Background(), log,
		newDependencies(hidden...), opts)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// resolvedRootKinds lists the root kinds of the incidents a replay
// resolved.
func resolvedRootKinds(result replay.Result) map[inventory.Kind]bool {
	kinds := map[inventory.Kind]bool{}
	for _, d := range result.Decisions {
		if d.Action == incident.Resolve && d.Incident.ID != "" &&
			d.Reason != incident.ReasonSuperseded {
			kinds[d.Incident.Root.Kind] = true
		}
	}
	return kinds
}

// The absence of findings on a kind kwatch cannot observe is missing
// data, not recovery: with the root kind unverifiable, no scenario may
// resolve an incident rooted there.
func TestScenariosNeverResolveUnverifiableRoots(t *testing.T) {
	checked := 0
	for _, s := range library() {
		log, e := loadScenario(t, s.expect.Name)
		opts := e.options(log.Start)
		for kind := range resolvedRootKinds(replayLog(t, log, opts)) {
			hidden := replayHiding(t, log, opts, kind)
			checked++
			for _, d := range hidden.Decisions {
				if d.Action == incident.Resolve &&
					d.Incident.Root.Kind == kind &&
					d.Reason != incident.ReasonSuperseded {
					t.Errorf("%s: resolved %s although %s is not "+
						"observable", e.Name, d.Incident.Root, kind)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no scenario resolves an incident; nothing was checked")
	}
	t.Logf("checked %d resolving root kinds", checked)
}

// Scenario messages are written for a named cluster: every incident
// message names it, the way production does when clusterName is set.
func TestScenarioMessagesNameTheCluster(t *testing.T) {
	for _, s := range library() {
		log, e := loadScenario(t, s.expect.Name)
		result := replayLog(t, log, e.options(log.Start))
		for i, m := range result.Messages {
			if !strings.Contains(m.Note, "("+scenarioCluster+")") {
				t.Errorf("%s message %d does not name the cluster: %q",
					e.Name, i, m.Note)
			}
		}
	}
}

// The engine persists incidents through its store during a replay, as
// it does in production.
func TestScenarioEngineWritesItsStore(t *testing.T) {
	log, e := loadScenario(t, "bad-rollout")
	deps := newDependencies()
	if _, err := replay.Run(context.Background(), log, deps,
		e.options(log.Start)); err != nil {
		t.Fatal(err)
	}
	store := deps.Store.(*memoryStore)
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.saves == 0 {
		t.Fatal("the engine never saved its incidents")
	}
}
