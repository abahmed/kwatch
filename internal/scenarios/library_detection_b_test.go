package scenarios

// detectionBScenarios are the cases of the detection fixes of the second
// review: what the detectors say about conditions, events, node reports,
// DaemonSets and admission webhooks.
func detectionBScenarios() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		detectionBWebhookScenarios(), detectionBNodeScenarios(),
		detectionBWorkloadScenarios(), detectionBCustomScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}
