package scenarios

// gap5aScenarios are the scenarios written from SCENARIO_GAPS_5A.md
// (managed-cluster guides, kubeadm and the Kubernetes docs), grouped by
// area in the library_gap5a_*_test.go files.
func gap5aScenarios() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		gap5aDNSScenarios(), gap5aNodeScenarios(),
		gap5aMetricsScenarios(),
		gap5aPressureScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}

// gap5bScenarios are the scenarios written from SCENARIO_GAPS_5B.md
// (reports of real users), grouped by area in library_gap5b_*_test.go.
func gap5bScenarios() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		gap5bStateScenarios(), gap5bReleaseScenarios(),
		gap5bClusterScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}
