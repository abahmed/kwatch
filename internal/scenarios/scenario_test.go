package scenarios

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/replay"
)

var update = flag.Bool("update", false,
	"rewrite the scenario logs and expectations in testdata")

// scenarioStart is when every labelled scenario begins. Nothing depends on
// the date; a fixed one keeps the logs byte-for-byte reproducible.
var scenarioStart = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

// Expected roots that are not entities.
const (
	// rootUnknown expects an incident that states no cause: nothing may
	// be blamed.
	rootUnknown = "unknown"
)

// expectation is the label of one scenario, committed next to its log as
// <name>.expect.json.
type expectation struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Root is the entity the first incident must end rooted at, as
	// "kind/namespace/name", or "unknown" when it must state no cause.
	// Empty only for a scenario that must stay quiet.
	Root string `json:"root,omitempty"`
	// OtherRoots are roots of further, independent incidents the scenario
	// must also report, each as its own incident.
	OtherRoots []string `json:"otherRoots,omitempty"`
	// Tier is the expected delivery tier of the first incident.
	Tier string `json:"tier,omitempty"`
	// BootHeld marks a scenario whose first message is held on purpose
	// by the node boot grace. It is measured by its own gate, not the
	// general first-message gate.
	BootHeld bool `json:"bootHeld,omitempty"`
	// MaxMessages bounds every message the scenario may produce.
	MaxMessages int `json:"maxMessages"`
	// MustNotBlame lists entities no stated cause may name.
	MustNotBlame []string `json:"mustNotBlame,omitempty"`
	// Quiet scenarios must produce no message at all.
	Quiet bool `json:"quiet,omitempty"`
	// SyncAfter, when set, signals that every source synced this long
	// after the start, as a cold start of kwatch does.
	SyncAfter duration `json:"syncAfter,omitempty"`
	// Tail is how long the replay runs after the last observation. Zero
	// takes the replay default.
	Tail duration `json:"tail,omitempty"`
}

// roots lists every expected root, the first incident's first.
func (e expectation) roots() []string {
	if e.Quiet {
		return nil
	}
	return append([]string{e.Root}, e.OtherRoots...)
}

// options returns the replay options the scenario needs.
func (e expectation) options(start time.Time) replay.Options {
	opts := replay.Options{Tail: time.Duration(e.Tail)}
	if e.SyncAfter > 0 {
		opts.SyncAt = start.Add(time.Duration(e.SyncAfter))
	}
	return opts
}

// duration is a time.Duration written as "90s" in expectation files.
type duration time.Duration

func (d duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *duration) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(text)
	*d = duration(parsed)
	return err
}

// scenario is one labelled case: the label and the generator that records
// what the cluster shows.
type scenario struct {
	expect expectation
	build  func(c *cluster)
}

// library returns every labelled scenario in a fixed order.
func library() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		workloadScenarios(), configScenarios(), nodeScenarios(),
		clusterScenarios(), schedulingScenarios(), lifecycleScenarios(),
		controlPlaneScenarios(), accessScenarios(), trafficScenarios(),
		portScenarios(), initWaitScenarios(),
		certificateScenarios(), nodeLifecycleScenarios(),
		operatorScenarios(), workloadConfigScenarios(),
		containerScenarios(), borderlineScenarios(),
		borderlineTrafficScenarios(), storageScenarios(),
		admissionScenarios(), webhookTLSScenarios(),
		loadBalancerScenarios(),
		trafficBackendScenarios(),
		autoscalingScenarios(), sharedErrorScenarios(),
		logsOnlyScenarios(),
		commonFactorScenarios(), dependencyScenarios(), bootScenarios(),
		bootEndpointScenarios(),
		releaseScenarios(), metricsHPAScenarios(), nodeFlapScenarios(),
		nodePinnedScenarios(),
		recentChangeScenarios(), fixAttemptScenarios(),
		flipScenarios(), unschedulableQuantifiedScenarios(),
		schedulingFitScenarios(),
		readyNeverScenarios(), readyZeroScenarios(),
		namespaceOutageScenarios(),
		reopenScenarios(), jobLongScenarios(), scaleZeroScenarios(),
		firstRolloutScenarios(), usageHistoryScenarios(), nodeOOMScenarios(),
		probeThrottleScenarios(),
		kwatchViewScenarios(), escalationScenarios(),
		crashReplaceScenarios(), metricsBlipScenarios(),
		digestFlapScenarios(), deprecatedAPIScenarios(),
		upgradeReadinessScenarios(),
		controlPlaneLoadScenarios(),
		serviceCallScenarios(), chainScenarios(), impactScenarios(),
		rolloutHoldScenarios(), baselineScenarios(),
		counterfactualScenarios(), revisionDiffScenarios(),
		ackScenarios(),
		configVersionScenarios(), wakeScenarios(), ongoingScenarios(),
		archScenarios(), ipScenarios(), preemptionScenarios(),
		graceKillScenarios(),
		flappingScenarios(), missingServiceScenarios(),
		livenessStartScenarios(),
		livenessCascadeScenarios(),
		stuckScenarios(),
		replacedFindingScenarios(), sandboxBlipScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}

// generate records a scenario into a log. suffix makes every name unique
// when the scenario runs as one instance of many.
func (s scenario) generate(start time.Time, suffix string) replay.Log {
	c := newCluster(start, suffix)
	s.build(c)
	return c.log()
}

// Directories of the committed logs. The labelled set may be used to
// develop the engine; the held-out set never is (see heldout_test.go).
var (
	labelledDir = "testdata"
	heldoutDir  = filepath.Join("testdata", "heldout")
)

func logPath(dir, name string) string {
	return filepath.Join(dir, name+".jsonl")
}

func expectPath(dir, name string) string {
	return filepath.Join(dir, name+".expect.json")
}

// encodeLog and encodeExpectation render the committed files.
func encodeLog(t *testing.T, log replay.Log) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := replay.Write(&buf, log); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func encodeExpectation(t *testing.T, e expectation) []byte {
	t.Helper()
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

// TestScenarioFixtures keeps the committed logs and labels in step with
// their generators. Regenerate them with:
//
//	go test ./internal/scenarios -run TestScenarioFixtures -update
func TestScenarioFixtures(t *testing.T) {
	runParallelUnlessUpdating(t)
	seen := map[string]bool{}
	checkFixtures(t, labelledDir, library(), seen)
	checkFixtures(t, heldoutDir, heldoutLibrary(), seen)
}

// checkFixtures compares, or with -update rewrites, the committed files
// of scenarios in dir. seen catches a name used twice in either set.
func checkFixtures(
	t *testing.T, dir string, scenarios []scenario, seen map[string]bool,
) {
	t.Helper()
	for _, s := range scenarios {
		name := s.expect.Name
		if seen[name] {
			t.Fatalf("scenario %s is defined twice", name)
		}
		seen[name] = true
		files := map[string][]byte{
			logPath(dir, name): encodeLog(t,
				s.generate(scenarioStart, "")),
			expectPath(dir, name): encodeExpectation(t, s.expect),
		}
		for path, want := range files {
			if *update {
				if err := os.WriteFile(path, want, 0o600); err != nil {
					t.Fatal(err)
				}
				continue
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, want) {
				t.Errorf("%s is stale; rerun with -update", path)
			}
		}
	}
}

// TestScenarioLabelsAreComplete checks every label can be judged.
func TestScenarioLabelsAreComplete(t *testing.T) {
	for _, s := range append(library(), heldoutLibrary()...) {
		e := s.expect
		switch {
		case e.Name == "" || e.Description == "":
			t.Errorf("%+v needs a name and a description", e)
		case e.Quiet && (e.Root != "" || e.MaxMessages != 0):
			t.Errorf("%s: a quiet scenario has no root and no messages",
				e.Name)
		case !e.Quiet && (e.Root == "" || e.MaxMessages <= 0 ||
			e.Tier == ""):
			t.Errorf("%s needs a root, a tier and a message budget", e.Name)
		}
	}
}

// loadScenario reads a committed labelled log and its label.
func loadScenario(t *testing.T, name string) (replay.Log, expectation) {
	t.Helper()
	return loadScenarioFrom(t, labelledDir, name)
}

// loadScenarioFrom reads a committed log and its label from dir.
func loadScenarioFrom(
	t *testing.T, dir, name string,
) (replay.Log, expectation) {
	t.Helper()
	log, e, err := readScenarioFrom(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	return log, e
}

// readScenarioFrom is loadScenarioFrom for callers that cannot fail a test.
func readScenarioFrom(dir, name string) (replay.Log, expectation, error) {
	var e expectation
	file, err := os.Open(logPath(dir, name))
	if err != nil {
		return replay.Log{}, e, err
	}
	defer func() { _ = file.Close() }()
	log, err := replay.Read(file)
	if err != nil {
		return replay.Log{}, e, fmt.Errorf("%s: %w", name, err)
	}
	data, err := os.ReadFile(expectPath(dir, name))
	if err != nil {
		return replay.Log{}, e, err
	}
	if err := json.Unmarshal(data, &e); err != nil {
		return replay.Log{}, e, fmt.Errorf("%s: %w",
			expectPath(dir, name), err)
	}
	return log, e, nil
}
