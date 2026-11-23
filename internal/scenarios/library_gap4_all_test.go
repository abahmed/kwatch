package scenarios

// gap4aScenarios are the scenarios written from SCENARIO_GAPS_4A.md
// (Kubernetes source and end-to-end tests), grouped by area in the
// library_gap4a_*_test.go files.
func gap4aScenarios() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		gap4aSchedScenarios(), gap4aAdmitScenarios(),
		gap4aNodeScenarios(), gap4aCtrlScenarios(),
		gap4aPlaneScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}

// gap4bScenarios are the scenarios written from SCENARIO_GAPS_4B.md
// (incident write-ups), without the names of the companies involved.
func gap4bScenarios() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		gap4bNodeScenarios(), gap4bPlaneAScenarios(),
		gap4bPlaneBScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}
