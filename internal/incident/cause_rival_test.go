package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// rivalOf raises one failure explained by a node at the chosen raw score
// while the solver also weighed a config map at altRaw, and returns the
// cause the incident keeps.
func rivalOf(t *testing.T, chosenRaw, altRaw float64) *rootcause.CauseRecord {
	t.Helper()
	return rivalWithRow(t, chosenRaw, altRaw, "config-missing-or-changed")
}

// rivalWithRow is rivalOf with the alternative blamed through row.
func rivalWithRow(
	t *testing.T, chosenRaw, altRaw float64, row string,
) *rootcause.CauseRecord {
	t.Helper()
	r := newRig(t, Config{})
	pod := entity(kube.KindPod, "api")
	node := entity(kube.KindNode, "n1")
	settings := entity(kube.KindConfigMap, "settings")
	c := r.cause(pod, node, "node n1 is down")
	c.Raw, c.Confidence = chosenRaw, 0.9
	r.rule.causes[pod] = c
	r.rule.alternatives = map[inventory.EntityID][]explain.Cause{
		pod: {{Root: settings, Row: row,
			Raw: altRaw, Confidence: 0.9,
			Covers: []inventory.EntityID{pod}}}}

	r.raise(at(0), sig(pod, reasons.CrashLoopBackOff, detection.Warning))

	got := r.only().Cause
	require.NotNil(t, got)
	return got
}

// An alternative that scores within the margin below the chosen cause is
// told as a rival.
func TestCauseHasARivalWithinTheMarginBelow(t *testing.T) {
	got := rivalOf(t, 0.80, 0.77)

	require.NotNil(t, got.Rival)
	assert.Equal(t, entity(kube.KindConfigMap, "settings"), got.Rival.Root)
	assert.LessOrEqual(t, got.Score, rootcause.RivalCap)
}

// An alternative that outscores the chosen cause is always a rival, however
// far above it scores: the message must not state the lower one alone.
func TestCauseHasARivalThatOutscoresIt(t *testing.T) {
	got := rivalOf(t, 0.50, 0.95)

	require.NotNil(t, got.Rival)
	assert.Equal(t, entity(kube.KindConfigMap, "settings"), got.Rival.Root)
	assert.LessOrEqual(t, got.Score, rootcause.RivalCap)
}

// An alternative far below the chosen cause is not a rival.
func TestCauseHasNoRivalFarBelow(t *testing.T) {
	got := rivalOf(t, 0.90, 0.30)

	assert.Nil(t, got.Rival)
}

// A workload failing on its own is not a second explanation: it says no
// outside cause was found, so it never makes the chosen cause a hedge.
func TestSelfAlternativeIsNotARival(t *testing.T) {
	got := rivalWithRow(t, 0.80, 0.90, "self")

	assert.Nil(t, got.Rival)
}
