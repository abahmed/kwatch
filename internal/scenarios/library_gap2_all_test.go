package scenarios

// gap2Scenarios are the scenarios written from SCENARIO_GAPS_2.md, grouped
// by area in the library_gap2_*_test.go files.
func gap2Scenarios() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		gap2ReleaseAScenarios(), gap2ReleaseBScenarios(),
		gap2NodeScenarios(), gap2ClusterAScenarios(),
		gap2ClusterBScenarios(), gap2CtrlScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}

// gap3Scenarios are the scenarios written from SCENARIO_GAPS_3.md.
func gap3Scenarios() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		gap3NodeScenarios(), gap3CtrlAScenarios(), gap3AdmitScenarios(),
		gap3MiscAScenarios(), gap3MiscBScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}
