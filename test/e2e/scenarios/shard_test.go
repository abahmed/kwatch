//go:build e2e

package scenarios

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

func TestShardPlanBalancesWork(t *testing.T) {
	for _, shards := range []int{4, 6, 8, 10} {
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
	for _, shards := range []int{1, 4, 6, 10} {
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
		for index := 1; index <= 10; index++ {
			if belongsToShard(id, fmt.Sprintf("%d/10", index)) {
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
	if belongsToShard("anything", "11/10") {
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
		`^\s*(inNamespace|inNamespaceEarly|inExtendedNamespace)\(`)
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

// Every exclusive scenario must say what it disturbs, and no other one may.
func TestAloneScenariosNameWhatTheyDisturb(t *testing.T) {
	for test, cost := range scenarioCosts {
		if cost.Alone && cost.Disturbs == "" {
			t.Errorf("%s is Alone but names nothing it disturbs", test)
		}
		if !cost.Alone && cost.Disturbs != "" {
			t.Errorf("%s disturbs %q but is not Alone", test, cost.Disturbs)
		}
	}
}

// An Own scenario must be the only one on its shard.
func TestOwnScenariosShareNoShard(t *testing.T) {
	plan := shardPlan(10)
	for test, cost := range scenarioCosts {
		if !cost.Own {
			continue
		}
		for other, shard := range plan {
			if other != test && shard == plan[test] {
				t.Errorf("%s shares shard %d with %s", test, shard+1, other)
			}
		}
	}
}

// Two scenarios that stop a worker node must not share a cluster.
func TestNodeStoppingScenariosUseDifferentShards(t *testing.T) {
	for _, shards := range []int{6, 8, 10} {
		plan := shardPlan(shards)
		seen := map[int]string{}
		for test, cost := range scenarioCosts {
			if cost.Disturbs != nodeDisturbance {
				continue
			}
			if other, ok := seen[plan[test]]; ok {
				t.Errorf("%d shards: %s and %s share shard %d",
					shards, test, other, plan[test]+1)
			}
			seen[plan[test]] = test
		}
	}
}

// Own scenarios must have their cluster to themselves: anything else in
// the shard runs before or beside them and delays the slowest job.
func TestOwnScenarioShardHoldsNothingElse(t *testing.T) {
	plan := shardPlan(10)
	for own, ownShard := range plan {
		if !costOf(own).Own {
			continue
		}
		for test, shard := range plan {
			if test != own && shard == ownShard {
				t.Errorf("%s shares shard %d with own scenario %s",
					test, shard+1, own)
			}
		}
	}
}

// Every scenario test is placed by the plan through its test name, never
// by the hash fallback, even when its helper is given a non-catalog ID.
func TestEveryScenarioTestIsPlannedByName(t *testing.T) {
	plan := shardPlan(10)
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	// A scenario test calls one of the run helpers first thing.
	helper := regexp.MustCompile(`func (TestScenario\w+)\(t \*testing\.T\)` +
		` \{\s+(in\w*Namespace\w*|in\w*Cluster|onCluster)\(t, `)
	found := 0
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range helper.FindAllStringSubmatch(string(body), -1) {
			test := match[1]
			found++
			if _, ok := plan[test]; !ok {
				t.Errorf("%s (%s) is not in the shard plan", test, file)
				continue
			}
			if got := shardOf(test, 10); got != plan[test] {
				t.Errorf("%s: shardOf = %d, plan = %d", test, got, plan[test])
			}
		}
	}
	if found < len(plan) {
		t.Errorf("found %d scenario tests, plan has %d", found, len(plan))
	}
}
