package scenarios

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/flight"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/scorecard"
	"github.com/abahmed/kwatch/internal/storage"
)

// The recorded set holds incidents captured from real clusters by the
// flight recorder (docs/production-operations.md, "Capturing an
// incident"). Like the held-out set it is scored but never tuned against:
// a miss is reported, not fixed by changing rules to fit the file. It is
// scored on its own, not in the labelled accuracy or the calibration, and
// its gate stays informational until recordedGateMinCases cases exist.
//
// Each case is <name>.jsonl, the exported log, and <name>.expect.json,
// the label a person wrote before the first replay, with the names the
// export used (pseudonyms). Files are found by listing the directory, so
// adding a case is adding two files.
var recordedDir = filepath.Join("testdata", "recorded")

// recordedGateMinCases is how many recorded cases the accuracy gate
// needs before it is enforced.
const recordedGateMinCases = 10

// recordedNames lists the recorded cases, sorted.
func recordedNames(t testing.TB) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(recordedDir, "*.expect.json"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names,
			strings.TrimSuffix(filepath.Base(f), ".expect.json"))
	}
	sort.Strings(names)
	return names
}

// recordedResult is the replay of every recorded case.
type recordedResult struct {
	verdicts []verdict
	accuracy scorecard.Accuracy
}

func measureRecorded(t *testing.T) recordedResult {
	t.Helper()
	var r recordedResult
	var cases []scorecard.Case
	for _, name := range recordedNames(t) {
		log, e, result := replayScenario(t, recordedDir, name)
		v := judge(e, log, result)
		r.verdicts = append(r.verdicts, v)
		cases = append(cases, v.cases...)
	}
	r.accuracy = scorecard.ScoreCases(cases)
	return r
}

// recordedGateLine is the informational gate: the accuracy of the
// recorded cases, never failing, with how many more are needed before
// it is enforced.
func recordedGateLine(r recordedResult) string {
	n := len(r.verdicts)
	if n >= recordedGateMinCases {
		return fmt.Sprintf("Recorded accuracy gate: %d of %d recorded "+
			"root cases right. Informational: no target is set yet.\n",
			r.accuracy.Correct, r.accuracy.Cases)
	}
	return fmt.Sprintf("Recorded accuracy gate: not gated, %d of %d "+
		"recorded cases needed (%d of %d root cases right so far).\n",
		n, recordedGateMinCases, r.accuracy.Correct, r.accuracy.Cases)
}

// TestRecordedScenariosAreLabelled keeps every recorded file paired and
// loadable, and the set apart from the other two.
func TestRecordedScenariosAreLabelled(t *testing.T) {
	t.Parallel()
	other := map[string]bool{}
	for _, s := range append(library(), heldoutLibrary()...) {
		other[s.expect.Name] = true
	}
	for _, name := range recordedNames(t) {
		_, e, err := readScenarioFrom(recordedDir, name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if other[name] {
			t.Errorf("%s is recorded and also labelled or held out", name)
		}
		if e.Name != name {
			t.Errorf("%s: label names %q", name, e.Name)
		}
	}
	files, _ := filepath.Glob(filepath.Join(recordedDir, "*.jsonl"))
	if len(files) != len(recordedNames(t)) {
		t.Errorf("%d logs for %d labels", len(files), len(recordedNames(t)))
	}
}

func TestRecordedGateIsInformationalUntilEnoughCases(t *testing.T) {
	t.Parallel()
	few := recordedResult{verdicts: make([]verdict, 1)}
	if line := recordedGateLine(few); !strings.Contains(line, "not gated") {
		t.Errorf("one case must not be gated: %s", line)
	}
	many := recordedResult{verdicts: make([]verdict, recordedGateMinCases)}
	if line := recordedGateLine(many); strings.Contains(line, "not gated") {
		t.Errorf("enough cases must report the accuracy: %s", line)
	}
}

// recordedExamples are the committed recorded cases, each built from a
// synthetic scenario that goes through the flight recorder and the
// export, to prove the format works end to end. A real capture replaces
// them the same way: two files in testdata/recorded.
var recordedExamples = []struct{ name, from string }{
	{"recorded-bad-rollout", "bad-rollout"},
}

// recordedSalt keys the example's pseudonyms. It is committed: the
// example holds no real names.
const recordedSalt = "kwatch-recorded-example"

// TestRecordedFixtures keeps the examples in step with the recorder,
// sanitizer and export that build them. Regenerate one with:
//
//	go test ./internal/scenarios -run 'TestRecordedFixtures/name$' -update
func TestRecordedFixtures(t *testing.T) {
	requireScopedUpdate(t)
	runParallelUnlessUpdating(t)
	for _, ex := range recordedExamples {
		t.Run(ex.name, func(t *testing.T) {
			log, e := loadScenario(t, ex.from)
			got := recordedExample(t, ex.name, log, e)
			files := map[string][]byte{
				logPath(recordedDir, ex.name):    got.log,
				expectPath(recordedDir, ex.name): got.expect,
			}
			for path, want := range files {
				if *update {
					if err := os.WriteFile(path, want, 0o600); err != nil {
						t.Fatal(err)
					}
					continue
				}
				have, err := os.ReadFile(path)
				if err != nil || string(have) != string(want) {
					t.Errorf("%s is stale; rerun with -update", path)
				}
			}
		})
	}
}

type recordedFiles struct{ log, expect []byte }

// recordedExample records log through the flight recorder, exports the
// ring and labels the result with the pseudonyms of e's names.
func recordedExample(
	t *testing.T, name string, log replay.Log, e expectation,
) recordedFiles {
	t.Helper()
	var now time.Time
	clock := func() time.Time { return now }
	store, err := storage.Open(filepath.Join(t.TempDir(), "state.db"),
		storage.Options{Now: clock})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if _, err := store.Claim(); err != nil {
		t.Fatal(err)
	}
	rec, err := flight.NewRecorder(flight.NewRing(store), recordedSalt, clock)
	if err != nil {
		t.Fatal(err)
	}
	submit := rec.Wrap(func(context.Context, ...inventory.Observation) {})
	now = log.Start
	for _, entry := range log.Entries {
		now = entry.At
		submit(context.Background(), entry.Observation)
	}
	rec.Flush()
	var out strings.Builder
	if _, err := flight.ExportFrom(flight.NewRing(store), time.Time{},
		time.Time{}, &out); err != nil {
		t.Fatal(err)
	}
	return recordedFiles{
		log:    []byte(out.String()),
		expect: encodeExpectation(t, pseudonymLabel(t, name, e)),
	}
}

// pseudonymLabel renames the entities of e's label the way the recorder
// renamed them.
func pseudonymLabel(t *testing.T, name string, e expectation) expectation {
	t.Helper()
	s, err := replay.NewSanitizer(replay.SanitizeOptions{Salt: recordedSalt})
	if err != nil {
		t.Fatal(err)
	}
	rename := func(root string) string {
		parts := strings.SplitN(root, "/", 3)
		if len(parts) != 3 {
			return root
		}
		id := inventory.EntityID{Kind: inventory.Kind(parts[0]),
			Namespace: parts[1], Name: parts[2]}
		o := s.Observation(inventory.Observation{
			Kind: inventory.Observed, Entity: id})
		return o.Entity.String()
	}
	renameAll := func(in []string) []string {
		var out []string
		for _, r := range in {
			out = append(out, rename(r))
		}
		return out
	}
	e.Name = name
	e.Description = "Recorded-format example: the synthetic scenario " +
		"through the flight recorder, sanitizer and export. " + e.Description
	e.Root = rename(e.Root)
	e.OtherRoots = renameAll(e.OtherRoots)
	e.Unrelated = renameAll(e.Unrelated)
	e.MustNotBlame = renameAll(e.MustNotBlame)
	return e
}

// TestRecordedExamplesAreScored replays the recorded cases and fails
// only on what no tuning could excuse: a log that does not load or
// replay. Whether the answers are right is reported by the scorecard, not
// enforced here.
func TestRecordedExamplesAreScored(t *testing.T) {
	t.Parallel()
	r := measureRecorded(t)
	if len(r.verdicts) == 0 {
		t.Skip("no recorded cases")
	}
	for _, v := range r.verdicts {
		t.Logf("%s: %s", v.expect.Name, outcome(v))
	}
}
