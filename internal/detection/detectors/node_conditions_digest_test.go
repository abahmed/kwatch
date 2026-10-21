package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
)

// A True node condition that says nothing readable (no message) and that
// is not a type a node problem reporter is known to set is not worth a
// digest line of its own: it stays evidence inside the incident of a real
// failure of the node, as an advisory finding.
func TestNodeConditionWithoutMessageOrKnownNameIsEvidenceOnly(t *testing.T) {
	m, node := conditionedNode(map[string]string{"Mystery": "True"})
	r := detection.NewRegistry(nil, NewNode(0))

	got := r.Evaluate(m, t0.Add(time.Hour), node)

	require.Len(t, got.Findings, 1)
	assert.True(t, got.Findings[0].Advisory)
}

// A message makes the condition worth reading, and a known problem type
// is worth a line even without one.
func TestNodeConditionWithMessageOrKnownNameIsADigestLine(t *testing.T) {
	m, node := conditionedNode(map[string]string{"Mystery": "True"})
	setCondition(m, node, "Mystery", "True", "Why", "disk is failing", t0)
	r := detection.NewRegistry(nil, NewNode(0))

	got := r.Evaluate(m, t0.Add(time.Hour), node)

	require.Len(t, got.Findings, 1)
	assert.False(t, got.Findings[0].Advisory, "it has a message")

	for _, known := range []string{"KernelDeadlock", "ReadonlyFilesystem",
		"CorruptDockerOverlay2", "FrequentKubeletRestart"} {
		m, node := conditionedNode(map[string]string{known: "True"})

		got := detection.NewRegistry(nil, NewNode(0)).Evaluate(m,
			t0.Add(time.Hour), node)

		require.Len(t, got.Findings, 1, known)
		assert.False(t, got.Findings[0].Advisory, known)
	}
}
