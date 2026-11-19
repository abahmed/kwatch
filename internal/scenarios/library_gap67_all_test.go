package scenarios

// gap67Scenarios are the scenarios written from SCENARIO_GAPS_6A.md,
// _6B.md, _7A.md and _7B.md (upgrades, storage and network plumbing,
// workload patterns, security), grouped in the library_gap6*, gap7*
// files.
func gap67Scenarios() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		gap6aScenarios(), gap6bStorageScenarios(), gap6bNetScenarios(),
		gap7aScenarios(), gap7bScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}
