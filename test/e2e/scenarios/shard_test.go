//go:build e2e

package scenarios

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

func TestShardPlanBalancesWork(t *testing.T) {
	for _, shards := range []int{4, 5, 6} {
		checkBalance(t, shards)
	}
}

func checkBalance(t *testing.T, shards int) {
	t.Helper()
	plan := shardPlan(shards)
	if len(plan) == 0 {
		t.Fatal("no covered scenarios found in coverage.yaml")
	}
	loads := make([]shardLoad, shards)
	var all shardLoad
	for test, shard := range plan {
		loads[shard] = loads[shard].with(costOf(test))
		all = all.with(costOf(test))
	}
	// No shard can beat an even split, or the longest single scenario.
	ideal := time.Duration(all.alone+all.parallel/parallelSlots) *
		time.Second / time.Duration(shards)
	floor := max(ideal, time.Duration(all.longest)*time.Second)
	for shard, load := range loads {
		// Dealing longest first stays within a scenario of the best case.
		if load.finish() > floor+4*time.Minute {
			t.Errorf("%d shards: shard %d finishes at %s, best case %s",
				shards, shard+1, load.finish(), floor)
		}
	}
}

func TestShardPlanAssignsEveryTestToAValidShard(t *testing.T) {
	for _, shards := range []int{1, 4, 6} {
		plan := shardPlan(shards)
		for test, shard := range plan {
			if shard < 0 || shard >= shards {
				t.Errorf("%s dealt to shard %d of %d", test, shard, shards)
			}
		}
	}
}

func TestBelongsToShardPlacesEachScenarioOnce(t *testing.T) {
	for id := range scenarioTests() {
		var owners int
		for _, shard := range []string{"1/6", "2/6", "3/6", "4/6", "5/6", "6/6"} {
			if belongsToShard(id, shard) {
				owners++
			}
		}
		if owners != 1 {
			t.Errorf("%s belongs to %d shards", id, owners)
		}
	}
	if !belongsToShard("anything", "") {
		t.Error("an empty shard value must run every scenario")
	}
	if belongsToShard("anything", "7/6") {
		t.Error("an out-of-range shard must run nothing")
	}
}

func TestScenarioCostsCoverEveryCoveredTest(t *testing.T) {
	covered := map[string]bool{}
	for _, entry := range coveredEntries() {
		covered[entry.Test] = true
		if _, ok := scenarioCosts[entry.Test]; !ok {
			t.Errorf("%s has no entry in scenarioCosts", entry.Test)
		}
	}
	for test := range scenarioCosts {
		if !covered[test] {
			t.Errorf("scenarioCosts lists %s, which is not covered", test)
		}
	}
}

var (
	testFunc   = regexp.MustCompile(`(?m)^func (TestScenario\w+)\(`)
	aloneCalls = regexp.MustCompile(
		`^\s*(inNamespaceAlone|inExtendedNamespaceAlone|onCluster|` +
			`inExtendedCluster)\(`)
	sharedCalls = regexp.MustCompile(
		`^\s*(inNamespace|inExtendedNamespace)\(`)
)

// The Alone flag decides which scenarios are dealt as exclusive, so it must
// say what the test really does.
func TestScenarioCostsMatchHelpers(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		starts := testFunc.FindAllStringSubmatchIndex(source, -1)
		for i, start := range starts {
			end := len(source)
			if i+1 < len(starts) {
				end = starts[i+1][0]
			}
			body := source[start[1]:end]
			name := source[start[2]:start[3]]
			alone, ok := helperClass(body)
			if !ok {
				continue
			}
			found++
			if scenarioCosts[name].Alone != alone {
				t.Errorf("%s: scenarioCosts Alone=%v, helper says %v",
					name, scenarioCosts[name].Alone, alone)
			}
		}
	}
	if found != len(scenarioCosts) {
		t.Errorf("checked %d tests, scenarioCosts has %d",
			found, len(scenarioCosts))
	}
}

// helperClass reports whether the first helper call in body runs alone.
func helperClass(body string) (alone, ok bool) {
	for _, line := range regexp.MustCompile(`\n`).Split(body, -1) {
		if aloneCalls.MatchString(line) {
			return true, true
		}
		if sharedCalls.MatchString(line) {
			return false, true
		}
	}
	return false, false
}
