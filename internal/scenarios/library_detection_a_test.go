package scenarios

// detectionAScenarios are the scenarios of the detection fixes: slow
// starts that are not failures, failures that the library did not yet
// expect to be heard, and the quiet twin of each.
func detectionAScenarios() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		nodeJoinScenarios(), serviceSelectorScenarios(),
		jobRetryScenarios(), kubeletNoticeScenarios(), chronicScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}
