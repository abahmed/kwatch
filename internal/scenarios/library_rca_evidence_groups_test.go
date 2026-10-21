package scenarios

// rcaEvidenceScenarios are the cases where something changed near a
// failure that it did not cause: the cause must be earned by evidence
// that ties the change to the failure, never by the change alone.
func rcaEvidenceScenarios() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		rcaEvidenceChangeScenarios(),
		rcaEvidenceAccessScenarios(),
		rcaEvidenceProbeScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}

// rcaEvidencePanic is a crash with a defect in the program's own code:
// no setting, secret, policy or permission produces these words.
const rcaEvidencePanic = "panic: runtime error: invalid memory address " +
	"or nil pointer dereference"
