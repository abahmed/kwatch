package scenarios

// adversarialScenarios are failures written by someone who did not build
// the engine, to probe where it is confidently wrong: a recent change
// that is not the cause, a look-alike name, a healthy dependency, two
// faults that share a node, and states that must stay quiet. Every label
// says what a person who knows Kubernetes would conclude, and every
// scenario is a permanent regression test. Fixtures and message goldens
// live in testdata like the rest of the library; the names start with
// "adv-".
func adversarialScenarios() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		advNodeScenarios(), advConfigScenarios(), advNetworkScenarios(),
		advCapacityScenarios(), advIndependenceScenarios(),
		advReachabilityScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}
