package scenarios

// gap17Scenarios are the scenarios written from SCENARIO_GAPS_17.md:
// cascades across namespaces.
func gap17Scenarios() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		gap17SharedScenarios(), gap17ConfigScenarios(),
		gap17ClusterScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}

// gap24Scenarios are the scenarios written from SCENARIO_GAPS_24.md:
// faults whose first message is judged on its words.
func gap24Scenarios() []scenario {
	return append(gap24FaultScenarios(), gap24PlatformScenarios()...)
}
