//go:build e2e

package scenarios

import "testing"

func TestShardPlanBalancesWork(t *testing.T) {
	const shards = 4
	plan := shardPlan(shards)
	if len(plan) == 0 {
		t.Fatal("no covered scenarios found in coverage.yaml")
	}
	minutes := map[string]int{}
	for _, entry := range coveredEntries() {
		minutes[entry.Test] = max(minutes[entry.Test], entry.Minutes, 1)
	}
	load := make([]int, shards)
	longest := 0
	for test, shard := range plan {
		load[shard] += minutes[test]
		longest = max(longest, minutes[test])
	}
	lightest, heaviest := load[0], load[0]
	for _, work := range load {
		lightest, heaviest = min(lightest, work), max(heaviest, work)
	}
	// Dealing longest first keeps every shard within one test of the others.
	if heaviest-lightest > longest {
		t.Errorf("shard work %v differs by more than %d minutes",
			load, longest)
	}
}

func TestBelongsToShardPlacesEachScenarioOnce(t *testing.T) {
	for id := range scenarioTests() {
		var owners int
		for _, shard := range []string{"1/4", "2/4", "3/4", "4/4"} {
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
	if belongsToShard("anything", "5/4") {
		t.Error("an out-of-range shard must run nothing")
	}
}
