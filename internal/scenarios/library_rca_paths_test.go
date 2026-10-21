package scenarios

// rcaPathScenarios are the paths of root-cause reasoning that the first
// labelled set never walked: an endpoint that answers kwatch's probe, a
// fault local to one node, a controller behind its custom resources, a
// workload that moves a fault from node to node, and the objects a
// refused create or pull names. Each lives in its own file.
func rcaPathScenarios() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		rcaEndpointScenarios(), rcaNodeNetworkScenarios(),
		rcaAgentScenarios(), rcaOperatorScenarios(),
		rcaDiskHogScenarios(), rcaAccountScenarios(), rcaPullScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}
