package detection

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

// archFinding is a crash loop the arch detector worded from its error
// line, on image.
func archFinding(id inventory.EntityID, image string) Finding {
	return Finding{Entity: id, Reason: "CrashLoopBackOff",
		Severity: Critical,
		Summary:  `Container crashes on arm64 node n1 with "exec format error"`,
		Evidence: []Evidence{
			{Label: EvidenceError, Value: "exec /app: exec format error"},
			{Label: evidenceImage, Value: image},
			{Label: EvidenceArchNode, Value: "arm64 node n1"},
			{Label: EvidenceArchImage, Value: image},
		}}
}

// generic is the same crash loop worded without an error line.
func generic(id inventory.EntityID, image string) Finding {
	return Finding{Entity: id, Reason: "CrashLoopBackOff",
		Severity: Critical,
		Summary:  "Container keeps crashing and is restarting with back-off",
		Evidence: []Evidence{{Label: evidenceImage, Value: image}}}
}

// A run that left no log line does not turn the proven CPU mismatch
// back into "keeps crashing": nothing changes and the active finding
// keeps its account.
func TestObserveKeepsTheProofWhenTheLineGoesAway(t *testing.T) {
	tr := NewTracker()
	id := inventory.EntityID{Kind: "Container", Namespace: "shop",
		Name: "api-1/app"}
	proven := archFinding(id, "r.example.com/api:1.4")
	tr.Observe(id, []Finding{proven})

	assert.Empty(t, tr.Observe(id, []Finding{generic(id,
		"r.example.com/api:1.4")}))

	active := tr.Active(id)
	require.Len(t, active, 1)
	assert.Equal(t, proven.Summary, active[0].Summary)
	assert.Equal(t, "arm64 node n1",
		evidenceValue(active[0], EvidenceArchNode))
	assert.Equal(t, "exec /app: exec format error",
		evidenceValue(active[0], EvidenceError))
}

// Another image is another binary: the old proof says nothing about it.
func TestObserveDropsTheProofForAnotherImage(t *testing.T) {
	tr := NewTracker()
	id := inventory.EntityID{Kind: "Container", Namespace: "shop",
		Name: "api-1/app"}
	tr.Observe(id, []Finding{archFinding(id, "r.example.com/api:1.4")})

	tr.Observe(id, []Finding{generic(id, "r.example.com/api:1.5")})

	active := tr.Active(id)
	require.Len(t, active, 1)
	assert.Empty(t, evidenceValue(active[0], EvidenceArchNode))
	assert.Empty(t, evidenceValue(active[0], EvidenceError))
}

// A line of its own replaces the old account, and another failure mode
// keeps nothing of it.
func TestObserveReplacesTheProofWithNewerEvidence(t *testing.T) {
	id := inventory.EntityID{Kind: "Container", Namespace: "shop",
		Name: "api-1/app"}
	tr := NewTracker()
	tr.Observe(id, []Finding{archFinding(id, "r.example.com/api:1.4")})
	other := generic(id, "r.example.com/api:1.4")
	other.Evidence = append(other.Evidence,
		Evidence{Label: EvidenceError, Value: "panic: nil map"})
	tr.Observe(id, []Finding{other})
	assert.Equal(t, other.Summary, tr.Active(id)[0].Summary)
	assert.Empty(t, evidenceValue(tr.Active(id)[0], EvidenceArchNode))

	tr = NewTracker()
	tr.Observe(id, []Finding{archFinding(id, "r.example.com/api:1.4")})
	moved := generic(id, "r.example.com/api:1.4")
	moved.Mode = ModeOOMKilledLimit
	tr.Observe(id, []Finding{moved})
	assert.Empty(t, evidenceValue(tr.Active(id)[0], EvidenceError))
}
