package detection

// evidenceImage is the image a container runs, as the container
// detector writes it.
const evidenceImage = "image"

// fromErrorLine are the evidence labels a detector derives from the
// error line alone, and with them rewords the finding: the CPU
// architecture an "exec format error" proves (container_arch.go). They
// say more than the line: they name the cause.
var fromErrorLine = map[string]bool{
	EvidenceArchNode:    true,
	EvidenceArchImage:   true,
	EvidenceArchHealthy: true,
}

// keepProof keeps the account an error line gave of a failure while a
// later look at the same failure has no line of its own. The crash log
// is read once per run and is empty between some runs; a run that left
// no line neither confirms nor contradicts the one before, so the more
// specific account stays: "crashes on arm64 node n1 with "exec format
// error"" is not replaced by "keeps crashing". It stays only while the
// failure is the same (same reason, as the tracker keys it, and same
// mode) and the container runs the same image; a new line, another
// image or another mode replaces it.
func keepProof(old, current Finding) Finding {
	line := evidenceValue(old, EvidenceError)
	if line == "" || evidenceValue(current, EvidenceError) != "" ||
		old.Mode != current.Mode || evidenceValue(old, evidenceImage) !=
		evidenceValue(current, evidenceImage) {
		return current
	}
	kept := append([]Evidence(nil), current.Evidence...)
	kept = append(kept, Evidence{Label: EvidenceError, Value: line})
	derived := false
	for _, e := range old.Evidence {
		if fromErrorLine[e.Label] && evidenceValue(current, e.Label) == "" {
			kept = append(kept, e)
			derived = true
		}
	}
	current.Evidence = kept
	if derived {
		// The detector worded the finding from what the line proves.
		current.Summary = old.Summary
	}
	return current
}
