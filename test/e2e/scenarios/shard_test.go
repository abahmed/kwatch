//go:build e2e

package scenarios

import "testing"

func TestShardOfSpreadsScenariosEvenly(t *testing.T) {
	const shards = 4
	ids := coveredScenarioIDs()
	if len(ids) == 0 {
		t.Fatal("no covered scenarios found in coverage.yaml")
	}
	counts := make([]int, shards)
	for _, id := range ids {
		counts[shardOf(id, shards)]++
	}
	for shard, count := range counts {
		if count < len(ids)/shards || count > len(ids)/shards+1 {
			t.Errorf("shard %d has %d of %d scenarios", shard+1, count, len(ids))
		}
	}
}

func TestBelongsToShardPlacesEachScenarioOnce(t *testing.T) {
	for _, id := range coveredScenarioIDs() {
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
