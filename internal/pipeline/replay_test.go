package pipeline_test

import (
	"context"
	"os"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/pipeline"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

const recordedRollout = "../replay/testdata/bad_rollout.jsonl"

func replayDependencies() pipeline.Dependencies {
	return pipeline.Dependencies{
		Model: inventory.NewModel(inventory.Options{}),
		Detectors: detection.NewRegistry(nil,
			detectors.Container{}, detectors.NewPod(detectors.PodThresholds{}),
			detectors.NewNode(0), detectors.NewWorkload(0),
		),
		Incidents: incident.NewManager(incident.Config{}, explain.NewSolver()),
	}
}

func readRecordedRollout(t *testing.T) replay.Log {
	t.Helper()
	file, err := os.Open(recordedRollout)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	log, err := replay.Read(file)
	if err != nil {
		t.Fatal(err)
	}
	return log
}

// The engine tells one story for a recorded bad rollout, rooted at the
// deployment and blaming the change.
func TestEngineReplaysRecordedRollout(t *testing.T) {
	result, err := replay.Run(context.Background(), readRecordedRollout(t),
		replayDependencies(), replay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Decisions) != 1 {
		t.Fatalf("got %d decisions, want one announcement",
			len(result.Decisions))
	}
	d := result.Decisions[0]
	if d.Action != incident.Announce ||
		d.Incident.Root.String() != "deployment/shop/payments" ||
		d.Incident.Cause == nil || d.Incident.Cause.Change == nil {
		t.Fatalf("decision %+v", d)
	}
}

// Out-of-scope incidents are tracked but never delivered.
func TestEngineReplayDropsOutOfScopeDecisions(t *testing.T) {
	deps := replayDependencies()
	deps.InScope = func(incident.Incident) bool { return false }
	result, err := replay.Run(context.Background(), readRecordedRollout(t),
		deps, replay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Decisions) != 0 {
		t.Fatalf("delivered %d out-of-scope decisions",
			len(result.Decisions))
	}
	if len(result.Incidents) == 0 {
		t.Fatal("out-of-scope incidents must still be tracked")
	}
}
