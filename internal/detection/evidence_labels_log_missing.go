package detection

// Evidence of a workload whose log keeps failing to resolve a Service
// that does not exist.
const (
	// EvidenceMissingService is the Service, "name.namespace".
	EvidenceMissingService = "missing service"
	// EvidenceSimilarService is an existing Service of that namespace a
	// very small number of edits away, as a hint.
	EvidenceSimilarService = "similar service"
	// EvidenceLogSamples is how many sample windows showed the lookup.
	EvidenceLogSamples = "log samples"
)
