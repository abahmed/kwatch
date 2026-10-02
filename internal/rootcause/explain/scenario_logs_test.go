package explain_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// scenarioDir holds the recorded scenario logs. They are read as files,
// so this package never imports the scenarios package.
const scenarioDir = "../../scenarios/testdata"

// logEntry is one line of an observation log after its header.
type logEntry struct {
	At          time.Time             `json:"at"`
	Observation inventory.Observation `json:"obs"`
}

// scenarioState is a replayed log: the model at the last entry and the
// findings every detector reports then.
type scenarioState struct {
	model    *inventory.Model
	findings map[inventory.EntityID][]detection.Finding
	end      time.Time
}

// loadScenario applies every observation of a log to a fresh model and
// evaluates all entities once, at the time of the last entry.
func loadScenario(t *testing.T, name string) scenarioState {
	t.Helper()
	file, err := os.Open(filepath.Join(scenarioDir, name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	scanner.Scan() // header
	model := inventory.NewModel(inventory.Options{})
	ids := map[inventory.EntityID]bool{}
	var end time.Time
	first := firstSeen{}
	for scanner.Scan() {
		var entry logEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		if _, err := model.Apply(entry.Observation); err != nil {
			t.Fatal(err)
		}
		if !end.IsZero() && entry.At.After(end) {
			first.record(evaluate(model, ids, end))
		}
		ids[entry.Observation.Entity] = true
		for _, target := range entry.Observation.Targets {
			ids[target] = true
		}
		end = entry.At
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	// The engine keeps ticking after the last observation, so judge the
	// state once detector grace periods have run, as a live run would.
	end = end.Add(settleAfterLog)
	findings := evaluate(model, ids, end)
	first.record(findings)
	first.apply(findings)
	return scenarioState{model: model, findings: findings, end: end}
}

// settleAfterLog is how long the engine ticks past the end of a log.
const settleAfterLog = detectors.DefaultConditionGrace

// firstSeen keeps when each finding was first detected, as the
// tracker does, so a finding without a Kubernetes start time keeps
// the time it appeared rather than the time of the last evaluation.
type firstSeen map[detection.Key]time.Time

func (fs firstSeen) record(
	findings map[inventory.EntityID][]detection.Finding,
) {
	for _, found := range findings {
		for _, f := range found {
			if at, ok := fs[f.Key()]; !ok || f.Since.Before(at) {
				fs[f.Key()] = f.Since
			}
		}
	}
}

func (fs firstSeen) apply(
	findings map[inventory.EntityID][]detection.Finding,
) {
	for id, found := range findings {
		for i := range found {
			found[i].Since = fs[found[i].Key()]
		}
		findings[id] = found
	}
}

func evaluate(
	model *inventory.Model, ids map[inventory.EntityID]bool, now time.Time,
) map[inventory.EntityID][]detection.Finding {
	synced := func(inventory.Kind) bool { return true }
	registry := detection.NewRegistry(synced, appDetectors()...)
	out := map[inventory.EntityID][]detection.Finding{}
	for id := range ids {
		found := registry.Evaluate(model, now, id).Findings
		if len(found) > 0 {
			out[id] = found
		}
	}
	return out
}

// appDetectors mirrors the detectors the application composes.
func appDetectors() []detection.Detector {
	return []detection.Detector{
		detectors.Generic{},
		detectors.Container{}, detectors.NewPod(detectors.PodThresholds{}),
		detectors.NewNode(0), detectors.NewWorkload(0), detectors.Job{},
		detectors.HPA{}, detectors.Claim{}, detectors.Volume{},
		detectors.Service{}, detectors.Certificate{}, detectors.Missing{},
		detectors.Event{}, detectors.Budget{}, detectors.Quota{},
		detectors.Attachment{}, detectors.Webhook{},
		detectors.NodeUsage{}, detectors.VolumeUsage{}, detectors.Custom{},
		detectors.ClusterService{}, detectors.Ingress{},
		detectors.EgressPolicy{}, detectors.Schedule{},
		detectors.Namespace{}, detectors.ContainerResources{},
		detectors.PodStorage{}, detectors.NodeHealth{},
		detectors.VersionSkew{},
		detectors.ActiveProbe{},
	}
}

// snapshotOf builds the snapshot the engine solves for a replayed log.
func snapshotOf(state scenarioState) explain.Snapshot {
	return explain.Snapshot{
		Model: state.model, Findings: state.findings,
		Links:   explain.KubeLinks{Reader: state.model},
		Changes: explain.ModelChanges{Reader: state.model},
		Synced:  func(inventory.Kind) bool { return true },
		Now:     state.end,
	}
}

// TestScenarioExplainDump prints the explanation of one scenario for
// debugging: SCENARIO=zone-failure go test -run ExplainDump -v.
func TestScenarioExplainDump(t *testing.T) {
	name := os.Getenv("SCENARIO")
	if name == "" {
		t.Skip()
	}
	ex := explain.Explain(snapshotOf(loadScenario(t, name)))
	for _, area := range ex.Areas {
		t.Logf("AREA failures=%d unexplained=%v", len(area.Failures),
			area.Unexplained)
		for _, c := range area.Causes {
			t.Logf("  CAUSE %s row=%s mode=%s conf=%.2f covers=%d %v",
				c.Root, c.Row, c.Mode, c.Confidence, len(c.Covers),
				c.Contributions)
		}
		for _, c := range area.Alternatives {
			t.Logf("  ALT %s row=%s conf=%.2f covers=%d %v", c.Root,
				c.Row, c.Confidence, len(c.Covers), c.Contributions)
		}
		for _, c := range area.Trace.Candidates {
			t.Logf("  CAND %s row=%s conf=%.2f covers=%d %v", c.Root,
				c.Row, c.Confidence, len(c.Covers), c.Contributions)
		}
		for _, r := range area.Trace.Rejected {
			t.Logf("  REJ %s: %s", r.Root, r.Reason)
		}
	}
}
